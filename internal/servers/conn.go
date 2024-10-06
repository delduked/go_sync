package servers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

// ConnInterface defines methods that other services need from Conn
type ConnInterface interface {
	RemovePeer(peerID string)
	SendMessage(msg any)
	AddPeer(addr string)
	Close()
	Start()
}

// Conn manages connections and communication with peers.
type Conn struct {
	mu       sync.Mutex
	file     FileDataInterface
	meta     MetaInterface
	peers    map[string]*Peer
	sendChan chan any // Use empty interface to allow any type
	wg       sync.WaitGroup
}

// NewConn initializes a new Conn without peers.
func NewConn() *Conn {
	return &Conn{

		peers:    make(map[string]*Peer),
		sendChan: make(chan any, 1000),
	}
}

// Start begins the dispatch of messages to peers.
func (c *Conn) Start() {
	c.wg.Add(1)
	go c.dispatchMessages()
}

// SendMessage enqueues a message to be sent to all peers.
func (c *Conn) SendMessage(msg any) {
	c.sendChan <- msg
}

// Close gracefully shuts down the Conn and all peer connections.
func (c *Conn) Close() {
	close(c.sendChan)
	c.mu.Lock()
	for _, peer := range c.peers {
		close(peer.doneChan)
		peer.closeChannels()
		peer.closeStreams()
		peer.closeConnections()
	}
	c.mu.Unlock()
	c.wg.Wait()
}

// func (c *Conn) SetGrpc(grpc GrpcInterface) {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	c.grpc = grpc
// }

// AddPeer adds a new peer and starts managing it.
func (c *Conn) AddPeer(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if the peer already exists
	if _, exists := c.peers[addr]; exists {
		log.Printf("Peer %s already exists", addr)
		return
	}

	peer := &Peer{
		ID:                     addr,
		Addr:                   addr,
		FileSyncRequestChannel: make(chan *pb.FileSyncRequest, 100),
		HealthCheckChannel:     make(chan *pb.Ping, 100),
		MetadataRequestChannel: make(chan *pb.MetadataRequest, 100),
		ChunkRequestChannel:    make(chan *pb.ChunkRequest, 100),
		GetMissingFileChannel:  make(chan *pb.FileList, 100),
		doneChan:               make(chan struct{}),
	}
	c.peers[peer.ID] = peer

	c.wg.Add(1)
	go c.managePeer(peer)
}

// RemovePeer removes a peer and cleans up resources.
func (c *Conn) RemovePeer(peerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	peer, exists := c.peers[peerID]
	if !exists {
		log.Printf("Peer %s does not exist", peerID)
		return
	}

	// Signal the peer's goroutines to stop
	close(peer.doneChan)

	// Close channels, streams, and connections
	peer.closeChannels()
	peer.closeStreams()
	peer.closeConnections()

	// Remove the peer from the map
	delete(c.peers, peerID)
}

// managePeer handles the connection and communication with a single peer.
func (c *Conn) managePeer(peer *Peer) {
	defer c.wg.Done()
	for {
		err := c.connectPeer(peer)
		if err != nil {
			log.Printf("Failed to connect to peer %s: %v", peer.ID, err)
			select {
			case <-peer.doneChan:
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Start message senders and receivers for each stream
		if peer.SyncFileStream != nil {
			c.wg.Add(2)
			go c.fileSyncMessageSender(peer)
			go c.fileSyncMessageReceiver(peer)
		}
		if peer.HealthCheckStream != nil {
			c.wg.Add(2)
			go c.healthCheckMessageSender(peer)
			go c.healthCheckMessageReceiver(peer)
		}
		if peer.ExchangeMetadataStream != nil {
			c.wg.Add(2)
			go c.exchangeMetadataMessageSender(peer)
			go c.exchangeMetadataMessageReceiver(peer)
		}
		if peer.RequestChunksStream != nil {
			c.wg.Add(2)
			go c.requestChunksMessageSender(peer)
			go c.requestChunksMessageReceiver(peer)
		}
		if peer.GetMissingFileStream != nil {
			c.wg.Add(2)
			go c.getMissingFileSender(peer)
			go c.getMissingFileReceiver(peer)
		}

		// Monitor connection state
		stateChange := make(chan struct{})
		go c.monitorConnectionState(peer, stateChange)

		select {
		case <-peer.doneChan:
			log.Printf("Peer %s is done", peer.ID)
			peer.closeChannels()
			return
		case <-stateChange:
			log.Printf("Connection state changed for peer %s", peer.ID)
		}

		// Clean up and attempt to reconnect
		peer.closeStreams()
		peer.closeConnections()
		select {
		case <-peer.doneChan:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

// connectPeer establishes a gRPC connection and persistent streams to a peer.
func (c *Conn) connectPeer(peer *Peer) error {
	conn, err := grpc.NewClient(peer.Addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}))
	if err != nil {
		return err
	}

	client := pb.NewFileSyncServiceClient(conn)

	// Create streams for methods that use streaming
	syncFileStream, err := client.SyncFile(context.Background())
	if err != nil {
		conn.Close()
		return err
	}

	healthCheckStream, err := client.HealthCheck(context.Background())
	if err != nil {
		syncFileStream.CloseSend()
		conn.Close()
		return err
	}

	exchangeMetadataStream, err := client.ExchangeMetadata(context.Background())
	if err != nil {
		syncFileStream.CloseSend()
		healthCheckStream.CloseSend()
		conn.Close()
		return err
	}

	requestChunksStream, err := client.RequestChunks(context.Background())
	if err != nil {
		syncFileStream.CloseSend()
		healthCheckStream.CloseSend()
		exchangeMetadataStream.CloseSend()
		conn.Close()
		return err
	}

	// Initialize other streams as needed

	peer.Conn = conn
	peer.Client = client
	peer.SyncFileStream = syncFileStream
	peer.HealthCheckStream = healthCheckStream
	peer.ExchangeMetadataStream = exchangeMetadataStream
	peer.RequestChunksStream = requestChunksStream

	return nil
}

// Implement message senders and receivers for each stream

func (c *Conn) fileSyncMessageSender(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case msg, ok := <-peer.FileSyncRequestChannel:
			if !ok {
				return // Channel closed
			}
			err := peer.SyncFileStream.Send(msg)
			if err != nil {
				log.Printf("Failed to send FileSyncRequest to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

func (c *Conn) fileSyncMessageReceiver(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case <-peer.doneChan:
			return
		default:
			msg, err := peer.SyncFileStream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("FileSync stream closed by peer %s", peer.ID)
					return
				}
				log.Printf("Error receiving FileSyncResponse from peer %s: %v", peer.ID, err)
				return
			}
			c.handleFileSyncResponse(peer, msg)
		}
	}
}

func (c *Conn) healthCheckMessageSender(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case msg, ok := <-peer.HealthCheckChannel:
			if !ok {
				return // Channel closed
			}
			err := peer.HealthCheckStream.Send(msg)
			if err != nil {
				log.Printf("Failed to send HealthCheck to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

func (c *Conn) healthCheckMessageReceiver(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case <-peer.doneChan:
			return
		default:
			msg, err := peer.HealthCheckStream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("HealthCheck stream closed by peer %s", peer.ID)
					return
				}
				log.Printf("Error receiving Pong from peer %s: %v", peer.ID, err)
				return
			}
			c.handleHealthCheckResponse(msg)
		}
	}
}

func (c *Conn) exchangeMetadataMessageSender(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case msg, ok := <-peer.MetadataRequestChannel:
			if !ok {
				return // Channel closed
			}
			err := peer.ExchangeMetadataStream.Send(msg)
			if err != nil {
				log.Printf("Failed to send MetadataRequest to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

func (c *Conn) exchangeMetadataMessageReceiver(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case <-peer.doneChan:
			return
		default:
			msg, err := peer.ExchangeMetadataStream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("ExchangeMetadata stream closed by peer %s", peer.ID)
					return
				}
				log.Printf("Error receiving MetadataResponse from peer %s: %v", peer.ID, err)
				return
			}
			c.handleMetadataResponse(peer, msg)
		}
	}
}

func (c *Conn) requestChunksMessageSender(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case msg, ok := <-peer.ChunkRequestChannel:
			if !ok {
				return // Channel closed
			}
			err := peer.RequestChunksStream.Send(msg)
			if err != nil {
				log.Printf("Failed to send ChunkRequest to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

func (c *Conn) requestChunksMessageReceiver(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case <-peer.doneChan:
			return
		default:
			msg, err := peer.RequestChunksStream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("RequestChunks stream closed by peer %s", peer.ID)
					return
				}
				log.Printf("Error receiving ChunkResponse from peer %s: %v", peer.ID, err)
				return
			}
			c.handleChunkResponse(peer, msg)
		}
	}
}
func (c *Conn) getMissingFileSender(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case msg, ok := <-peer.GetMissingFileChannel:
			if !ok {
				return // Channel closed
			}
			err := peer.GetMissingFileStream.Send(msg)
			if err != nil {
				log.Printf("Failed to send ChunkRequest to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

func (c *Conn) getMissingFileReceiver(peer *Peer) {
	defer c.wg.Done()
	for {
		select {
		case <-peer.doneChan:
			return
		default:
			msg, err := peer.GetMissingFileStream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("RequestChunks stream closed by peer %s", peer.ID)
					return
				}
				log.Printf("Error receiving ChunkResponse from peer %s: %v", peer.ID, err)
				return
			}
			c.handleGetMissingFileResponse(peer, msg)
		}
	}
}

// monitorConnectionState watches for changes in the connection state to a peer.
func (c *Conn) monitorConnectionState(peer *Peer, stateChange chan struct{}) {
	defer close(stateChange)
	for {
		state := peer.Conn.GetState()
		if state == connectivity.Shutdown {
			return // Connection is shutdown, exit to trigger reconnection
		}
		if state != connectivity.Ready {
			return // Connection is not ready, exit to trigger reconnection
		}
		peer.Conn.WaitForStateChange(context.Background(), state)
	}
}

// dispatchMessages distributes messages from the central sendChan to all peers.
func (c *Conn) dispatchMessages() {
	defer c.wg.Done()
	for msg := range c.sendChan {
		c.mu.Lock()
		for _, peer := range c.peers {
			switch m := msg.(type) {
			case *pb.FileSyncRequest:
				select {
				case peer.FileSyncRequestChannel <- m:
				default:
					log.Printf("FileSyncRequestChannel for peer %s is full, dropping message", peer.ID)
				}
			case *pb.Ping:
				select {
				case peer.HealthCheckChannel <- m:
				default:
					log.Printf("HealthCheckChannel for peer %s is full, dropping message", peer.ID)
				}
			case *pb.MetadataRequest:
				select {
				case peer.MetadataRequestChannel <- m:
				default:
					log.Printf("MetadataRequestChannel for peer %s is full, dropping message", peer.ID)
				}
			case *pb.ChunkRequest:
				select {
				case peer.ChunkRequestChannel <- m:
				default:
					log.Printf("ChunkRequestChannel for peer %s is full, dropping message", peer.ID)
				}
			case *pb.FileList:
				select {
				case peer.GetMissingFileChannel <- m:
				default:
					log.Printf("GetMissingFileChannel for peer %s is full, dropping message", peer.ID)
				}
			default:
				log.Printf("Unsupported message type: %T", msg)
			}
		}
		c.mu.Unlock()
	}
}

func (c *Conn) handleFileChunk(chunk *pb.FileChunk) error {
	filePath := filepath.Join(conf.AppConfig.SyncFolder, chunk.FileName)
	defer c.file.markFileAsComplete(filePath)

	// Open the file for writing
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return err
	}
	defer file.Close()

	// Write the chunk data at the specified offset
	_, err = file.WriteAt(chunk.ChunkData, chunk.Offset)
	if err != nil {
		log.Printf("Failed to write to file %s at offset %d: %v", filePath, chunk.Offset, err)
		return err
	}

	// Update the metadata
	c.SaveMetaData(filePath, chunk.ChunkData, chunk.Offset)

	log.Printf("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)
	return nil
}
func (c *Conn) SaveMetaData(filename string, chunk []byte, offset int64) error {
	// Save new metadata
	c.meta.SaveMetaDataToMem(filename, chunk, offset)
	c.meta.SaveMetaDataToDB(filename, chunk, offset)

	return nil
}

// Peer represents a connection to a peer with persistent streams.
type Peer struct {
	ID       string
	Addr     string
	Conn     *grpc.ClientConn
	Client   pb.FileSyncServiceClient
	doneChan chan struct{}

	// Channels for different message types
	FileSyncRequestChannel chan *pb.FileSyncRequest
	HealthCheckChannel     chan *pb.Ping
	MetadataRequestChannel chan *pb.MetadataRequest
	ChunkRequestChannel    chan *pb.ChunkRequest
	GetMissingFileChannel  chan *pb.FileList

	// Streams for methods that use streaming
	SyncFileStream         pb.FileSyncService_SyncFileClient
	HealthCheckStream      pb.FileSyncService_HealthCheckClient
	ExchangeMetadataStream pb.FileSyncService_ExchangeMetadataClient
	RequestChunksStream    pb.FileSyncService_RequestChunksClient
	GetMissingFileStream   pb.FileSyncService_GetMissingFilesClient
}

// closeChannels closes all message channels for the peer.
func (peer *Peer) closeChannels() {
	log.Debugf("Closing all message channels for peer %s", peer.ID)
	close(peer.FileSyncRequestChannel)
	close(peer.HealthCheckChannel)
	close(peer.MetadataRequestChannel)
	close(peer.ChunkRequestChannel)
	close(peer.GetMissingFileChannel)
}

// closeStreams closes all gRPC streams for the peer.
func (peer *Peer) closeStreams() {
	log.Debugf("Closing all streams for peer %s", peer.ID)
	if peer.SyncFileStream != nil {
		peer.SyncFileStream.CloseSend()
		peer.SyncFileStream = nil
		log.Debugf("SyncFileStream closed for peer %s", peer.ID)
	}
	if peer.HealthCheckStream != nil {
		peer.HealthCheckStream.CloseSend()
		peer.HealthCheckStream = nil
		log.Debugf("HealthCheckStream closed for peer %s", peer.ID)
	}
	if peer.ExchangeMetadataStream != nil {
		peer.ExchangeMetadataStream.CloseSend()
		peer.ExchangeMetadataStream = nil
		log.Debugf("ExchangeMetadataStream closed for peer %s", peer.ID)
	}
	if peer.RequestChunksStream != nil {
		peer.RequestChunksStream.CloseSend()
		peer.RequestChunksStream = nil
		log.Debugf("RequestChunksStream closed for peer %s", peer.ID)
	}
	if peer.GetMissingFileStream != nil {
		peer.GetMissingFileStream.CloseSend()
		peer.GetMissingFileStream = nil
		log.Debugf("GetMissingFileStream closed for peer %s", peer.ID)
	}
}

// closeConnections closes the gRPC connection for the peer.
func (peer *Peer) closeConnections() {
	log.Debugf("Closing gRPC connection for peer %s", peer.ID)
	if peer.Conn != nil {
		peer.Conn.Close()
		peer.Conn = nil
		log.Debugf("gRPC connection closed for peer %s", peer.ID)
	}
}

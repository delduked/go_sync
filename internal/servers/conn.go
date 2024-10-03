// connn.go

package servers // Replace with your actual package name

import (
	"context"
	"log"
	"sync"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

// Payload represents a generic payload interface.
// Each specific payload type implements this interface.
type Payload interface{}

// ConnManager manages connections and communication with peers.
type ConnManager struct {
	mu       sync.Mutex
	peers    map[string]*Peer
	sendChan chan any // Use empty interface to allow any type
	wg       sync.WaitGroup
}

// NewConnManager initializes a new ConnManager without peers.
func NewConnManager() *ConnManager {
	return &ConnManager{
		peers:    make(map[string]*Peer),
		sendChan: make(chan any, 1000),
	}
}

// Start begins the dispatch of messages to peers.
func (cm *ConnManager) Start() {
	cm.wg.Add(1)
	go cm.dispatchMessages()
}

// SendMessage enqueues a message to be sent to all peers.
func (cm *ConnManager) SendMessage(msg any) {
	cm.sendChan <- msg
}

// Close gracefully shuts down the ConnManager and all peer connections.
func (cm *ConnManager) Close() {
	close(cm.sendChan)
	cm.mu.Lock()
	for _, peer := range cm.peers {
		close(peer.doneChan)
		if peer.Stream != nil {
			peer.Stream.CloseSend()
		}
		if peer.Conn != nil {
			peer.Conn.Close()
		}
	}
	cm.mu.Unlock()
	cm.wg.Wait()
}

// AddPeer adds a new peer and starts managing it.
func (cm *ConnManager) AddPeer(addr string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if the peer already exists
	if _, exists := cm.peers[addr]; exists {
		log.Printf("Peer %s already exists", addr)
		return
	}

	peer := &Peer{
		ID:                     addr,
		Addr:                   addr,
		FileSyncRequestChannel: make(chan *pb.FileSyncRequest, 100),
		doneChan:               make(chan struct{}),
	}
	cm.peers[peer.ID] = peer

	cm.wg.Add(1)
	go cm.managePeer(peer)
}

// RemovePeer removes a peer and cleans up resources.
func (cm *ConnManager) RemovePeer(peerID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	peer, exists := cm.peers[peerID]
	if !exists {
		log.Printf("Peer %s does not exist", peerID)
		return
	}

	// Signal the peer's goroutines to stop
	close(peer.doneChan)

	// Close the peer's send channel
	close(peer.FileSyncRequestChannel)

	// Close the gRPC stream and connection
	if peer.Stream != nil {
		peer.Stream.CloseSend()
		peer.Stream = nil
	}
	if peer.Conn != nil {
		peer.Conn.Close()
		peer.Conn = nil
	}

	// Remove the peer from the map
	delete(cm.peers, peerID)
}

// managePeer handles the connection and communication with a single peer.
func (cm *ConnManager) managePeer(peer *Peer) {
	defer cm.wg.Done()
	for {
		err := cm.connectPeer(peer)
		if err != nil {
			log.Printf("Failed to connect to peer %s: %v", peer.ID, err)
			select {
			case <-peer.doneChan:
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		sendDone := make(chan struct{})
		go cm.messageSender(peer, sendDone)

		stateChange := make(chan struct{})
		go cm.monitorConnectionState(peer, stateChange)

		select {
		case <-peer.doneChan:
			log.Printf("Peer %s is done", peer.ID)
			close(peer.FileSyncRequestChannel)
			return
		case <-sendDone:
			log.Printf("Message sender for peer %s exited", peer.ID)
		case <-stateChange:
			log.Printf("Connection state changed for peer %s", peer.ID)
		}

		// Clean up and attempt to reconnect
		if peer.Stream != nil {
			peer.Stream.CloseSend()
			peer.Stream = nil
		}
		if peer.Conn != nil {
			peer.Conn.Close()
			peer.Conn = nil
		}
		select {
		case <-peer.doneChan:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

// connectPeer establishes a gRPC connection and persistent stream to a peer.
func (cm *ConnManager) connectPeer(peer *Peer) error {
	conn, err := grpc.Dial(peer.Addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}))
	if err != nil {
		return err
	}

	client := pb.NewFileSyncServiceClient(conn)

	// Create the persistent stream
	stream, err := client.SyncFile(context.Background())
	if err != nil {
		conn.Close()
		return err
	}

	peer.Conn = conn
	peer.Client = client
	peer.Stream = stream
	return nil
}

// messageSender continuously sends messages to the peer over a persistent stream.
func (cm *ConnManager) messageSender(peer *Peer, done chan struct{}) {
	defer close(done)
	for {
		select {
		case msg, ok := <-peer.FileSyncRequestChannel:
			if !ok {
				return // Channel closed
			}

			err := peer.Stream.Send(msg)
			if err != nil {
				log.Printf("Failed to send message to peer %s: %v", peer.ID, err)
				return // Exit to trigger reconnection
			}
		case <-peer.doneChan:
			return
		}
	}
}

// monitorConnectionState watches for changes in the connection state to a peer.
func (cm *ConnManager) monitorConnectionState(peer *Peer, stateChange chan struct{}) {
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
func (cm *ConnManager) dispatchMessages() {
	defer cm.wg.Done()
	for msg := range cm.sendChan {
		grpcReq := convertToGRPCRequest(msg)
		if grpcReq == nil {
			log.Printf("Failed to convert message to gRPC request")
			continue
		}

		cm.mu.Lock()
		for _, peer := range cm.peers {

			switch grpcReq.(type) {
			case pb.RequestFileTransfer:
				select {
				case peer.FileTransferChannel <- grpcReq.(*pb.RequestFileTransfer):
				default:
					log.Printf("Send channel for peer %s is full, dropping message", peer.ID)
				}
			case pb.FileSyncRequest:
				select {
				case peer.FileSyncRequestChannel <- grpcReq.(*pb.FileSyncRequest):
				default:
					log.Printf("Send channel for peer %s is full, dropping message", peer.ID)
				}
			}

		}
		cm.mu.Unlock()
	}
}

// convertToGRPCRequest converts a generic message to a pb.FileSyncRequest.
// It uses type assertions based on the actual type of msg.
func convertToGRPCRequest(msg any) any {
	switch payload := msg.(type) {
	case FileChunkPayload:
		return &pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    payload.FileName,
					ChunkData:   payload.ChunkData,
					Offset:      payload.Offset,
					IsNewFile:   payload.IsNewFile,
					TotalChunks: payload.TotalChunks,
					TotalSize:   payload.TotalSize,
				},
			},
		}
	case FileDeletePayload:
		return &pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: payload.FileName,
					Offset:   payload.Offset,
				},
			},
		}
	case FileTruncatePayload:
		return &pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileTruncate{
				FileTruncate: &pb.FileTruncate{
					FileName: payload.FileName,
					Size:     payload.Size,
				},
			},
		}
	case FileTransfer:
		return &pb.RequestFileTransfer{
			FileName: payload.FileName,
		}
	default:
		log.Printf("Unsupported message type: %T", msg)
		return nil
	}
}

// Peer represents a connection to a peer with a persistent stream.
type Peer struct {
	ID                     string
	Addr                   string
	Conn                   *grpc.ClientConn
	Client                 pb.FileSyncServiceClient
	Stream                 pb.FileSyncService_SyncFileClient
	FileSyncRequestChannel chan *pb.FileSyncRequest
	FileTransferChannel    chan *pb.RequestFileTransfer
	doneChan               chan struct{}
}

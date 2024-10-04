package servers

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

// ConnManager manages connections and communication with peers.
type Conn struct {
    mu       sync.Mutex
    peers    map[string]*Peer
    sendChan chan any // Use empty interface to allow any type
    wg       sync.WaitGroup
}

// NewConnManager initializes a new ConnManager without peers.
func NewConnManager() *Conn {
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

// Close gracefully shuts down the ConnManager and all peer connections.
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

        // Start message senders for each stream
        if peer.SyncFileStream != nil {
            c.wg.Add(1)
            go c.fileSyncMessageSender(peer)
        }
        if peer.HealthCheckStream != nil {
            c.wg.Add(1)
            go c.healthCheckMessageSender(peer)
        }
        if peer.ExchangeMetadataStream != nil {
            c.wg.Add(1)
            go c.exchangeMetadataMessageSender(peer)
        }
        if peer.RequestChunksStream != nil {
            c.wg.Add(1)
            go c.requestChunksMessageSender(peer)
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

// Implement message senders for each stream

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
            // Add cases for other message types as needed
            default:
                log.Printf("Unsupported message type: %T", msg)
            }
        }
        c.mu.Unlock()
    }
}

// Peer represents a connection to a peer with persistent streams.
type Peer struct {
    ID                     string
    Addr                   string
    Conn                   *grpc.ClientConn
    Client                 pb.FileSyncServiceClient
    doneChan               chan struct{}
    // Channels for different message types
    FileSyncRequestChannel chan *pb.FileSyncRequest
    HealthCheckChannel     chan *pb.Ping
    MetadataRequestChannel chan *pb.MetadataRequest
    ChunkRequestChannel    chan *pb.ChunkRequest
    // Streams for methods that use streaming
    SyncFileStream         pb.FileSyncService_SyncFileClient
    HealthCheckStream      pb.FileSyncService_HealthCheckClient
    ExchangeMetadataStream pb.FileSyncService_ExchangeMetadataClient
    RequestChunksStream    pb.FileSyncService_RequestChunksClient
}

// Methods to close channels, streams, and connections

func (peer *Peer) closeChannels() {
    close(peer.FileSyncRequestChannel)
    close(peer.HealthCheckChannel)
    close(peer.MetadataRequestChannel)
    close(peer.ChunkRequestChannel)
    // Close other channels
}

func (peer *Peer) closeStreams() {
    if peer.SyncFileStream != nil {
        peer.SyncFileStream.CloseSend()
        peer.SyncFileStream = nil
    }
    if peer.HealthCheckStream != nil {
        peer.HealthCheckStream.CloseSend()
        peer.HealthCheckStream = nil
    }
    if peer.ExchangeMetadataStream != nil {
        peer.ExchangeMetadataStream.CloseSend()
        peer.ExchangeMetadataStream = nil
    }
    if peer.RequestChunksStream != nil {
        peer.RequestChunksStream.CloseSend()
        peer.RequestChunksStream = nil
    }
    // Close other streams
}

func (peer *Peer) closeConnections() {
    if peer.Conn != nil {
        peer.Conn.Close()
        peer.Conn = nil
    }
}

package servers

// import (
// 	"context"
// 	"fmt"
// 	"io"
// 	"net"
// 	"strconv"
// 	"sync"
// 	"time"

// 	"github.com/TypeTerrors/go_sync/conf"
// 	"github.com/TypeTerrors/go_sync/internal/clients"
// 	"github.com/TypeTerrors/go_sync/pkg"
// 	pb "github.com/TypeTerrors/go_sync/proto"
// 	"github.com/charmbracelet/log"
// 	"github.com/grandcat/zeroconf"
// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/keepalive"
// )

// type Mdns struct {
// 	Clients      []*grpc.ClientConn
// 	LocalIP      string
// 	Subnet       string
// 	mu           sync.Mutex
// 	SyncedFiles  map[string]bool // Set to track files being synchronized
// 	sendChannels map[string]chan *pb.FileSyncRequest
// 	streams      map[string]pb.FileSyncService_SyncFileClient
// 	done         chan struct{}
// }

// func NewMdns() *Mdns {

// 	localIP, subnet, err := pkg.GetLocalIPAndSubnet()
// 	if err != nil {
// 		log.Fatalf("Failed to get local IP and subnet: %v", err)
// 	}

// 	return &Mdns{
// 		Clients:      make([]*grpc.ClientConn, 0),
// 		SyncedFiles:  make(map[string]bool),
// 		LocalIP:      localIP,
// 		Subnet:       subnet,
// 		sendChannels: make(map[string]chan *pb.FileSyncRequest),
// 		streams:      make(map[string]pb.FileSyncService_SyncFileClient),
// 	}
// }

// func (m *Mdns) Start() {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	m.streams = make(map[string]pb.FileSyncService_SyncFileClient)
// 	for _, conn := range m.Clients {
// 		host, _, err := net.SplitHostPort(conn.Target())
// 		if err != nil {
// 			log.Errorf("Invalid client target %s: %v", conn.Target(), err)
// 			continue
// 		}
// 		if host == m.LocalIP {
// 			continue // Skip self
// 		}

// 		// Use grpc.NewClient or grpc.Dial based on your gRPC version
// 		// conn, err := grpc.NewClient(target, grpc.WithInsecure(), grpc.WithBlock())
// 		// if err != nil {
// 		// 	log.Printf("Error initializing connection with peer %s: %v", target, err)
// 		// 	continue
// 		// }
// 		client := pb.NewFileSyncServiceClient(conn)
// 		stream, err := client.SyncFile(context.Background())
// 		if err != nil {
// 			log.Printf("Error creating stream with peer %s: %v", conn.Target(), err)
// 			continue
// 		}

// 		m.streams[conn.Target()] = stream
// 		log.Printf("Initialized persistent stream with peer %s", conn.Target())
// 	}
// }

// func (m *Mdns) Scan(ctx context.Context, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	log.Infof("Local IP: %s, Subnet: %s", m.LocalIP, m.Subnet)

// 	instance := fmt.Sprintf("filesync-%s", m.LocalIP)
// 	serviceType := "_myapp_filesync._tcp"
// 	domain := "local."
// 	txtRecords := []string{"version=1.0", "service_id=go_sync"}
// 	port, err := strconv.Atoi(conf.AppConfig.Port)
// 	if err != nil {
// 		log.Fatalf("Failed to parse port: %v", err)
// 	}
// 	server, err := zeroconf.Register(instance, serviceType, domain, port, txtRecords, nil)
// 	if err != nil {
// 		log.Fatalf("Failed to register mDNS service: %v", err)
// 	}
// 	defer server.Shutdown()

// 	// Initialize mDNS resolver
// 	resolver, err := zeroconf.NewResolver(nil)
// 	if err != nil {
// 		log.Fatalf("Failed to initialize mDNS resolver: %v", err)
// 	}

// 	entries := make(chan *zeroconf.ServiceEntry)

// 	go func(results <-chan *zeroconf.ServiceEntry) {
// 		for entry := range results {
// 			if entry.Instance == instance {
// 				log.Infof("Skipping own service instance: %s", entry.Instance)
// 				continue // Skip own service
// 			}
// 			for _, ip := range entry.AddrIPv4 {
// 				if !pkg.IsInSameSubnet(ip.String(), m.Subnet) {
// 					continue
// 				}

// 				if ip.String() == m.LocalIP || entry.TTL == 0 {
// 					continue
// 				}

// 				if pkg.ValidateService(entry.Text) {
// 					log.Infof("Discovered valid service at IP: %s", ip.String())
// 					err := m.AddClientConnection(ip.String(), "50051", wg)
// 					if err != nil {
// 						log.Errorf("Failed to add client connection for %s: %v", ip.String(), err)
// 					}
// 				} else {
// 					log.Warnf("Service at IP %s did not advertise the correct service, skipping...", ip.String())
// 				}
// 			}
// 		}
// 	}(entries)

// 	err = resolver.Browse(ctx, serviceType, domain, entries)
// 	if err != nil {
// 		log.Fatalf("Failed to browse mDNS: %v", err)
// 	}

// 	<-ctx.Done()
// 	log.Warn("Shutting down mDNS discovery...")
// 	close(entries)
// }

// func (m *Mdns) Ping(ctx context.Context, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	ticker := time.NewTicker(5 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Warn("Shutting down periodic metadata exchange...")
// 			return
// 		case <-ticker.C:
// 			for _, conn := range m.Clients {
// 				if conn.Target() == m.LocalIP {
// 					continue
// 				}
// 				stream, err := clients.Ping(conn)
// 				if err != nil {
// 					log.Errorf("Failed to ping %s: %v", conn.Target(), err)
// 					continue
// 				}

// 				go func() {
// 					for {
// 						_, err := stream.Recv()
// 						if err != nil {
// 							log.Errorf("Failed to receive health check response from %s: %v", conn.Target(), err)
// 							break
// 						}
// 						// log.Infof(recv.Message)
// 					}
// 				}()
// 				stream.Send(&pb.Ping{
// 					Message: fmt.Sprintf("Ping from %v at %v", m.LocalIP, time.Now().Unix()),
// 				})
// 			}
// 		}
// 	}
// }

// func (m *Mdns) AddClientConnection(ip string, port string, wg *sync.WaitGroup) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	conn, err := grpc.NewClient(ip+":"+port, grpc.WithInsecure(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
// 		Time:                10 * time.Second,
// 		Timeout:             20 * time.Second,
// 		PermitWithoutStream: true,
// 	}))
// 	if err != nil {
// 		return fmt.Errorf("failed to connect to gRPC server at %s: %w", ip, err)
// 	}

// 	log.Infof("Created connection to: %s", conn.Target())

// 	for _, c := range m.Clients {
// 		if c.Target() == conn.Target() {
// 			log.Warnf("Connection to %s already exists", conn.Target())
// 			return nil
// 		}
// 	}

// 	stream, err := clients.SyncConn(conn)
// 	if err != nil {
// 		log.Error("failed to open SyncFile stream on %s: %w", conn.Target(), err)
// 		return err
// 	}

// 	m.mu.Lock()
// 	m.streams[conn.Target()] = stream
// 	m.mu.Unlock()

// 	// Create a send channel for the peer
// 	sendChan := make(chan *pb.FileSyncRequest, 100) // Buffered to prevent blocking
// 	m.sendChannels[conn.Target()] = sendChan

// 	// Launch a sender goroutine for the peer
// 	wg.Add(1)
// 	go func(peerID string, stream pb.FileSyncService_SyncFileClient, sendChan chan *pb.FileSyncRequest) {
// 		defer wg.Done()
// 		m.peerSender(peerID, stream, sendChan)
// 	}(conn.Target(), stream, sendChan)

// 	// Start a receiver goroutine for the stream
// 	wg.Add(1)
// 	go func(peerID string, stream pb.FileSyncService_SyncFileClient) {
// 		defer wg.Done()
// 		m.receiveResponses(peerID, stream)
// 	}(conn.Target(), stream)

// 	// Store the connection in the map
// 	m.Clients = append(m.Clients, conn)
// 	log.Infof("Added gRPC client connection to %s", conn.Target())
// 	return nil
// }

// // receiveResponses handles incoming responses from a specific peer.
// func (m *Mdns) receiveResponses(peerID string, stream pb.FileSyncService_SyncFileClient) {
// 	for {
// 		recv, err := stream.Recv()
// 		if err != nil {
// 			if err == io.EOF {
// 				log.Infof("Stream closed gracefully by peer %s", peerID)
// 			} else {
// 				log.Errorf("Failed to receive response from %s: %v", peerID, err)
// 			}
// 			break
// 		}
// 		log.Infof("Received response from %s: %s", peerID, recv.Message)
// 	}

// 	// Handle stream closure, possibly attempt reconnection
// 	m.handleStreamFailure(peerID)
// }

// // peerSender listens on the send channel and sends messages via the stream.
// func (m *Mdns) peerSender(peerID string, stream pb.FileSyncService_SyncFileClient, sendChan <-chan *pb.FileSyncRequest) {
// 	for {
// 		select {
// 		case msg := <-sendChan:
// 			if err := stream.Send(msg); err != nil {
// 				log.Errorf("Failed to send message to peer %s: %v", peerID, err)
// 				// Handle stream failure, possibly attempt reconnection
// 				m.handleStreamFailure(peerID)
// 				return
// 			}
// 		case <-m.done:
// 			// Gracefully close the stream
// 			if err := stream.CloseSend(); err != nil {
// 				log.Errorf("Failed to close stream for peer %s: %v", peerID, err)
// 			}
// 			return
// 		}
// 	}
// }

// // handleStreamFailure manages the failure of a stream to a specific peer.
// // It removes the failed stream, closes the send channel, and initiates reconnection attempts.
// func (m *Mdns) handleStreamFailure(peerID string) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	// Remove the failed stream from activeStreams
// 	stream, exists := m.streams[peerID]
// 	if exists {
// 		// Attempt to close the stream gracefully
// 		if err := stream.CloseSend(); err != nil {
// 			log.Errorf("Failed to close stream for peer %s: %v", peerID, err)
// 		}
// 		delete(m.streams, peerID)
// 	}

// 	// Close the send channel to stop the sender goroutine
// 	sendChan, exists := m.sendChannels[peerID]
// 	if exists {
// 		close(sendChan)
// 		delete(m.sendChannels, peerID)
// 	}

// 	log.Infof("Handling stream failure for peer %s. Initiating reconnection attempts.", peerID)

// 	// Start reconnection attempts in a separate goroutine
// 	go m.attemptReconnection(peerID)
// }
// func (m *Mdns) attemptReconnection(peerID string) {
// 	for {
// 		select {
// 		case <-m.done:
// 			return
// 		default:
// 			// Attempt to retrieve the connection
// 			conn := m.GetConnByTarget(peerID) // Implement this method to retrieve connection
// 			if conn == nil {
// 				log.Errorf("No connection found for peer %s during reconnection", peerID)
// 				time.Sleep(5 * time.Second)
// 				continue
// 			}

// 			// Attempt to open a new stream
// 			stream, err := clients.SyncConn(conn)
// 			if err != nil {
// 				log.Errorf("Failed to reconnect SyncFile stream on %s: %v", peerID, err)
// 				time.Sleep(5 * time.Second)
// 				continue
// 			}

// 			// Update activeStreams
// 			m.mu.Lock()
// 			m.streams[peerID] = stream
// 			m.mu.Unlock()

// 			// Create a new send channel for the peer
// 			sendChan := make(chan *pb.FileSyncRequest, 100) // Buffered
// 			m.mu.Lock()
// 			m.sendChannels[peerID] = sendChan
// 			m.mu.Unlock()

// 			// Launch a new sender goroutine for the peer
// 			go m.peerSender(peerID, stream, sendChan)

// 			// Start a new receiver goroutine for the stream
// 			go m.receiveResponses(peerID, stream)

// 			log.Infof("Reconnected SyncFile stream on %s", peerID)
// 			return
// 		}
// 	}
// }

// func (mdns *Mdns) GetConnByTarget(target string) *grpc.ClientConn {
// 	for _, conn := range mdns.Clients {
// 		if conn.Target() == target {
// 			return conn
// 		}
// 	}
// 	return nil
// }

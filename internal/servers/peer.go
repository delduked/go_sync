package servers

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type PeerData struct {
	Clients     []*grpc.ClientConn
	LocalIP     string
	Subnet      string
	Streams     map[string]pb.FileSyncService_SyncFileClient // Map of IP to stream
	mu          sync.Mutex
	SyncedFiles map[string]bool // Set to track files being synchronized
}

func NewPeerData() *PeerData {

	localIP, subnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	return &PeerData{
		Clients:     make([]*grpc.ClientConn, 0),
		SyncedFiles: make(map[string]bool),
		LocalIP:     localIP,
		Subnet:      subnet,
	}
}

func (pd *PeerData) InitializeStreams() {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	pd.Streams = make(map[string]pb.FileSyncService_SyncFileClient)
	for _, conn := range pd.Clients {
		host, _, err := net.SplitHostPort(conn.Target())
		if err != nil {
			log.Errorf("Invalid client target %s: %v", conn.Target(), err)
			continue
		}
		if host == pd.LocalIP {
			continue // Skip self
		}

		// Use grpc.NewClient or grpc.Dial based on your gRPC version
		// conn, err := grpc.NewClient(target, grpc.WithInsecure(), grpc.WithBlock())
		// if err != nil {
		// 	log.Printf("Error initializing connection with peer %s: %v", target, err)
		// 	continue
		// }
		client := pb.NewFileSyncServiceClient(conn)
		stream, err := client.SyncFile(context.Background())
		if err != nil {
			log.Printf("Error creating stream with peer %s: %v", conn.Target(), err)
			continue
		}

		pd.Streams[conn.Target()] = stream
		log.Printf("Initialized persistent stream with peer %s", conn.Target())
	}
}

func (pd *PeerData) ScanMdns(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Infof("Local IP: %s, Subnet: %s", pd.LocalIP, pd.Subnet)

	instance := fmt.Sprintf("filesync-%s", pd.LocalIP)
	serviceType := "_myapp_filesync._tcp"
	domain := "local."
	txtRecords := []string{"version=1.0", "service_id=go_sync"}

	server, err := zeroconf.Register(instance, serviceType, domain, 50051, txtRecords, nil)
	if err != nil {
		log.Fatalf("Failed to register mDNS service: %v", err)
	}
	defer server.Shutdown()

	// Initialize mDNS resolver
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalf("Failed to initialize mDNS resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			if entry.Instance == instance {
				log.Infof("Skipping own service instance: %s", entry.Instance)
				continue // Skip own service
			}
			for _, ip := range entry.AddrIPv4 {
				if !pkg.IsInSameSubnet(ip.String(), pd.Subnet) {
					continue
				}

				if ip.String() == pd.LocalIP || entry.TTL == 0 {
					continue
				}

				if pkg.ValidateService(entry.Text) {
					log.Infof("Discovered valid service at IP: %s", ip.String())
					err := pd.AddClientConnection(ip.String(), "50051")
					if err != nil {
						log.Errorf("Failed to add client connection for %s: %v", ip.String(), err)
					}
				} else {
					log.Warnf("Service at IP %s did not advertise the correct service, skipping...", ip.String())
				}
			}
		}
	}(entries)

	err = resolver.Browse(ctx, serviceType, domain, entries)
	if err != nil {
		log.Fatalf("Failed to browse mDNS: %v", err)
	}

	<-ctx.Done()
	log.Warn("Shutting down mDNS discovery...")
	close(entries)
}

func (pd *PeerData) StartPeriodicSync(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(conf.AppConfig.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pd.SyncWithPeers()
		}
	}
}
func (pd *PeerData) HealthCheck(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down periodic metadata exchange...")
			return
		case <-ticker.C:
			for _, conn := range pd.Clients {
				if conn.Target() == pd.LocalIP {
					continue
				}
				stream, err := clients.Ping(conn)
				if err != nil {
					log.Errorf("Failed to ping %s: %v", conn.Target(), err)
					continue
				}

				go func() {
					for {
						_, err := stream.Recv()
						if err != nil {
							log.Errorf("Failed to receive health check response from %s: %v", conn.Target(), err)
							break
						}
						// log.Infof(recv.Message)
					}
				}()
				stream.Send(&pb.Ping{
					Message: fmt.Sprintf("Ping from %v at %v", pd.LocalIP, time.Now().Unix()),
				})
			}
		}
	}
}

func (pd *PeerData) AddClientConnection(ip string, port string) error {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	conn, err := grpc.NewClient(ip+":"+port, grpc.WithInsecure(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}))
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server at %s: %w", ip, err)
	}

	log.Infof("Created connection to: %s", conn.Target())

	for _, c := range pd.Clients {
		if c.Target() == conn.Target() {
			log.Warnf("Connection to %s already exists", conn.Target())
			return nil
		}
	}

	// Store the connection in the map
	pd.Clients = append(pd.Clients, conn)
	log.Infof("Added gRPC client connection to %s", conn.Target())
	return nil
}

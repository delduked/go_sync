package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "go_sync/filesync"
	"go_sync/pkg"

	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
)

type PeerData struct {
	mu          sync.RWMutex
	Clients     []string
	SyncedFiles []string
	WatchDir    string
}

func (pd *PeerData) AddClientConnection(ip string, port string) error {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	conn, err := grpc.NewClient(ip+":"+port, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server at %s: %w", ip, err)
	}

	log.Infof("Created connection to: %s", conn.Target())

	if pkg.ContainsString(pd.Clients, conn.Target()) {
		log.Warnf("Connection to %s already exists, skipping...", ip)
		return nil
	}

	// Store the connection in the map
	pd.Clients = append(pd.Clients, conn.Target())
	log.Infof("Added gRPC client connection to %s", conn.Target())
	return nil
}

func (pd *PeerData) PeriodicCheck(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down periodic check...")
			return
		case <-ticker.C:
			for _, ip := range pd.Clients {
				go func(ip string) {
					conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
					client := pb.NewFileSyncServiceClient(conn)
					stream, err := client.SyncFiles(context.Background())
					if err != nil {
						log.Errorf("Failed to open stream for periodic check on %s: %v", conn.Target(), err)
						return
					}

					poll := fmt.Sprintf("Poll request from server: %s", conn.Target())
					stream.Send(&pb.FileSyncRequest{
						Request: &pb.FileSyncRequest_Poll{
							Poll: &pb.Poll{
								Message: poll,
							}}})

					log.Infof("Sent periodic poll to %s: %s", conn.Target(), poll)
				}(ip)
			}
		}
	}
}

func (pd *PeerData) StartMDNSDiscovery(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	localIP, localSubnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	log.Infof("Local IP: %s, Subnet: %s", localIP, localSubnet)

	instance := fmt.Sprintf("filesync-%s", localIP)
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
			for _, ip := range entry.AddrIPv4 {
				if !pkg.IsInSameSubnet(ip.String(), localSubnet) {
					continue
				}

				if ip.String() == localIP || entry.TTL == 0 {
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

func (pd *PeerData) markFileAsInProgress(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if !pkg.ContainsString(pd.SyncedFiles, fileName) {
		log.Infof("Marking file %s as in progress", fileName)
		pd.SyncedFiles = append(pd.SyncedFiles, fileName)
	} else {
		log.Infof("File %s is already in progress", fileName)
	}
}

func (pd *PeerData) markFileAsComplete(fileName string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	for i, file := range pd.SyncedFiles {
		if file == fileName {
			log.Infof("Marking file %s as complete", fileName)
			pd.SyncedFiles = append(pd.SyncedFiles[:i], pd.SyncedFiles[i+1:]...)
			break
		}
	}
}
func (pd *PeerData) IsFileInProgress(fileName string) bool {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	for _, file := range pd.SyncedFiles {
		if file == fileName {
			return true
		}
	}
	return false
}

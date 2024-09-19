package servers

import (
	"fmt"
	"sync"

	"github.com/TypeTerrors/go_sync/pkg"

	"github.com/charmbracelet/log"
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

// func (pd *PeerData) PeriodicCheck(ctx context.Context, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	ticker := time.NewTicker(20 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Warn("Shutting down periodic check...")
// 			return
// 		case <-ticker.C:
// 			for _, ip := range pd.Clients {
// 				go func(ip string) {
// 					conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
// 					client := pb.NewFileSyncServiceClient(conn)
// 					stream, err := client.SyncFiles(context.Background())
// 					if err != nil {
// 						log.Errorf("Failed to open stream for periodic check on %s: %v", conn.Target(), err)
// 						return
// 					}

// 					poll := fmt.Sprintf("Poll request from server: %s", conn.Target())
// 					stream.Send(&pb.FileSyncRequest{
// 						Request: &pb.FileSyncRequest_Poll{
// 							Poll: &pb.Poll{
// 								Message: poll,
// 							},
// 						},
// 					})

// 					log.Infof("Sent periodic poll to %s: %s", conn.Target(), poll)
// 				}(ip)
// 			}
// 		}
// 	}
// }

package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "go_sync/filesync" // Import your protobufs here

	"github.com/charmbracelet/log" // Bubble Tea log package for colorful logs
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
)

// Shared resource with a map of IPs to gRPC client connections
type SharedData struct {
	mu      sync.RWMutex
	Clients map[string]*grpc.ClientConn // Map of IPs to gRPC client connections
}

// Function to create and store a gRPC client connection
func (sd *SharedData) AddClientConnection(ip string, port string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Check if the IP already exists in the map
	if _, exists := sd.Clients[ip]; exists {
		log.Warnf("Connection to %s already exists, skipping...", ip)
		return nil
	}

	// Create a connection to the gRPC server at the specified IP
	conn, err := grpc.NewClient(ip+":"+port, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server at %s: %w", ip, err)
	}

	// Store the connection in the map
	sd.Clients[ip] = conn
	log.Infof("Added gRPC client connection to %s", ip)
	return nil
}

// Function to handle bidirectional streams for a single connection
func (sd *SharedData) HandleStreamForConnection(ip string, conn *grpc.ClientConn) {
	// Create a new client from the connection
	client := pb.NewFileSyncServiceClient(conn)

	// Create a stream
	stream, err := client.SyncFiles(context.Background())
	if err != nil {
		log.Errorf("Failed to open stream for %s: %v", ip, err)
		return
	}

	// Goroutine to receive responses from the stream
	go func() {
		for {
			response, err := stream.Recv()
			if err != nil {
				log.Errorf("Error receiving response from %s: %v", ip, err)
				return
			}
			log.Infof("Received response from %s: %s", ip, response.Message)

			// Send a success acknowledgment back to the gRPC service
			ack := fmt.Sprintf("Successfully received message from %s", ip)

			err = stream.Send(&pb.FileSyncRequest{
				Request: &pb.FileSyncRequest_Ack{
					Ack: &pb.Acknowledgment{
						Message: ack,
					}}})
			if err != nil {
				log.Errorf("Error sending acknowledgment to %s: %v", ip, err)
				return
			}
			log.Infof("Sent success acknowledgment to %s: %s", ip, ack)
		}
	}()
}

// Periodic check function that sends queries to all connections every 20 seconds
func (sd *SharedData) PeriodicCheck(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down periodic check...")
			return
		case <-ticker.C:
			sd.mu.RLock() // Lock for reading the clients
			for ip, conn := range sd.Clients {
				// Open stream if not already done and send a request
				go func(ip string, conn *grpc.ClientConn) {
					client := pb.NewFileSyncServiceClient(conn)
					stream, err := client.SyncFiles(context.Background())
					if err != nil {
						log.Errorf("Failed to open stream for periodic check on %s: %v", ip, err)
						return
					}

					poll := fmt.Sprintf("Poll request from server: %s", ip)
					stream.Send(&pb.FileSyncRequest{
						Request: &pb.FileSyncRequest_Poll{
							Poll: &pb.Poll{
								Message: poll,
							}}})
					if err != nil {
						log.Errorf("Error sending poll to %s: %v", ip, err)
						return
					}
					log.Infof("Sent periodic poll to %s: %s", ip, poll)
				}(ip, conn)
			}
			sd.mu.RUnlock() // Unlock after sending queries
		}
	}
}

// mDNS service that automatically updates the map with discovered gRPC services
func (sd *SharedData) StartMDNSDiscovery(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatal("Failed to initialize mDNS resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			// If an IPv4 address is found, add it to the Clients map
			if len(entry.AddrIPv4) > 0 {
				ip := entry.AddrIPv4[0].String()
				log.Infof("Discovered service at IP: %s", ip)

				// Try to add the discovered client connection (port can be customized)
				err := sd.AddClientConnection(ip, "50051")
				if err != nil {
					log.Errorf("Failed to add client connection for %s: %v", ip, err)
				}
			}
		}
	}(entries)

	go func() {
		// Handle context cancellation
		<-ctx.Done()
		log.Warn("Shutting down mDNS discovery...")
		close(entries)
	}()

	// Browse for services in the "_grpc._tcp" domain
	err = resolver.Browse(context.Background(), "_grpc._tcp", "local.", entries)
	if err != nil {
		log.Fatal("Failed to browse mDNS: %v", err)
	}
}

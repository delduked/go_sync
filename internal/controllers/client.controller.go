package controllers

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	pb "go_sync/filesync" // Import your protobufs here
	"go_sync/pkg"

	"github.com/charmbracelet/log" // Bubble Tea log package for colorful logs
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// Shared resource with a map of IPs to gRPC client connections
type SharedData struct {
	mu          sync.RWMutex
	Clients     map[string]*grpc.ClientConn // Map of IPs to gRPC client connections
	SyncedFiles map[string]struct{}         // Tracks files currently being synchronized
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

// StartMDNSDiscovery discovers gRPC services via mDNS and excludes its own IP
func (sd *SharedData) StartMDNSDiscovery(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// Find local IP address to exclude from discovery and calculate subnet
	localIP, localSubnet, err := pkg.GetLocalIPAndSubnet()
	if err != nil {
		log.Fatalf("Failed to get local IP and subnet: %v", err)
	}

	log.Infof("Local IP: %s, Subnet: %s", localIP, localSubnet)

	// Register the service to make itself discoverable
	instance := fmt.Sprintf("filesync-%s", localIP)
	serviceType := "_myapp_filesync._tcp" // Custom service type
	domain := "local."
	txtRecords := []string{"version=1.0", "service_id=my_unique_service"}

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

	// Channel to receive discovered entries
	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			// Check if the discovered IP is in the same subnet
			for _, ip := range entry.AddrIPv4 {
				if !pkg.IsInSameSubnet(ip.String(), localSubnet) {
					log.Warnf("Skipping service at IP %s, outside the local subnet", ip.String())
					continue
				}

				// Skip if it's the local IP or TTL == 0 (Goodbye message)
				if ip.String() == localIP || entry.TTL == 0 {
					continue
				}

				// Validate service by its TXT records
				if pkg.ValidateService(entry.Text) {
					log.Infof("Discovered valid service at IP: %s", ip.String())
					err := sd.AddClientConnection(ip.String(), "50051")
					if err != nil {
						log.Errorf("Failed to add client connection for %s: %v", ip.String(), err)
					}
				} else {
					log.Warnf("Service at IP %s did not advertise the correct service, skipping...", ip.String())
				}
			}
		}
	}(entries)

	// Start mDNS browsing for other services
	err = resolver.Browse(ctx, serviceType, domain, entries)
	if err != nil {
		log.Fatalf("Failed to browse mDNS: %v", err)
	}

	// Handle context cancellation for graceful shutdown
	<-ctx.Done()
	log.Warn("Shutting down mDNS discovery...")
	close(entries)
}

// verifyPeer tries to establish a gRPC connection and ping the discovered peer
func (sd *SharedData) verifyPeer(ip, port string) bool {
	// Dial the discovered gRPC server
	conn, err := grpc.NewClient(ip+":"+port, grpc.WithInsecure(), grpc.WithTimeout(3*time.Second))
	if err != nil {
		log.Errorf("Failed to dial %s: %v", ip, err)
		return false
	}
	defer conn.Close()

	// Check connection state
	if conn.GetState() != connectivity.Ready {
		// log.Warnf("Connection to %s is not ready", ip)
		return false
	}

	// Create a new client and send a ping request
	client := pb.NewFileSyncServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.SyncFiles(ctx)
	if err != nil {
		log.Warnf("Ping to %s failed: %v", ip, err)
		return false
	}

	stream.Send(&pb.FileSyncRequest{
		Request: &pb.FileSyncRequest_Poll{
			Poll: &pb.Poll{
				Message: "Verifying peer",
			},
		},
	})

	log.Infof("Successfully verified peer at %s", ip)
	return true
}

// RemoveClientConnection removes a gRPC client connection by IP and closes it
func (sd *SharedData) RemoveClientConnection(ip string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Check if the IP exists in the map
	conn, exists := sd.Clients[ip]
	if !exists {
		log.Warnf("Connection to %s does not exist, cannot remove", ip)
		return nil
	}

	// Close the connection
	err := conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close connection to %s: %v", ip, err)
	}

	// Remove the connection from the map
	delete(sd.Clients, ip)
	log.Infof("Removed gRPC client connection to %s", ip)
	return nil
}

// Function to mark a file as being synchronized
func (sd *SharedData) markFileAsInProgress(fileName string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	log.Infof("Marking file %s as in progress", fileName)
	sd.SyncedFiles[fileName] = struct{}{}
}

// Function to mark a file synchronization as complete
func (sd *SharedData) markFileAsComplete(fileName string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	log.Infof("Marking file %s as complete", fileName)
	delete(sd.SyncedFiles, fileName)
}

// getLocalIP retrieves the local IP address from the network interfaces
func getLocalIP() (string, error) {
	// Get a list of all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("unable to get network interfaces: %w", err)
	}

	// Loop through each interface
	for _, iface := range interfaces {
		// Skip down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Get addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			return "", fmt.Errorf("unable to get addresses for interface %s: %w", iface.Name, err)
		}

		// Loop through the addresses and find the first valid IPv4 address
		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Ensure this is a valid global unicast address
			if ip != nil && ip.IsGlobalUnicast() && ip.To4() != nil {
				return ip.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no valid local IP address found")
}

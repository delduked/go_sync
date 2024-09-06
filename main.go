package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pb "go_sync/filesync"

	"github.com/fsnotify/fsnotify"
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
)

var (
	activeTransfers = make(map[string]bool)
	mu              sync.Mutex
	peers           = make(map[string]string) // To store discovered peers (IP:port)
)

type server struct {
	pb.UnimplementedFileSyncServer
	localFolder string
}

func (s *server) SyncFile(stream pb.FileSync_SyncFileServer) error {
	if p, ok := peer.FromContext(stream.Context()); ok {
		log.Printf("Server: Connection established from %s", p.Addr)
	}
	defer log.Println("Server: Connection closed.")

	var filePath string
	var file *os.File

	defer func() {
		if file != nil {
			file.Close()
		}
		log.Println("Server: Finished receiving file stream.")
	}()

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			log.Println("Server: Reached end of file stream.")
			break
		}
		if err != nil {
			log.Printf("Server: Error receiving chunk: %v\n", err)
			return err
		}

		if filePath == "" {
			filePath = filepath.Join(s.localFolder, chunk.Filename)
			file, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Printf("Server: Error opening file for writing: %v\n", err)
				return err
			}

			mu.Lock()
			if activeTransfers[chunk.Filename] {
				mu.Unlock()
				return nil // Skip if this file is already being transferred
			}
			activeTransfers[chunk.Filename] = true
			mu.Unlock()
		}

		if _, err := file.Write(chunk.Content); err != nil {
			log.Printf("Server: Error writing chunk to file: %v\n", err)
			return err
		}

		if err := stream.Send(&pb.SyncResponse{Message: "Chunk received"}); err != nil {
			log.Printf("Server: Error sending acknowledgment: %v\n", err)
			return err
		}
	}

	mu.Lock()
	delete(activeTransfers, filepath.Base(filePath))
	mu.Unlock()

	return nil
}

func (s *server) DeleteFile(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	log.Printf("Server: Deleting file or folder: %s", req.Filename)
	err := os.RemoveAll(req.Filename)
	if err != nil {
		log.Printf("Server: Error deleting file or folder: %v", err)
		return &pb.DeleteResponse{Message: "Error deleting file or folder"}, err
	}

	log.Printf("Server: Successfully deleted file or folder: %s", req.Filename)
	return &pb.DeleteResponse{Message: "File or folder deleted"}, nil
}

func getHostIP() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		// Ignore interfaces that are down, loopback, or Docker-related (veth)
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(iface.Name, "veth") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Return the first valid IPv4 address
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("no valid IPv4 address found")
}

func discoverPeersPeriodically() {
	// Get the local machine's IP address
	localIP, err := getHostIP()
	if err != nil {
		log.Fatalf("Failed to get local IP address: %v", err)
	}

	// Channel to receive discovered services
	entries := make(chan *zeroconf.ServiceEntry)

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			// Skip if the discovered service is from the local machine
			if entry.AddrIPv4[0].String() == localIP.String() {
				continue
			}

			remoteAddr := fmt.Sprintf("%s:%d", entry.AddrIPv4[0], entry.Port)
			mu.Lock()
			peers[entry.AddrIPv4[0].String()] = remoteAddr
			mu.Unlock()
			log.Printf("Discovered peer: %s", remoteAddr)
		}
	}(entries)

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalf("Failed to initialize resolver: %v", err)
	}

	for {
		// No timeout: keep browsing for services continuously
		err := resolver.Browse(context.Background(), "_filesync._tcp", "local.", entries)
		if err != nil {
			log.Fatalf("Failed to browse services: %v", err)
		}

		time.Sleep(5 * time.Second) // Pause before the next discovery round
	}
}

func getAnyPeer() (string, error) {
	mu.Lock()
	defer mu.Unlock()

	for _, addr := range peers {
		return addr, nil
	}
	return "", fmt.Errorf("no peers found")
}

func startServer(port, localFolder string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterFileSyncServer(s, &server{localFolder: localFolder})

	// Register service via mDNS using zeroconf
	server, err := zeroconf.Register(
		"gRPC File Sync Service",        // Service Name
		"_filesync._tcp",                // Service Type
		"local.",                        // Domain
		50051,                           // Port
		[]string{"txtvers=1", "path=/"}, // Text records
		nil,                             // Interface
	)
	if err != nil {
		log.Fatalf("Failed to register service: %v", err)
	}
	defer server.Shutdown()

	log.Printf("Server is running on port %s, syncing folder: %s", port, localFolder)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func startClient(filePath string, isDelete bool) {
	remoteAddr, err := getAnyPeer() // Get any discovered peer
	if err != nil {
		log.Println("Client: No peers available for syncing")
		return
	}

	if isDelete {
		deleteFileOnRemote(remoteAddr, filePath)
		return
	}

	log.Printf("Client: Preparing to sync file %s to %s", filePath, remoteAddr)

	mu.Lock()
	if activeTransfers[filepath.Base(filePath)] {
		mu.Unlock()
		return
	}
	activeTransfers[filepath.Base(filePath)] = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(activeTransfers, filepath.Base(filePath))
		mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	conn, err := grpc.Dial(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Client: Failed to connect to %s: %v", remoteAddr, err)
	}
	defer conn.Close()

	client := pb.NewFileSyncClient(conn)
	stream, err := client.SyncFile(ctx)
	if err != nil {
		log.Fatalf("Client: Failed to sync file: %v", err)
	}

	if err := streamFileInRealTime(stream, filePath); err != nil {
		log.Fatalf("Client: Failed to sync file: %v", err)
	}

	log.Printf("Client: Finished syncing file %s", filePath)
}

func deleteFileOnRemote(remoteAddr, filePath string) {
	log.Printf("Client: Preparing to delete file or folder %s on remote %s", filePath, remoteAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	conn, err := grpc.Dial(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Client: Failed to connect to %s: %v", remoteAddr, err)
	}
	defer conn.Close()

	client := pb.NewFileSyncClient(conn)
	_, err = client.DeleteFile(ctx, &pb.DeleteRequest{Filename: filePath})
	if err != nil {
		log.Fatalf("Client: Failed to delete file or folder: %v", err)
	}

	log.Printf("Client: Successfully deleted file or folder %s on remote %s", filePath, remoteAddr)
}

func streamFileInRealTime(stream pb.FileSync_SyncFileClient, filePath string) error {
	log.Printf("Client: Started streaming file %s\n", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Client: Error opening file: %v\n", err)
		return err
	}
	defer file.Close()

	buffer := make([]byte, 32*1024) // 32 KB buffer size
	filename := filepath.Base(filePath)

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if err := stream.Send(&pb.FileChunk{
				Filename: filename,
				Content:  buffer[:n],
			}); err != nil {
				log.Printf("Client: Error sending chunk: %v\n", err)
				return err
			}

			_, err := stream.Recv()
			if err != nil && err != io.EOF {
				log.Printf("Client: Error receiving acknowledgment: %v\n", err)
				return err
			}
		}
		if err == io.EOF {
			log.Println("Client: Reached end of file.")
			break
		}
		if err != nil && err != io.EOF {
			log.Printf("Client: Error reading file: %v\n", err)
			return err
		}
	}

	if err := stream.CloseSend(); err != nil {
		log.Printf("Client: Error closing stream: %v\n", err)
		return err
	}

	log.Printf("Client: Finished streaming file %s\n", filePath)
	return nil
}

func watchFolderForRealTimeSync(folderPath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	err = watcher.Add(folderPath)
	if err != nil {
		log.Fatalf("failed to add folder to watcher: %v", err)
	}

	log.Printf("Watching folder: %s", folderPath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Handle file/folder creations and modifications
			if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				mu.Lock()
				if activeTransfers[filepath.Base(event.Name)] {
					mu.Unlock()
					continue
				}
				mu.Unlock()

				log.Printf("Detected file change: %s", event.Name)
				go startClient(event.Name, false) // Sync the file to remote system
			}

			// Handle file/folder deletions and moves (rename)
			if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				log.Printf("Detected file/folder removal or move to trash: %s", event.Name)
				go startClient(event.Name, true) // Delete the file on the remote system
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func main() {
	localFolder := flag.String("local", "", "Local folder to watch for file changes")
	port := flag.String("port", "50051", "Port on which the gRPC server will run")
	flag.Parse()

	if *localFolder == "" {
		log.Fatalf("Flag -local must be provided")
	}

	go startServer(*port, *localFolder)

	// Start peer discovery in the background
	go discoverPeersPeriodically()

	time.Sleep(2 * time.Second)

	watchFolderForRealTimeSync(*localFolder)
}

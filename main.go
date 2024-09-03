package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "go_sync/filesync"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
)

var activeTransfers = make(map[string]bool)
var mu sync.Mutex

type server struct {
	pb.UnimplementedFileSyncServer
	localFolder string
}

func (s *server) SyncFile(stream pb.FileSync_SyncFileServer) error {
	// Log when a connection is made
	if p, ok := peer.FromContext(stream.Context()); ok {
		log.Printf("Server: Connection established from %s", p.Addr)
	}
	defer func() {
		log.Println("Server: Connection closed.")
	}()

	log.Println("Server: Started receiving file stream.")
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
				// log.Printf("Server: File %s is already being transferred, skipping...", chunk.Filename)
				mu.Unlock()
				return nil // Skip if this file is already being transferred
			}
			activeTransfers[chunk.Filename] = true
			mu.Unlock()
		}

		// log.Printf("Server: Receiving chunk for file %s", chunk.Filename)

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

func startServer(port, localFolder string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterFileSyncServer(s, &server{localFolder: localFolder})

	log.Printf("Server is running on port %s, syncing folder: %s", port, localFolder)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func startClient(remoteAddr, filePath string) {
	log.Printf("Client: Preparing to sync file %s to %s", filePath, remoteAddr)

	// Mark the file as being transferred before starting the stream
	mu.Lock()
	if activeTransfers[filepath.Base(filePath)] {
		// log.Printf("Client: File %s is already being transferred, skipping...", filePath)
		mu.Unlock()
		return // Skip if this file is already being transferred
	}
	activeTransfers[filepath.Base(filePath)] = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(activeTransfers, filepath.Base(filePath))
		mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // Increase timeout for large files
	defer cancel()

	log.Printf("Client: Attempting to connect to %s", remoteAddr)
	conn, err := grpc.Dial(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Client: Failed to connect to %s: %v", remoteAddr, err)
	}
	defer func() {
		log.Println("Client: Connection closed.")
		conn.Close()
	}()

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
			// log.Printf("Client: Sending chunk of size %d for file %s\n", n, filename)
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
			// log.Printf("Client: Server acknowledgment: %s\n", res.Message)
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

	// Close the stream
	if err := stream.CloseSend(); err != nil {
		log.Printf("Client: Error closing stream: %v\n", err)
		return err
	}

	log.Printf("Client: Finished streaming file %s\n", filePath)
	return nil
}

func watchFolderForRealTimeSync(folderPath, remoteAddr string) {
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
			if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				mu.Lock()
				if activeTransfers[filepath.Base(event.Name)] {
					// log.Printf("File %s is already being transferred, skipping...", event.Name)
					mu.Unlock()
					continue
				}
				mu.Unlock()

				log.Printf("Detected file change: %s", event.Name)
				go startClient(remoteAddr, event.Name)
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
	// Parse command-line arguments
	localFolder := flag.String("local", "", "Local folder to watch for file changes")
	remoteAddr := flag.String("remoteAddr", "", "Address of the remote system")
	port := flag.String("port", "50051", "Port on which the gRPC server will run")
	flag.Parse()

	if *localFolder == "" || *remoteAddr == "" {
		log.Fatalf("Both flags -local and -remoteAddr must be provided")
	}

	// Start the gRPC server
	go startServer(*port, *localFolder)

	// Delay to ensure the server is up before starting the watcher
	time.Sleep(2 * time.Second)

	// Start watching the local folder for new files
	watchFolderForRealTimeSync(*localFolder, *remoteAddr)
}

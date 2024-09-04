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

// SyncFile is the function to sync files
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

	// Check if the file or folder exists before attempting to delete
	if _, err := os.Stat(req.Filename); os.IsNotExist(err) {
		log.Printf("Server: File or folder does not exist: %s", req.Filename)
		return &pb.DeleteResponse{Message: "File or folder does not exist"}, nil
	}

	// Attempt to delete the file or folder
	err := os.RemoveAll(req.Filename)
	if err != nil {
		log.Printf("Server: Error deleting file or folder: %v. Possible permission issue", err)
		return &pb.DeleteResponse{Message: "Error deleting file or folder"}, err
	}

	log.Printf("Server: Successfully deleted file or folder: %s", req.Filename)
	return &pb.DeleteResponse{Message: "File or folder deleted"}, nil
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

func startClient(remoteAddr, filePath string, isDelete bool) {
	if isDelete {
		deleteFileOnRemote(remoteAddr, filePath)
		return
	}

	log.Printf("Client: Preparing to sync file %s to %s", filePath, remoteAddr)

	// Mark the file as being transferred before starting the stream
	mu.Lock()
	if activeTransfers[filepath.Base(filePath)] {
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

			// Handle file/folder creations and modifications
			if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				mu.Lock()
				if activeTransfers[filepath.Base(event.Name)] {
					mu.Unlock()
					continue
				}
				mu.Unlock()

				log.Printf("Detected file change: %s", event.Name)
				go startClient(remoteAddr, event.Name, false) // Sync the file to remote system
			}

			// Handle file/folder deletions and moves (rename)
			if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				log.Printf("Detected file/folder removal or move to trash: %s", event.Name)
				go startClient(remoteAddr, event.Name, true) // Delete the file on the remote system
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
	remoteAddr := flag.String("remoteAddr", "", "Address of the remote system")
	port := flag.String("port", "50051", "Port on which the gRPC server will run")
	flag.Parse()

	if *localFolder == "" || *remoteAddr == "" {
		log.Fatalf("Both flags -local and -remoteAddr must be provided")
	}

	go startServer(*port, *localFolder)

	time.Sleep(2 * time.Second)

	watchFolderForRealTimeSync(*localFolder, *remoteAddr)
}

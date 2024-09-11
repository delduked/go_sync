package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	pb "go_sync/filesync" 

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
)

// SyncServer holds the configuration for the synchronization server
type SyncServer struct {
	grpcServer *grpc.Server
	listener   net.Listener
	watchDir   string
	port       string
	sharedData *SharedData
}

// Configuration
const chunkSize = 1024 // Size of file chunks for streaming

// NewSyncServer creates a new SyncServer with default settings
func NewSyncServer(watchDir, port string) (*SyncServer, error) {
	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %v", port, err)
	}

	// Initialize SyncServer
	server := &SyncServer{
		grpcServer: grpc.NewServer(),
		listener:   listener,
		watchDir:   watchDir,
		port:       port,
		sharedData: &SharedData{Clients: make(map[string]*grpc.ClientConn), SyncedFiles: make(map[string]struct{})}, // Add map to track synced files
	}

	return server, nil
}

// Start starts the gRPC server and file watcher
func (s *SyncServer) Start(wg *sync.WaitGroup) error {
	defer wg.Done()

	// Start gRPC server in a goroutine
	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, &FileSyncServer{})
		if err := s.grpcServer.Serve(s.listener); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()

	// Start file watcher
	_, err := s.watchDirectory()
	if err != nil {
		return fmt.Errorf("failed to start directory watcher: %v", err)
	}

	return nil
}

// WatchDirectory monitors the directory for file system events (create, modify, delete, rename)
func (s *SyncServer) watchDirectory() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	err = watcher.Add(s.watchDir)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Check if this file is already in progress (to avoid re-sending)
				if _, inProgress := s.sharedData.SyncedFiles[event.Name]; !inProgress {
					s.handleFileEvent(event)
				} else {
					log.Printf("Skipping file %s, already in sync", event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error:", err)
			}
		}
	}()

	return watcher, nil
}

// Handle file events such as create, modify, delete, rename
func (s *SyncServer) handleFileEvent(event fsnotify.Event) {
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		log.Println("File created:", event.Name)
		s.sharedData.markFileAsInProgress(event.Name) // Mark file as in progress
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Write == fsnotify.Write:
		log.Println("File modified:", event.Name)
		s.sharedData.markFileAsInProgress(event.Name) // Mark file as in progress
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		log.Println("File deleted:", event.Name)
		s.propagateDelete(event.Name)
		s.sharedData.markFileAsComplete(event.Name) // Mark file as complete
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		log.Println("File renamed:", event.Name)
		s.propagateRename(event.Name)
		s.sharedData.markFileAsComplete(event.Name) // Mark file as complete
	}
}

// Start streaming a file to all peers
func (s *SyncServer) startStreamingFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Println("Error opening file:", err)
		s.sharedData.markFileAsComplete(filePath) // Mark file as complete in case of error
		return
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			log.Println("Error reading file:", err)
			s.sharedData.markFileAsComplete(filePath) // Mark file as complete in case of error
			return
		}
		if n == 0 {
			break
		}
		s.sendFileChunkToPeers(filePath, buffer[:n])
	}
	s.sharedData.markFileAsComplete(filePath) // Mark file as complete when finished
}

// Send file chunk to peers using gRPC stream
func (s *SyncServer) sendFileChunkToPeers(fileName string, chunk []byte) {
	s.sharedData.mu.RLock()
	defer s.sharedData.mu.RUnlock()

	for peer, conn := range s.sharedData.Clients {
		client := pb.NewFileSyncServiceClient(conn)
		stream, err := client.SyncFiles(context.Background())
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", peer, err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:  fileName,
					ChunkData: chunk,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending chunk to peer %s: %v", peer, err)
		}
	}
}

// Handle file delete event
func (s *SyncServer) propagateDelete(fileName string) {
	s.sharedData.mu.RLock()
	defer s.sharedData.mu.RUnlock()

	for peer, conn := range s.sharedData.Clients {
		client := pb.NewFileSyncServiceClient(conn)
		stream, err := client.SyncFiles(context.Background())
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", peer, err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{FileName: fileName},
			},
		})
		if err != nil {
			log.Printf("Error sending delete request to peer %s: %v", peer, err)
		}
	}
}

// Handle file rename event
func (s *SyncServer) propagateRename(fileName string) {
	// Implement renaming logic, similar to propagateDelete
}

// Save file chunk to the local directory
func (s *SyncServer) saveFileChunk(chunk *pb.FileChunk) error {
	path := filepath.Join(s.watchDir, chunk.FileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(chunk.ChunkData)
	return err
}

// Delete a file from the local directory
func (s *SyncServer) deleteFile(fileName string) error {
	path := filepath.Join(s.watchDir, fileName)
	return os.Remove(path)
}

// Rename a file in the local directory
func (s *SyncServer) renameFile(oldName, newName string) error {
	oldPath := filepath.Join(s.watchDir, oldName)
	newPath := filepath.Join(s.watchDir, newName)
	return os.Rename(oldPath, newPath)
}

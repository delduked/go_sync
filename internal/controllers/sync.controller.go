package controllers

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "go_sync/filesync"
	"go_sync/pkg"

	"github.com/charmbracelet/log" // Bubble Tea log package for colorful logs
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
func (s *SyncServer) Start(wg *sync.WaitGroup, ctx context.Context) error {
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

	wg.Add(1)
	go s.syncMissingFiles(ctx, wg)

	return nil
}

// WatchDirectory monitors the directory for file system events (create, modify, delete, rename)
func (s *SyncServer) watchDirectory() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log.Printf("Watching directory: %s", s.watchDir)
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
				log.Print("Error:", err)
			}
		}
	}()

	return watcher, nil
}

// Handle file events such as create, modify, delete, rename
func (s *SyncServer) handleFileEvent(event fsnotify.Event) {
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		log.Print("File created:", event.Name)
		s.sharedData.markFileAsInProgress(event.Name) // Mark file as in progress
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Write == fsnotify.Write:
		log.Print("File modified:", event.Name)
		s.sharedData.markFileAsInProgress(event.Name) // Mark file as in progress
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		log.Print("File deleted:", event.Name)
		s.propagateDelete(event.Name)
		s.sharedData.markFileAsComplete(event.Name) // Mark file as complete
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		log.Print("File renamed:", event.Name)
		s.propagateRename(event.Name)
		s.sharedData.markFileAsComplete(event.Name) // Mark file as complete
	}
}

// Start streaming a file to all peers
func (s *SyncServer) startStreamingFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Print("Error opening file:", err)
		s.sharedData.markFileAsComplete(filePath) // Mark file as complete in case of error
		return
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			log.Print("Error reading file:", err)
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

func (s *SyncServer) syncMissingFiles(ctx context.Context, wg *sync.WaitGroup) {
    defer wg.Done()

    ticker := time.NewTicker(20 * time.Second) // Adjust the sync interval if needed
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            log.Warn("Shutting down list check...")
            return
        case <-ticker.C:

            localFiles, err := pkg.GetFileList()
            if err != nil {
                log.Errorf("Failed to get local file list: %v", err)
                return
            }

            if len(localFiles) == 0 {
                log.Warn("No files to sync")
                return
            } else {
                log.Infof("Local files: %v", localFiles)
            }

            s.sharedData.mu.RLock() // Lock for reading the clients
            for ip, conn := range s.sharedData.Clients {
                go func(ip string, conn *grpc.ClientConn) {
                    client := pb.NewFileSyncServiceClient(conn)
                    stream, err := client.SyncFiles(context.Background())
                    if err != nil {
                        log.Errorf("Failed to open stream for list check on %s: %v", ip, err)
                        return
                    }

                    log.Infof("Opened stream with %s to check missing files", ip)

                    // Send local files to peer
                    err = stream.Send(&pb.FileSyncRequest{
                        Request: &pb.FileSyncRequest_FileList{
                            FileList: &pb.FileList{
                                Files: localFiles,
                            }},
                    })
                    if err != nil {
                        log.Errorf("Error sending list to %s: %v", ip, err)
                        return
                    }
                    log.Infof("Sent list to %s: %v", ip, localFiles)

                    // Receive response from peer (files they are missing)
                    for {
                        response, err := stream.Recv()
                        if err == io.EOF {
                            log.Warnf("Stream closed by %s", ip)
                            break
                        }
                        if err != nil {
                            log.Errorf("Error receiving response from %s: %v", ip, err)
                            break
                        }

                        if response.Filestosend != nil && len(response.Filestosend) > 0 {
                            log.Infof("Peer %s is missing files: %v", ip, response.Filestosend)

                            for _, file := range response.Filestosend {
                                s.sharedData.markFileAsInProgress(file)
                                log.Infof("Sending file %s to peer %s", file, ip)
                                s.startStreamingFile(file)
                                s.sharedData.markFileAsComplete(file)
                            }
                        }
                    }
                }(ip, conn)
            }
            s.sharedData.mu.RUnlock() // Unlock after sending queries
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

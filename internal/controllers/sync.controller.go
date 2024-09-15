package controllers

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
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
// 32 KB buffer size for file chunks
const chunkSize = 32 * 1024

// NewSyncServer creates a new SyncServer with default settings
func NewSyncServer(sharedData *SharedData, watchDir, port string) (*SyncServer, error) {
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
		sharedData: sharedData,
	}

	return server, nil
}

// Start starts the gRPC server and file watcher
func (s *SyncServer) Start(wg *sync.WaitGroup, ctx context.Context, sd *SharedData) error {
	defer wg.Done()

	// Start gRPC server in a goroutine
	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, &FileSyncServer{
			SharedData: sd,
		})
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
				// Ignore events for files in SyncedFiles
				if !pkg.ContainsString(s.sharedData.SyncedFiles, event.Name) {
					s.handleFileEvent(event)
				} else {
					log.Printf("Ignoring event for %s; file is currently being synchronized", event.Name)
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
	s.sharedData.mu.Lock()
	// Add file to SyncedFiles to mark it as being synchronized
	if pkg.ContainsString(s.sharedData.SyncedFiles, event.Name) {
		s.sharedData.mu.Unlock()
		return
	}
	s.sharedData.SyncedFiles = append(s.sharedData.SyncedFiles, event.Name)
	s.sharedData.mu.Unlock()

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		log.Printf("File created: %s", event.Name)
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Write == fsnotify.Write:
		log.Printf("File modified: %s", event.Name)
		s.startStreamingFile(event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		log.Printf("File deleted: %s", event.Name)
		s.propagateDelete(event.Name)
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		log.Printf("File renamed: %s", event.Name)
		s.propagateRename(event.Name)
	}

	// After processing, remove the file from SyncedFiles
	s.sharedData.mu.Lock()
	s.sharedData.markFileAsComplete(event.Name)
	s.sharedData.mu.Unlock()
}

// Start streaming a file to all peers
func (s *SyncServer) startStreamingFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Print("Error opening file:", err)
		s.sharedData.markFileAsComplete(filePath)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Print("Error getting file info:", err)
		s.sharedData.markFileAsComplete(filePath)
		return
	}
	fileSize := fileInfo.Size()
	chunkSize := int64(32 * 1024)
	totalChunks := int((fileSize + chunkSize - 1) / chunkSize)
	log.Printf("File size: %d bytes, Chunk size: %d bytes, Total chunks: %d", fileSize, chunkSize, totalChunks)

	chunks := make([][]byte, 0, totalChunks)
	buffer := make([]byte, chunkSize)

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			log.Print("Error reading file:", err)
			s.sharedData.markFileAsComplete(filePath)
			return
		}
		if n == 0 {
			break
		}
		chunkData := make([]byte, n)
		copy(chunkData, buffer[:n])
		chunks = append(chunks, chunkData)
	}

	s.sendFileChunkToPeers(filePath, chunks, totalChunks)
	s.sharedData.markFileAsComplete(filePath)
}

// Send file chunk to peers using gRPC stream
func (s *SyncServer) sendFileChunkToPeers(fileName string, chunks [][]byte, totalChunks int) {
	s.sharedData.mu.RLock()
	clients := make([]string, len(s.sharedData.Clients))
	copy(clients, s.sharedData.Clients)
	s.sharedData.mu.RUnlock()

	for _, ip := range clients {
		conn, err := grpc.Dial(ip, grpc.WithInsecure())
		if err != nil {
			log.Printf("Failed to connect to gRPC server at %s: %v", ip, err)
			continue
		}
		defer conn.Close()

		client := pb.NewFileSyncServiceClient(conn)
		stream, err := client.SyncFiles(context.Background())
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", ip, err)
			continue
		}

		for chunkNumber, chunk := range chunks {
			log.Printf("Sending chunk %d/%d of file %s to peer %s", chunkNumber+1, totalChunks, fileName, ip)
			err = stream.Send(&pb.FileSyncRequest{
				Request: &pb.FileSyncRequest_FileChunk{
					FileChunk: &pb.FileChunk{
						FileName:    fileName,
						ChunkData:   chunk,
						ChunkNumber: int32(chunkNumber + 1),
						TotalChunks: int32(totalChunks),
					},
				},
			})
			if err != nil {
				log.Printf("Error sending chunk to peer %s: %v", ip, err)
				break
			}
		}

		// Close the stream after sending all chunks
		err = stream.CloseSend()
		if err != nil {
			log.Printf("Error closing stream to peer %s: %v", ip, err)
		}
	}
}

func (s *SyncServer) syncMissingFiles(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down list check...")
			return
		case <-ticker.C:
			log.Info("Starting syncMissingFiles routine...")

			localFiles, err := pkg.GetFileList()
			if err != nil {
				log.Errorf("Failed to get local file list: %v", err)
				return
			}

			log.Infof("Local files found: %v", localFiles)

			// Check if there are any clients connected
			log.Infof("Number of connected clients: %d", len(s.sharedData.Clients))
			for _, conn := range s.sharedData.Clients {
				log.Infof("Client found: %s", conn)
			}

			if len(s.sharedData.Clients) != 0 {
				log.Info("Starting list check with peers...")
				for _, ip := range s.sharedData.Clients {
					go func(ip string) {
						log.Infof("Checking missing files with %s", ip)
						conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
						if err != nil {
							log.Errorf("failed to connect to gRPC server at %v: %v", ip, err)
							return
						}
						client := pb.NewFileSyncServiceClient(conn)
						stream, err := client.SyncFiles(context.Background())
						if err != nil {
							log.Errorf("Failed to open stream for list check on %s: %v", conn.Target(), err)
							return
						}

						log.Infof("Opened stream with %s to check missing files", conn.Target())

						go func() {
							// Receive response from peer (files they are missing)
							for {
								response, err := stream.Recv()
								if err == io.EOF {
									log.Warnf("Stream closed by %s", conn.Target())
									break
								}
								if err != nil {
									log.Errorf("Error receiving response from %s: %v", conn.Target(), err)
									break
								}
						
								if len(response.Filestosend) != 0 {
									log.Infof("Peer %s is missing files: %v", conn.Target(), response.Filestosend)
						
									for _, file := range response.Filestosend {
										s.sharedData.markFileAsInProgress(file)
						
										log.Infof("Sending file %s to peer %s", file, conn.Target())
										go s.startStreamingFile(file)
						
										s.sharedData.markFileAsComplete(file)
									}
								} else {
									log.Infof("Peer %s is currently in sync. No files to sync", conn.Target())
								}
							}
						}()
						
						// Send local files to peer
						err = stream.Send(&pb.FileSyncRequest{
							Request: &pb.FileSyncRequest_FileList{
								FileList: &pb.FileList{
									Files: localFiles,
								}},
						})
						if err != nil {
							log.Errorf("Error sending list to %s: %v", conn.Target(), err)
							return
						}
						log.Infof("Sent list to %s: %v", conn.Target(), localFiles)
						
						// Ensure stream closure after sending
						// err = stream.CloseSend()
						// if err != nil {
						// 	log.Errorf("Error closing stream: %v", err)
						// }

					}(ip)
				}
			} else {
				log.Warn("No connected clients to sync with")
			}
		}
	}
}

// Handle file delete event
func (s *SyncServer) propagateDelete(fileName string) {

	for peer, conn := range s.sharedData.Clients {
		conn, err := grpc.NewClient(conn, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Printf("failed to connect to gRPC server at %s: %v", conn.Target(), err)
			continue
		}
		client := pb.NewFileSyncServiceClient(conn)
		stream, err := client.SyncFiles(context.Background())
		if err != nil {
			log.Printf("Error starting stream to peer %v: %v", peer, err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{FileName: fileName},
			},
		})
		if err != nil {
			log.Printf("Error sending delete request to peer %v: %v", peer, err)
		}
	}
}

// Handle file rename event
func (s *SyncServer) propagateRename(fileName string) {
	// Implement renaming logic, similar to propagateDelete
}

// // Save file chunk to the local directory
// func (s *SyncServer) saveFileChunk(chunk *pb.FileChunk) error {
// 	path := filepath.Join(s.watchDir, chunk.FileName)
// 	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	if err != nil {
// 		return err
// 	}
// 	defer file.Close()

// 	_, err = file.Write(chunk.ChunkData)
// 	return err
// }

// // Delete a file from the local directory
// func (s *SyncServer) deleteFile(fileName string) error {
// 	path := filepath.Join(s.watchDir, fileName)
// 	return os.Remove(path)
// }

// // Rename a file in the local directory
// func (s *SyncServer) renameFile(oldName, newName string) error {
// 	oldPath := filepath.Join(s.watchDir, oldName)
// 	newPath := filepath.Join(s.watchDir, newName)
// 	return os.Rename(oldPath, newPath)
// }

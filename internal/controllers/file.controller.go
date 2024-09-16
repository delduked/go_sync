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
	"go_sync/internal/clients"
	"go_sync/pkg"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
)

type State struct {
	grpcServer *grpc.Server
	listener   net.Listener
	watchDir   string
	port       string
	sharedData *PeerData
}

// NewState creates a new State with default settings
func StateServer(sharedData *PeerData, watchDir, port string) (*State, error) {
	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %v", port, err)
	}

	// Initialize State
	server := &State{
		grpcServer: grpc.NewServer(),
		listener:   listener,
		watchDir:   watchDir,
		port:       port,
		sharedData: sharedData,
	}

	return server, nil
}

// Start starts the gRPC server and file state listener
func (s *State) Start(wg *sync.WaitGroup, ctx context.Context, sd *PeerData) error {
	defer wg.Done()

	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, &FileSyncServer{PeerData: sd})
		if err := s.grpcServer.Serve(s.listener); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()

	_, err := s.listen()
	if err != nil {
		return fmt.Errorf("failed to start directory watcher: %v", err)
	}

	wg.Add(1)
	go s.State(ctx, wg)

	return nil
}

func (s *State) listen() (*fsnotify.Watcher, error) {
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
				if !pkg.ContainsString(s.sharedData.SyncedFiles, event.Name) {
					s.EventHandler(event)
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
func (s *State) EventHandler(event fsnotify.Event) {
	s.sharedData.markFileAsInProgress(event.Name)

	switch {
	case event.Has(fsnotify.Create):
		// instant file creation on peer
		log.Printf("File created: %s", event.Name)
		s.startStreamingFile(event.Name)
	case event.Has(fsnotify.Write):
		// If file has been modified, start streaming new chunks file on peer
		log.Printf("File modified: %s", event.Name)
		s.startStreamingFile(event.Name)
	case event.Has(fsnotify.Remove):
		// delete file on peer
		log.Printf("File deleted: %s", event.Name)
		s.sharedData.markFileAsInProgress(event.Name)
		s.streamDelete(event.Name)
	}

	s.sharedData.markFileAsComplete(event.Name)
}

// Start streaming a file to all peers
func (s *State) startStreamingFile(filePath string) {
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

func (s *State) sendFileChunkToPeers(fileName string, chunks [][]byte, totalChunks int) {
	s.sharedData.mu.RLock()
	clients := make([]string, len(s.sharedData.Clients))
	copy(clients, s.sharedData.Clients)
	s.sharedData.mu.RUnlock()

	for _, ip := range clients {
		conn, err := grpc.NewClient(ip, grpc.WithInsecure())
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
	}
}

func (s *State) State(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down list check...")
			return
		case <-ticker.C:

			localFiles, err := pkg.GetFileList()
			if err != nil {
				log.Errorf("Error getting file list: %v", err)
				return
			}

			for peer, conn := range s.sharedData.Clients {
				go func() {
					stream, err := clients.StateStream(conn)
					if err != nil {
						log.Printf("Error starting stream to peer %v: %v", peer, err)
						return
					}

					res, err := stream.Recv()
					if err != nil {
						log.Printf("Error receiving response from %v: %v", peer, err)
						return
					}

					filesFromPeer := res.Message
					log.Printf("Files on peer: %v: %v", conn, filesFromPeer)
					log.Printf("Files on host: %v", localFiles)

					peerMissingFiles := pkg.SubtractValues(filesFromPeer, localFiles)

					for _, file := range peerMissingFiles {
						s.sharedData.markFileAsInProgress(file)
					}

					for _, file := range peerMissingFiles {
						go s.startStreamingFile(file)
					}
				}()
			}
		}
	}
}

func (s *State) streamDelete(fileName string) {

	for peer, conn := range s.sharedData.Clients {

		stream, err := clients.SyncStream(conn)
		if err != nil {
			log.Printf("Error starting stream to peer %v: %v", peer, err)
			continue
		}

		go func() {
			for {
				recv, err := stream.Recv()
				if err != nil {
					log.Printf("Error receiving response from %v: %v", peer, err)
					break
				}
				if err == io.EOF {
					log.Printf("Stream closed by %v", peer)
					break
				}

				s.sharedData.markFileAsComplete(fileName)
				log.Printf("Received response from %v: %v", peer, recv.Message)
			}
		}()

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: fileName,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending delete request to peer %v: %v", peer, err)
		}
	}
}

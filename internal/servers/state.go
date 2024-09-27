package servers

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/internal/clients"
	pb "github.com/TypeTerrors/go_sync/proto"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
)

type State struct {
	listener   net.Listener
	grpcServer *grpc.Server
	sharedData *PeerData
	MetaData   *Meta
	fw         *FileWatcher
	syncdir    string
	port       string
}

// NewState creates a new State with default settings
func StateServer(metaData *Meta, sharedData *PeerData, port, syncDir string) (*State, error) {
	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %v", port, err)
	}

	// Initialize FileWatcher
	fw := NewFileWatcher(sharedData)

	// Initialize State
	server := &State{
		grpcServer: grpc.NewServer(),
		listener:   listener,
		port:       port,
		sharedData: sharedData,
		MetaData:   metaData,
		fw:         fw,
		syncdir:    syncDir,
	}

	return server, nil
}

func (s *State) Start(wg *sync.WaitGroup, ctx context.Context, sd *PeerData, md *Meta) error {
	defer wg.Done()

	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, NewFileSyncServer(s.syncdir, s.sharedData, s.MetaData))
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

	log.Printf("Watching directory: %s", s.syncdir)
	err = watcher.Add(s.syncdir)
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

				if event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("File created: %s", event.Name)
					s.fw.HandleFileCreation(event.Name)
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("File modified: %s", event.Name)
					s.fw.HandleFileModification(event.Name)
				}

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					log.Printf("File deleted: %s", event.Name)
					s.fw.HandleFileDeletion(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Error:", err)
			}
		}
	}()

	return watcher, nil
}

func (s *State) State(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// If you decide to keep periodic checks, adjust them to work with FileWatcher
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down list check...")
			return
		case <-ticker.C:
			// Implement any periodic tasks if necessary
		}
	}
}

func (s *State) PeriodicMetadataExchange(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down periodic metadata exchange...")
			return
		case <-ticker.C:
			s.exchangeMetadataWithPeers()
		}
	}
}

func (s *State) exchangeMetadataWithPeers() {
	s.sharedData.mu.RLock()
	peers := make([]string, len(s.sharedData.Clients))
	copy(peers, s.sharedData.Clients)
	s.sharedData.mu.RUnlock()

	for _, ip := range peers {
		go func(ip string) {
			stream, err := clients.ExchangeMetadataStream(ip)
			if err != nil {
				log.Printf("Error starting metadata exchange stream with peer %s: %v", ip, err)
				return
			}

			s.MetaData.mu.Lock()
			fileNames := make([]string, 0, len(s.MetaData.MetaData))
			for fileName := range s.MetaData.MetaData {
				fileNames = append(fileNames, fileName)
			}
			s.MetaData.mu.Unlock()

			for _, fileName := range fileNames {
				err := stream.Send(&pb.MetadataRequest{FileName: fileName})
				if err != nil {
					log.Printf("Error sending metadata request to peer %s: %v", ip, err)
					return
				}

				// Receive metadata response
				res, err := stream.Recv()
				if err != nil {
					log.Printf("Error receiving metadata response from peer %s: %v", ip, err)
					return
				}

				// Compare local and peer metadata
				s.handleMetadataResponse(res, fileName, ip)
			}
		}(ip)
	}
}

func (s *State) requestMissingChunks(fileName string, offsets []int64, ip string) {
	stream, err := clients.RequestChunksStream(ip)
	if err != nil {
		log.Printf("Error starting chunk request stream with peer %s: %v", ip, err)
		return
	}

	// Send chunk request
	err = stream.Send(&pb.ChunkRequest{
		FileName: fileName,
		Offsets:  offsets,
	})
	if err != nil {
		log.Printf("Error sending chunk request to peer %s: %v", ip, err)
		return
	}

	// Receive chunks
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error receiving chunk response from peer %s: %v", ip, err)
			break
		}

		// Write chunk to file
		s.MetaData.WriteChunkToFile(res.FileName, res.ChunkData, res.Offset)
		// Update metadata
		s.MetaData.UpdateFileMetaData(res.FileName, res.ChunkData, res.Offset, int64(len(res.ChunkData)))
	}
}

func (s *State) handleMetadataResponse(res *pb.MetadataResponse, fileName, ip string) {
	s.MetaData.mu.Lock()
	localMetaData, exists := s.MetaData.MetaData[fileName]
	s.MetaData.mu.Unlock()

	if !exists {
		log.Printf("Local metadata not found for file %s", fileName)
		return
	}

	// Build maps for easy comparison
	localChunks := localMetaData.Chunks
	peerChunks := make(map[int64]string)
	for _, chunk := range res.Chunks {
		peerChunks[chunk.Offset] = chunk.Hash
	}

	// Identify missing or mismatched chunks
	var missingOffsets []int64
	for offset, localHash := range localChunks {
		peerHash, exists := peerChunks[offset]
		if !exists || peerHash != localHash {
			missingOffsets = append(missingOffsets, offset)
		}
	}

	// Request missing chunks from peer
	if len(missingOffsets) > 0 {
		log.Printf("File %s has %d missing or mismatched chunks with peer %s", fileName, len(missingOffsets), ip)
		s.requestMissingChunks(fileName, missingOffsets, ip)
	}
}

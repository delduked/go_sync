package servers

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"

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
	MetaData   *Meta
}

// NewState creates a new State with default settings
func StateServer(metaData *Meta, sharedData *PeerData, watchDir, port string) (*State, error) {
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
		MetaData:   metaData,
	}

	return server, nil
}

// Start starts the gRPC server and file state listener
func (s *State) Start(wg *sync.WaitGroup, ctx context.Context, sd *PeerData, md *Meta) error {
	defer wg.Done()

	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, &FileSyncServer{PeerData: sd, LocalMetaData: md})
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
				} else if event.Has(fsnotify.Remove) {
					s.streamDelete(event.Name)
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
	fileName := event.Name

	// Check if the file is being deleted first
	if event.Has(fsnotify.Remove) {
		log.Printf("File deleted: %s", fileName)
		s.sharedData.markFileAsComplete(fileName) // Mark it complete to ensure no further actions
		s.streamDelete(fileName)
		return
	}

	// Handle other events like create and modify
	if s.sharedData.IsFileInProgress(fileName) {
		log.Printf("File %s is still being synced. Ignoring further modifications.", fileName)
		return
	}

	s.sharedData.markFileAsInProgress(event.Name)

	switch {
	case event.Has(fsnotify.Create):
		// instant file creation on peer
		log.Printf("File created: %s", event.Name)
		s.startStreamingFileInChunks(event.Name)
		// s.startStreamingFile(event.Name)
		// case event.Has(fsnotify.Write):
		// 	// If file has been modified, start streaming new chunks file on peer
		// 	log.Printf("File modified: %s", event.Name)
		// 	s.startStreamingFileInChunks(event.Name)
		// s.startStreamingFile(event.Name)
		// case event.Has(fsnotify.Remove):
		// 	// delete file on peer
		// 	log.Printf("File deleted: %s", event.Name)
		// 	s.streamDelete(event.Name)
	}

	s.sharedData.markFileAsComplete(event.Name)
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
						go s.startStreamingFileInChunks(file)
					}
				}()
			}
		}
	}
}

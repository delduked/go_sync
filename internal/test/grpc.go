package test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/charmbracelet/log"
)

type Grpc struct {
	pb.UnimplementedFileSyncServiceServer
	grpcServer *grpc.Server
	mdns       *Mdns
	meta       *Meta
	file       *FileData
	listener   net.Listener
	syncDir    string
	port       string
}

func NewGrpc(syncDir string, mdns *Mdns, meta *Meta, file *FileData, port string) *Grpc {

	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		panic(fmt.Sprintf("failed to listen on port %s: %v", port, err))
	}

	return &Grpc{
		grpcServer: grpc.NewServer(),
		syncDir:    syncDir,
		mdns:       mdns,
		meta:       meta,
		file:       file,
		listener:   listener,
		port:       port,
	}
}

func (s *Grpc) Start(wg *sync.WaitGroup) error {
	defer wg.Done()

	go func() {
		log.Printf("Starting gRPC server on port %s...", s.port)
		pb.RegisterFileSyncServiceServer(s.grpcServer, s) // Registering on s.grpcServer
		if err := s.grpcServer.Serve(s.listener); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server
func (s *Grpc) Stop() {
	s.grpcServer.GracefulStop()
	log.Info("gRPC server stopped gracefully.")
}

// Implement the SyncFile method as per the generated interface
func (s *Grpc) SyncFile(stream pb.FileSyncService_SyncFileServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving from stream: %v", err)
			return err
		}

		switch req := req.GetRequest().(type) {
		case *pb.FileSyncRequest_FileChunk:
			err := s.handleFileChunk(req.FileChunk)
			if err != nil {
				log.Errorf("Error handling file chunk: %v", err)
				return err
			}
		case *pb.FileSyncRequest_FileDelete:
			err := s.handleFileDelete(req.FileDelete)
			if err != nil {
				log.Errorf("Error handling file delete: %v", err)
				return err
			}
			if req.FileDelete.Offset != 0 {
				stream.Send(&pb.FileSyncResponse{
					Message: fmt.Sprintf("Chunk %s deleted in file %v on peer %v", req.FileDelete.FileName, req.FileDelete.Offset, s.mdns.LocalIP),
				})
			} else {
				stream.Send(&pb.FileSyncResponse{
					Message: fmt.Sprintf("File %s deleted on peer %v", req.FileDelete.FileName, s.mdns.LocalIP),
				})
			}
		case *pb.FileSyncRequest_FileTruncate:
			err := s.handleFileTruncate(req.FileTruncate)
			if err != nil {
				log.Errorf("Error handling file truncate: %v", err)
				return err
			}
		default:
			log.Warnf("Received unknown request type")
		}
	}
}

func (s *Grpc) ExchangeMetadata(stream pb.FileSyncService_ExchangeMetadataServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving metadata request: %v", err)
			return err
		}

		fileName := req.FileName

		metaData, err := s.meta.GetEntireFileMetaData(fileName)
		if err != nil {
			log.Errorf("Error getting metadata for file %s: %v", fileName, err)
			err := stream.Send(&pb.MetadataResponse{
				FileName: fileName,
				Chunks:   []*pb.ChunkMetadata{},
			})
			if err != nil {
				log.Errorf("Error sending metadata response: %v", err)
			}
			continue
		} else {
			// Prepare the chunks metadata
			var chunkMetadataList []*pb.ChunkMetadata
			for offset, hash := range metaData {
				chunkMetadataList = append(chunkMetadataList, &pb.ChunkMetadata{
					Offset:       offset,
					Hash:         hash.Stronghash,
					WeakChecksum: hash.Weakhash,
				})
			}

			// Send the metadata response
			err = stream.Send(&pb.MetadataResponse{
				FileName: fileName,
				Chunks:   chunkMetadataList,
			})
		}

		if err != nil {
			log.Errorf("Error sending metadata response: %v", err)
			return err
		}
	}
}

func (s *Grpc) RequestChunks(stream pb.FileSyncService_RequestChunksServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving chunk request: %v", err)
			return err
		}

		fileName := req.FileName
		offsets := req.Offsets

		filePath := filepath.Join(s.syncDir, fileName)
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Error opening file %s: %v", filePath, err)
			continue
		}
		defer file.Close()

		for _, offset := range offsets {
			chunkData := make([]byte, conf.AppConfig.ChunkSize)
			n, err := file.ReadAt(chunkData, offset)
			if err != nil && err != io.EOF {
				log.Printf("Error reading file %s at offset %d: %v", filePath, offset, err)
				continue
			}

			err = stream.Send(&pb.ChunkResponse{
				FileName:  fileName,
				Offset:    offset,
				ChunkData: chunkData[:n],
			})
			if err != nil {
				log.Errorf("Error sending chunk response: %v", err)
				return err
			}
		}
	}
}

func (s *Grpc) GetFileList(ctx context.Context, req *pb.GetFileListRequest) (*pb.GetFileListResponse, error) {
	fileList, err := s.buildFileList()
	if err != nil {
		return nil, err
	}
	return &pb.GetFileListResponse{
		FileList: fileList,
	}, nil
}

func (s *Grpc) buildFileList() (*pb.FileList, error) {
	files, err := pkg.GetFileList() // Function to get local file paths
	if err != nil {
		return nil, err
	}

	var fileEntries []*pb.FileEntry
	for _, filePath := range files {
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue // Skip if unable to stat file
		}

		fileEntries = append(fileEntries, &pb.FileEntry{
			FileName:     filepath.Base(filePath),
			FileSize:     fileInfo.Size(),
			LastModified: fileInfo.ModTime().Unix(),
		})
	}

	return &pb.FileList{
		Files: fileEntries,
	}, nil
}

func (s *Grpc) HealthCheck(stream pb.FileSyncService_HealthCheckServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Errorf("Error receiving health check request: %v", err)
		}
		// log.Infof(recv.Message)

		stream.Send(&pb.Pong{
			Message: fmt.Sprintf("Pong from %v at %v", s.mdns.LocalIP, time.Now().Unix()),
		})
	}
}

func (s *Grpc) GetFile(ctx context.Context, req *pb.RequestFileTransfer) (*pb.EmptyResponse, error) {
	fileName := req.GetFileName()
	filePath := filepath.Join(s.syncDir, fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound, "File %s not found", fileName)
	}

	// Start transferring the file to the requester
	go s.file.transferFile(filePath, true) // Assuming transferFile sends to all peers, modify if needed

	return &pb.EmptyResponse{}, nil
}

func (s *Grpc) RequestFileTransfer(ctx context.Context, req *pb.RequestFileTransfer) (*pb.EmptyResponse, error) {
	fileName := req.GetFileName()
	filePath := filepath.Join(s.syncDir, fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound, "File %s not found", fileName)
	}

	// Start transferring the file to the requester
	go s.file.transferFile(filePath, true) // Assuming transferFile sends to all peers, modify if needed

	return &pb.EmptyResponse{}, nil
}

// Handler for FileChunk messages
func (s *Grpc) handleFileChunk(chunk *pb.FileChunk) error {
	filePath := filepath.Join(s.syncDir, chunk.FileName)
	s.file.markFileAsInProgress(filePath)

	defer func() {
		s.file.markFileAsComplete(filePath)
	}()

	// Open the file for writing
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open file %s: %v", filePath, err)
		return err
	}
	defer file.Close()

	// Write the chunk data at the specified offset
	_, err = file.WriteAt(chunk.ChunkData, chunk.Offset)
	if err != nil {
		log.Printf("Failed to write to file %s at offset %d: %v", filePath, chunk.Offset, err)
		return err
	}

	// Update the metadata
	s.meta.SaveMetaData(filePath, chunk.ChunkData, chunk.Offset)

	log.Printf("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)
	return nil
}

// handleFileDelete deletes the specified file.
func (s *Grpc) handleFileDelete(fileDelete *pb.FileDelete) error {
	filePath := filepath.Clean(fileDelete.FileName)
	if fileDelete.Offset != 0 {
		// Delete specific chunk
		err := s.file.DeleteFileChunk(filePath, fileDelete.Offset)
		if err != nil {
			log.Printf("Error deleting chunk at offset %d in file %s: %v", fileDelete.Offset, filePath, err)
			return err
		}
		log.Printf("Deleted chunk at offset %d in file %s as per request", fileDelete.Offset, filePath)
	} else {
		// Delete entire file
		err := os.Remove(filePath)
		if err != nil {
			log.Printf("Error deleting file %s: %v", filePath, err)
			return err
		}

		s.meta.DeleteEntireFileMetaData(filePath)

		s.file.markFileAsComplete(filePath)

		log.Printf("Deleted file %s as per request", filePath)
	}

	return nil
}

// handleFileTruncate truncates the specified file to the given size.
func (s *Grpc) handleFileTruncate(fileTruncate *pb.FileTruncate) error {
	filePath := filepath.Join(s.syncDir, filepath.Clean(fileTruncate.FileName))
	err := os.Truncate(filePath, fileTruncate.Size)
	if err != nil {
		log.Printf("Error truncating file %s to size %d: %v", filePath, fileTruncate.Size, err)
		return err
	}
	log.Printf("Truncated file %s to size %d as per request", filePath, fileTruncate.Size)
	return nil
}

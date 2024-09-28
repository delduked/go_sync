package servers

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/charmbracelet/log"
)

type FileSyncServer struct {
	pb.UnimplementedFileSyncServiceServer
	syncDir       string
	PeerData      *PeerData
	LocalMetaData *Meta
	fw            *FileWatcher
}

func NewFileSyncServer(syncDir string, peerData *PeerData, localMetaData *Meta, fw *FileWatcher) *FileSyncServer {
	return &FileSyncServer{
		syncDir:       syncDir,
		PeerData:      peerData,
		LocalMetaData: localMetaData,
		fw:            fw,
	}
}

// Implement the SyncFile method as per the generated interface
func (s *FileSyncServer) SyncFile(stream pb.FileSyncService_SyncFileServer) error {
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

func (s *FileSyncServer) ExchangeMetadata(stream pb.FileSyncService_ExchangeMetadataServer) error {
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

		s.LocalMetaData.mu.Lock()
		metaData, exists := s.LocalMetaData.MetaData[fileName]
		s.LocalMetaData.mu.Unlock()

		if !exists {
			// If the file does not exist, send an empty response
			err := stream.Send(&pb.MetadataResponse{
				FileName: fileName,
				Chunks:   []*pb.ChunkMetadata{},
			})
			if err != nil {
				log.Errorf("Error sending metadata response: %v", err)
			}
			continue
		}

		// Prepare the chunks metadata
		var chunkMetadataList []*pb.ChunkMetadata
		for offset, hash := range metaData.Chunks {
			chunkMetadataList = append(chunkMetadataList, &pb.ChunkMetadata{
				Offset: offset,
				Hash:   hash,
			})
		}

		// Send the metadata response
		err = stream.Send(&pb.MetadataResponse{
			FileName: fileName,
			Chunks:   chunkMetadataList,
		})
		if err != nil {
			log.Errorf("Error sending metadata response: %v", err)
			return err
		}
	}
}

func (s *FileSyncServer) RequestChunks(stream pb.FileSyncService_RequestChunksServer) error {
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
			chunkData := make([]byte, conf.ChunkSize)
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



func (s *FileSyncServer) GetFileList(ctx context.Context, req *pb.GetFileListRequest) (*pb.GetFileListResponse, error) {
	fileList, err := s.buildFileList()
	if err != nil {
		return nil, err
	}
	return &pb.GetFileListResponse{
		FileList: fileList,
	}, nil
}

func (s *FileSyncServer) buildFileList() (*pb.FileList, error) {
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

func (s *FileSyncServer) GetFile(ctx context.Context, req *pb.RequestFileTransfer) (*pb.EmptyResponse, error) {
	fileName := req.GetFileName()
	filePath := filepath.Join(s.syncDir, fileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound, "File %s not found", fileName)
	}

	// Start transferring the file to the requester
	go s.fw.transferFile(filePath, true) // Assuming transferFile sends to all peers, modify if needed

	return &pb.EmptyResponse{}, nil
}

func (s *FileSyncServer) RequestFileTransfer(ctx context.Context, req *pb.RequestFileTransfer) (*pb.EmptyResponse, error) {
    fileName := req.GetFileName()
    filePath := filepath.Join(s.syncDir, fileName)

    // Check if file exists
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        return nil, status.Errorf(codes.NotFound, "File %s not found", fileName)
    }

    // Start transferring the file to the requester
    go s.fw.transferFile(filePath, true) // Assuming transferFile sends to all peers, modify if needed

    return &pb.EmptyResponse{}, nil
}

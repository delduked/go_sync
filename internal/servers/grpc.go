package servers

import (
	"io"
	"os"
	"path/filepath"

	"github.com/TypeTerrors/go_sync/conf"
	pb "github.com/TypeTerrors/go_sync/proto"

	"github.com/charmbracelet/log"
)

type FileSyncServer struct {
	pb.UnimplementedFileSyncServiceServer
	syncDir       string
	PeerData      *PeerData
	LocalMetaData *Meta
}

func NewFileSyncServer(syncDir string, peerData *PeerData, localMetaData *Meta) *FileSyncServer {
	return &FileSyncServer{
		syncDir:       syncDir,
		PeerData:      peerData,
		LocalMetaData: localMetaData,
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

// Handler for FileChunk messages
func (s *FileSyncServer) handleFileChunk(chunk *pb.FileChunk) error {
	// Directly write to the sync directory
	filePath := filepath.Join(s.syncDir, chunk.FileName)

	// Ensure the directory structure exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		log.Printf("Error creating directories for %s: %v", filePath, err)
		return err
	}

	// Determine file flags based on whether it's a new file
	flags := os.O_CREATE | os.O_WRONLY
	if chunk.IsNewFile {
		flags |= os.O_TRUNC
	}

	// Open the file with appropriate flags
	file, err := os.OpenFile(filePath, flags, 0644)
	if err != nil {
		log.Printf("Error opening file %s: %v", filePath, err)
		return err
	}
	defer file.Close()

	// Write the chunk at the specified offset
	_, err = file.WriteAt(chunk.ChunkData, chunk.Offset)
	if err != nil {
		log.Printf("Error writing to file %s: %v", filePath, err)
		return err
	}

	log.Printf("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)

	return nil
}

// handleFileDelete deletes the specified file.
func (s *FileSyncServer) handleFileDelete(fileDelete *pb.FileDelete) error {
	filePath := filepath.Clean(fileDelete.FileName)
	err := os.Remove(filePath)
	if err != nil {
		log.Printf("Error deleting file %s: %v", filePath, err)
		return err
	}
	log.Printf("Deleted file %s as per request", filePath)
	return nil
}

// handleFileTruncate truncates the specified file to the given size.
func (s *FileSyncServer) handleFileTruncate(fileTruncate *pb.FileTruncate) error {
	filePath := filepath.Clean(fileTruncate.FileName)
	err := os.Truncate(filePath, fileTruncate.Size)
	if err != nil {
		log.Printf("Error truncating file %s to size %d: %v", filePath, fileTruncate.Size, err)
		return err
	}
	log.Printf("Truncated file %s to size %d as per request", filePath, fileTruncate.Size)
	return nil
}

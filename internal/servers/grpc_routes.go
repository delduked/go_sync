package servers

import (
	"os"
	"path/filepath"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
)

// Handler for FileChunk messages
func (s *FileSyncServer) handleFileChunk(chunk *pb.FileChunk) error {
	filePath := filepath.Join(s.syncDir, chunk.FileName)
	s.fw.mu.Lock()
	s.fw.inProgress[filePath] = true
	s.fw.mu.Unlock()

	defer func() {
		s.fw.mu.Lock()
		delete(s.fw.inProgress, filePath)
		s.fw.mu.Unlock()
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
	s.LocalMetaData.UpdateFileMetaData(filePath, chunk.ChunkData, chunk.Offset, int64(len(chunk.ChunkData)))

	log.Printf("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)
	return nil
}

// handleFileDelete deletes the specified file.
func (s *FileSyncServer) handleFileDelete(fileDelete *pb.FileDelete) error {
	filePath := filepath.Clean(fileDelete.FileName)
	if fileDelete.Offset != 0 {
		// need to delete a specific offset in the file
		s.f.DeleteFileChunk(filePath, fileDelete.Offset)
		return nil
	}
	err := os.Remove(filePath)
	if err != nil {
		log.Printf("Error deleting file %s: %v", filePath, err)
		return err
	}

	s.LocalMetaData.DeleteFileMetaData(filePath)

	s.fw.mu.Lock()
	delete(s.fw.inProgress, filePath)
	s.fw.mu.Unlock()

	log.Printf("Deleted file %s as per request", filePath)
	return nil
}

// handleFileTruncate truncates the specified file to the given size.
func (s *FileSyncServer) handleFileTruncate(fileTruncate *pb.FileTruncate) error {
	filePath := filepath.Join(s.syncDir, filepath.Clean(fileTruncate.FileName))
	err := os.Truncate(filePath, fileTruncate.Size)
	if err != nil {
		log.Printf("Error truncating file %s to size %d: %v", filePath, fileTruncate.Size, err)
		return err
	}
	log.Printf("Truncated file %s to size %d as per request", filePath, fileTruncate.Size)
	return nil
}

package servers

import (
	"os"
	"path/filepath"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
)

// Handler for FileChunk messages
func (s *FileSyncServer) handleFileChunk(chunk *pb.FileChunk) error {
	s.fw.mu.Lock()
	s.fw.inProgress[chunk.FileName] = true
	s.fw.mu.Unlock()

	defer func() {
		s.fw.mu.Lock()
		delete(s.fw.inProgress, chunk.FileName)
		s.fw.mu.Unlock()
	}()

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

	if chunk.Offset+int64(len(chunk.ChunkData)) >= chunk.TotalSize {
		s.fw.mu.Lock()
		delete(s.fw.inProgress, chunk.FileName)
		s.fw.mu.Unlock()
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
	filePath := filepath.Join(s.syncDir, filepath.Clean(fileTruncate.FileName))
	err := os.Truncate(filePath, fileTruncate.Size)
	if err != nil {
		log.Printf("Error truncating file %s to size %d: %v", filePath, fileTruncate.Size, err)
		return err
	}
	log.Printf("Truncated file %s to size %d as per request", filePath, fileTruncate.Size)
	return nil
}

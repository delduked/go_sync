package pkg

import (
	"fmt"
	"os"
)

type ChunkReader struct {
	file *os.File
}

// NewChunkReader opens a file and initializes a ChunkReader for reading chunks.
func NewChunkReader(filePath string) (*ChunkReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return &ChunkReader{
		file: file,
	}, nil
}

// ReadChunk reads a chunk from the file at the given position (offset) and size.
func (cr *ChunkReader) ReadChunk(offset, chunkSize int64) ([]byte, error) {
	buffer := make([]byte, chunkSize)
	_, err := cr.file.ReadAt(buffer, offset)
	if err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("failed to read chunk at offset %d: %v", offset, err)
	}

	return buffer, nil
}

func (cr *ChunkReader) WriteChunk(offset int64, chunk []byte) error {
	_, err := cr.file.WriteAt(chunk, offset)
	if err != nil {
		return fmt.Errorf("failed to write chunk at offset %d: %v", offset, err)
	}

	return nil
}

// Close closes the file when done.
func (cr *ChunkReader) Close() error {
	return cr.file.Close()
}

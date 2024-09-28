// Contents of ./internal/servers/meta_scan.go
package servers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
)

// PreScanAndStoreMetaData scans all files in a directory and stores metadata in memory and BadgerDB.
func (m *Meta) PreScanAndStoreMetaData(dir string) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Printf("Processing file: %s", path)
			metaData, err := m.getLocalFileMetadata(path, conf.AppConfig.ChunkSize)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
			m.saveMetaData(path, metaData)
		}
		return nil
	})
	return err
}

// getLocalFileMetadata retrieves metadata for a local file by reading it chunk by chunk.
func (m *Meta) getLocalFileMetadata(fileName string, chunkSize int64) (MetaData, error) {
	// Open the file
	file, err := os.Open(fileName)
	if err != nil {
		return MetaData{}, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	// Initialize MetaData structure
	fileMeta := MetaData{
		Chunks:    make(map[int64]string), // For strong hashes (using XXH3 here)
		WeakSums:  make(map[int64]uint32), // For rolling weak checksums
		ChunkSize: chunkSize,
	}

	// Buffer to hold file chunks
	buffer := make([]byte, chunkSize)
	var offset int64 = 0

	for {
		// Read chunk of file
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return MetaData{}, fmt.Errorf("error reading file: %w", err)
		}
		if bytesRead == 0 {
			break // End of file
		}

		// Calculate weak rolling checksum
		weakSum := pkg.NewRollingChecksum(buffer[:bytesRead]).Sum()
		fileMeta.WeakSums[offset] = weakSum // Store the weak checksum

		// Calculate strong hash using XXH3
		strongHash := m.hashChunk(buffer[:bytesRead])
		fileMeta.Chunks[offset] = strongHash // Store the XXH3 strong checksum

		// Move to the next chunk
		offset += int64(bytesRead)
	}

	// Return the file metadata with both weak and strong checksums
	return fileMeta, nil
}

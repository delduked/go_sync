package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TypeTerrors/go_sync/conf"
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
			metaData, err := m.getLocalFileMetadata(path, conf.ChunkSize)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
			m.mu.Lock()
			m.MetaData[path] = metaData
			m.mu.Unlock()

			// Store in BadgerDB
			if err := m.saveMetaDataToDB(path, metaData); err != nil {
				log.Errorf("Failed to store metadata in BadgerDB for file %s: %v", path, err)
			}
		}
		return nil
	})
	return err
}

// getLocalFileMetadata retrieves metadata for a local file by reading it chunk by chunk.
func (m *Meta) getLocalFileMetadata(fileName string, chunkSize int64) (MetaData, error) {
	var res MetaData
	file, err := os.Open(fileName)
	if err != nil {
		return res, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	fileMeta := MetaData{
		Chunks:    make(map[int64]string),
		ChunkSize: chunkSize,
	}

	buffer := make([]byte, chunkSize)
	var chunkIndex int64 = 0

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return res, fmt.Errorf("error reading file: %w", err)
		}
		if bytesRead == 0 {
			break // End of file
		}

		hasher := sha256.New()
		hasher.Write(buffer[:bytesRead])
		hash := hex.EncodeToString(hasher.Sum(nil))

		// Store the chunk hash in the metadata
		fileMeta.Chunks[chunkIndex] = hash
		chunkIndex++
	}

	return fileMeta, nil
}

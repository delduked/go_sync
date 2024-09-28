// Contents of ./internal/servers/meta_update.go
package servers

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
)

// ScanLocalMetaData periodically updates the metadata of all local files by reading each file and calculating the hash of its chunks.
func (m *Meta) ScanLocalMetaData(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down local metadata scan...")
			return
		case <-ticker.C:
			localFiles, err := pkg.GetFileList()
			if err != nil {
				log.Errorf("Error getting file list: %v", err)
				continue
			}

			for _, file := range localFiles {
				if m.PeerData.IsFileInProgress(file) {
					continue
				}
				fileMetaData, err := m.getLocalFileMetadata(file, conf.AppConfig.ChunkSize)
				if err != nil {
					log.Errorf("Failed to get metadata for file %s: %v", file, err)
					continue
				}

				m.mu.Lock()
				m.MetaData[file] = fileMetaData
				m.mu.Unlock()

				if err := m.saveMetaDataToDB(file, fileMetaData); err != nil {
					log.Errorf("Failed to store metadata in BadgerDB for file %s: %v", file, err)
				}
			}
		}
	}
}

// UpdateFileMetaData updates a specific file's metadata and persists it to BadgerDB.
func (m *Meta) UpdateFileMetaData(file string, chunkData []byte, offset int64, chunkSize int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.MetaData[file]; !ok {
		log.Warnf("File %s not found in metadata", file)
		m.MetaData[file] = MetaData{
			Chunks:    make(map[int64]string),
			WeakSums:  make(map[int64]uint32),
			ChunkSize: chunkSize,
		}
	}

	m.MetaData[file].WeakSums[offset] = pkg.NewRollingChecksum(chunkData).Sum()
	m.MetaData[file].Chunks[offset] = m.hashChunk(chunkData)

	// Save to BadgerDB
	if err := m.saveMetaDataToDB(file, m.MetaData[file]); err != nil {
		log.Errorf("Failed to update metadata in BadgerDB: %v", err)
	}
}

// WriteChunkToFile writes a chunk of data to the specified file at the given offset.
func (m *Meta) WriteChunkToFile(file string, chunkData []byte, offset int64) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Seek(offset, 0)
	if err != nil {
		return err
	}

	_, err = f.Write(chunkData)
	if err != nil {
		return err
	}

	return nil
}

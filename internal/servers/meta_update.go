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

// UpdateLocalMetaData periodically updates the metadata of all local files by reading each file and calculating the hash of its chunks.
func (m *Meta) ScanLocalMetaData(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(30 * time.Second)
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

			for _, file := range localFiles {
				if m.PeerData.IsFileInProgress(file) {
					continue
				}
				fileMetaData, err := m.getLocalFileMetadata(file, conf.ChunkSize)
				if err != nil || len(fileMetaData.Chunks) == 0 || fileMetaData.ChunkSize == 0 {
					continue
				}

				m.mu.Lock()
				m.MetaData[file] = fileMetaData
				m.mu.Unlock()

				go func() {
					if err := m.saveMetaDataToDB(file, fileMetaData); err != nil {
						log.Errorf("failed to store metadata in BadgerDB for file %s: %v", file, err)
					}
				}()
			}
		}
	}
}

// UpdateFileMetaData updates a specific file's metadata and persists it to BadgerDB.
func (m *Meta) UpdateFileMetaData(file string, chunkData []byte, offset int64, chunkSize int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metaData, ok := m.MetaData[file]
	if !ok {
		m.MetaData[file] = MetaData{
			Chunks:    make(map[int64]string),
			ChunkSize: chunkSize,
		}
		metaData = m.MetaData[file]
	}

	// Calculate the new hash for the current chunk
	newHash := m.hashChunk(chunkData)

	// Compare the new hash with the old one, and update only if necessary
	if oldHash, exists := metaData.Chunks[offset]; exists && oldHash == newHash {
		return // No need to update if the hash is the same
	}

	metaData.Chunks[offset] = newHash
	m.MetaData[file] = metaData

	// Save to BadgerDB
	if err := m.saveMetaDataToDB(file, metaData); err != nil {
		log.Errorf("failed to update metadata in badger DB: %v", err)
	}
}

func (m *Meta) WriteChunkToFile(file string, chunkData []byte, offset int64, chunkSize int64) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
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

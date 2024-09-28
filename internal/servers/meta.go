// Contents of ./internal/servers/meta.go
package servers

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
)

type MetaData struct {
	Chunks    map[int64]string // map[offset]hash
	WeakSums  map[int64]uint32 // map[offset]weakRollingChecksum
	ChunkSize int64
}

func (m MetaData) CalculateFileSize() (int64, error) {
	if len(m.Chunks) == 0 {
		return 0, fmt.Errorf("no chunks available")
	}
	numChunks := int64(len(m.Chunks))
	fileSize := numChunks * m.ChunkSize
	return fileSize, nil
}

type Meta struct {
	MetaData map[string]MetaData // map[fileName]MetaData
	PeerData *PeerData
	db       *badger.DB // BadgerDB instance
	mu       sync.Mutex
}

// NewMeta initializes the Meta struct with the provided PeerData and BadgerDB.
func NewMeta(peerData *PeerData, db *badger.DB) *Meta {
	return &Meta{
		MetaData: make(map[string]MetaData),
		PeerData: peerData,
		db:       db,
	}
}

// saveMetaData saves the metadata to both in-memory map and BadgerDB.
func (m *Meta) saveMetaData(file string, metadata MetaData) {
	m.mu.Lock()
	m.MetaData[file] = metadata
	m.mu.Unlock()

	if err := m.saveMetaDataToDB(file, metadata); err != nil {
		log.Errorf("Failed to save metadata to BadgerDB for file %s: %v", file, err)
	} else {
		log.Printf("Saved metadata for file %s", file)
	}
}

// // AddFileMetaData adds or updates metadata for a file.
// func (m *Meta) addFileMetaData(file string, chunkData []byte, offset int64, chunkSize int64) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()

// 	hash := m.hashChunk(chunkData)

// 	metaData, exists := m.MetaData[file]
// 	if !exists {
// 		metaData = MetaData{
// 			Chunks:    make(map[int64]string),
// 			ChunkSize: chunkSize,
// 		}
// 	}

// 	metaData.Chunks[offset] = hash
// 	m.MetaData[file] = metaData

// 	// Save to BadgerDB
// 	if err := m.saveMetaDataToDB(file, metaData); err != nil {
// 		log.Errorf("Failed to update metadata in BadgerDB: %v", err)
// 	}
// }

// GetMetaData retrieves the hash for a specific chunk of a file.
func (m *Meta) GetMetaData(file string, offset int64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metaData, ok := m.MetaData[file]
	if !ok {
		return "", fmt.Errorf("file not found in metadata")
	}
	hash, exists := metaData.Chunks[offset]
	if !exists {
		return "", fmt.Errorf("offset not found in metadata")
	}
	return hash, nil
}

func (m *Meta) DetectChangedChunks(fileName string, chunkSize int64) ([]int64, error) {
	currentMetaData, err := m.getLocalFileMetadata(fileName, chunkSize)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	storedMetaData, exists := m.MetaData[fileName]
	m.mu.Unlock()

	var changedOffsets []int64

	if !exists {
		// If we don't have stored metadata, consider all chunks as changed
		for offset := range currentMetaData.Chunks {
			changedOffsets = append(changedOffsets, offset)
		}
	} else {
		// Compare current chunks with stored chunks
		for offset, currentHash := range currentMetaData.Chunks {
			storedHash, exists := storedMetaData.Chunks[offset]
			if !exists || currentHash != storedHash {
				changedOffsets = append(changedOffsets, offset)
			}
		}
	}
	log.Printf("Detected %d changed chunks for file %s", len(changedOffsets), fileName)
	return changedOffsets, nil
}

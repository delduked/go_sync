package servers

import (
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v3"
)

type MetaData struct {
	Chunks    map[int64]string // map[chunk position]hash
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
	MetaData map[string]MetaData
	PeerData *PeerData
	db       *badger.DB // BadgerDB instance
	mu       sync.Mutex
}

// NewMeta initializes the Meta struct with the provided PeerData and BadgerDB.
func NewMeta(peerdata *PeerData, db *badger.DB) *Meta {
	return &Meta{
		MetaData: make(map[string]MetaData),
		PeerData: peerdata,
		db:       db,
	}
}

func (m *Meta) AddFileMetaData(file string, chunk []byte, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash, pos := m.hashPosition(chunk, offset)

	// Update the in-memory map
	if _, ok := m.MetaData[file]; ok {
		m.MetaData[file].Chunks[pos] = hash
	} else {
		m.MetaData[file] = MetaData{
			Chunks:    map[int64]string{pos: hash},
			ChunkSize: int64(len(chunk)),
		}
	}
}

// saveToBadger saves the updated metadata to BadgerDB.
// func (m *Meta) saveToBadger(file string) error {
// 	err := m.db.Update(func(txn *badger.Txn) error {
// 		meta := m.MetaData[file]

// 		// Serialize the MetaData into JSON format (or another format if necessary)
// 		val, err := json.Marshal(meta)
// 		if err != nil {
// 			return err
// 		}

// 		// Store the metadata in BadgerDB using the file name as the key
// 		return txn.Set([]byte(file), val)
// 	})
// 	return err
// }
// func (m *Meta) Close() error {
// 	return m.db.Close()
// }

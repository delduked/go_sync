package test

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
	"github.com/zeebo/xxh3"
)

type Meta struct {
	ChunkSize    int64
	Files        map[string]FileMetaData // map[filename][offset]Hash
	mismatchQeue chan MetaData
	mdns         *Mdns
	db           *badger.DB // BadgerDB instance
	mu           sync.Mutex
}

// Used for comparing metadata between old scan and new scan
// If the metadata was found to be different
type MetaData struct {
	filename string
	offset   int64
	filesize int64
}

type FileMetaData struct {
	hashes   map[int64]Hash
	filesize int64
}

func (f FileMetaData) CurrentSize() int64 {
	return int64(len(f.hashes)) * conf.AppConfig.ChunkSize
}
func (f FileMetaData) UpdateFileSize() {
	f.filesize = int64(len(f.hashes)) * conf.AppConfig.ChunkSize
}

type Hash struct {
	Stronghash string
	Weakhash   uint32
	filesize   int64
}

func (h Hash) Bytes() []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	enc.Encode(h)

	return buf.Bytes()
}

func NewMeta(db *badger.DB) *Meta {
	return &Meta{
		Files: make(map[string]FileMetaData),
		db:    db,
		mu:    sync.Mutex{},
	}
}

func (m *Meta) Start() error {
	err := filepath.Walk(conf.AppConfig.SyncFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Printf("Processing file: %s", path)
			err := m.CreateFileMetaData(path)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
		}
		return nil
	})
	return err
}
func (m *Meta) Scan(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down local metadata scan...")
			return
		case <-ticker.C:
			m.Start()
		}
	}
}

func (m *Meta) CreateFileMetaData(fileName string) error {
	// Open the file
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	// Buffer to hold file chunks
	buffer := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0
	for {
		// Read chunk of file
		bytesRead, err := file.Seek(offset, 0) // not sure if this is correct, i couuld be looping from the start or from the end of the file, but i'm incrementing the offset upwards
		if err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}
		if err != io.EOF {
			return fmt.Errorf("EOF reached: %w", err)
		}
		if bytesRead == 0 {
			break // End of file
		}

		m.SaveMetaData(fileName, buffer[:bytesRead], offset)

		// Move to the next chunk
		offset += int64(bytesRead)
	}

	// Return the file metadata with both weak and strong checksums
	return nil
}

// SaveMetaData will save the metadata to both in-memory map and the database
func (m *Meta) SaveMetaData(filename string, chunk []byte, offset int64) error {

	if pkg.IsTemporaryFile(filename) {
		return nil
	}

	// Also need to check if the hashes match, if they do, then we don't need to save the metadata
	// If they don't match, then we need to save the metadata
	// And send the new chunks at the same offsets that are different to the other peer

	// what would happen if the file size is now smaller?
	// i woulld not know how each chunk was changed, just that the file is now smaller
	// i would need to compare the file size to the number of chunks i have stored
	// what if only a portion of a chunk was removed? i wouldn't know how the missing chunk was changed
	// i would need to compare the file size to the number of chunks i have stored

	// what would happen if the file size is now larger?
	

	h, err := m.GetMetaData(filename, offset)
	if err != nil {
		log.Errorf("Failed to get metadata for file %s: %v", filename, err)
	} else if h.Stronghash == m.hashChunk(chunk) {
		m.mismatchQeue <- MetaData{
			filename: filename,
			offset:   offset,
		}
	}

	m.saveMetaDataToMem(filename, chunk, offset)

	if err := m.saveMetaDataToDB(filename, chunk, offset); err != nil {
		log.Printf("Failed to save metadata to BadgerDB: %v", err)
		return err
	}
	return nil
}

// saveMetaDataToDB will save the metadata to the database using the filename and the offset to determine the chunk position
func (m *Meta) saveMetaDataToDB(filename string, chunk []byte, offset int64) error {

	err := m.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(&badger.Entry{
			Key: []byte(filename + fmt.Sprintf("%d", offset)),
			Value: Hash{
				Stronghash: m.hashChunk(chunk),
				Weakhash:   pkg.NewRollingChecksum(chunk).Sum(),
			}.Bytes(),
		})
	})
	if err != nil {
		return err
	}
	return nil
}

// saveMetaDataToMem will save the metadata to the in-memory map using the filename and the offset to determine the chunk position
func (m *Meta) saveMetaDataToMem(filename string, chunk []byte, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[filename]; !ok {
		m.Files[filename] = FileMetaData{
			hashes:   make(map[int64]Hash),
			filesize: 0,
		}
	}
	m.Files[filename].hashes[offset] = Hash{
		Stronghash: m.hashChunk(chunk),
		Weakhash:   pkg.NewRollingChecksum(chunk).Sum(),
	}
	m.Files[filename].UpdateFileSize()
}

// GetMetaData will retrieve the metadata using the filename and the offset determining the chunk position.
// It will first check the in-memory map and then the database if the metadata is not found in the map.
func (m *Meta) GetMetaData(filename string, offset int64) (Hash, error) {
	// Check in-memory map first
	// If not found, check the database
	// If not found in the database, return an error
	h, err := m.getMetaDataFromMem(filename, offset)
	if err == nil {
		return h, err
	}
	h, err = m.getMetaDataFromDB(filename, offset)
	if err != nil {
		return h, fmt.Errorf("failed to get metadata for file %s: %v", filename, err)
	}
	return h, nil
}

// getMetaDataFromDB will retrieve the metdata using the filename and the offset determining the chunk position in thhe database
func (m *Meta) getMetaDataFromDB(filename string, offset int64) (Hash, error) {
	var h Hash
	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(filename + fmt.Sprintf("%d", offset)))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			dec := gob.NewDecoder(bytes.NewReader(val))
			err := dec.Decode(&h)
			if err != nil {
				return err
			}
			return nil
		})
	})
	if err != nil {
		return h, fmt.Errorf("failed to get metadata from BadgerDB: %v", err)
	}
	return h, nil
}

// getMetaDataFromMem will retrieve the metadata from the in-memory map using the offset to dertermine the chunk position
func (m *Meta) getMetaDataFromMem(filename string, offset int64) (Hash, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var h Hash

	if _, ok := m.Files[filename]; !ok {
		return h, fmt.Errorf("file not found in metadata")
	}
	if _, ok := m.Files[filename].hashes[offset]; !ok {
		return h, fmt.Errorf("offset not found in metadata")
	}
	h = m.Files[filename].hashes[offset]
	return h, nil
}

// DeleteMetaData will delete the metadata from both the in-memory map and the database
func (m *Meta) DeleteMetaData(filename string, offset int64) (error, error) {
	var err1, err2 error
	if err1 := m.deleteMetaDataFromMem(filename, offset); err1 != nil {
		log.Errorf("Failed to delete metadata from in-memory map: %v", err1)
	}
	if err2 := m.deleteMetaDataFromDB(filename, offset); err2 != nil {
		log.Errorf("Failed to delete metadata from BadgerDB: %v", err2)
	}
	return err1, err2
}

// deleteMetaDataFromDB will delete the metadata from the database using the filename and the offset to determine the chunk position
func (m *Meta) deleteMetaDataFromDB(filename string, offset int64) error {
	err := m.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(filename + fmt.Sprintf("%d", offset)))
	})
	if err != nil {
		return err
	}
	return nil
}

// deleteMetaDataFromMem will delete the metadata from the in-memory map using the filename and the offset to determine the chunk position
func (m *Meta) deleteMetaDataFromMem(filename string, offset int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[filename]; !ok {
		return fmt.Errorf("file not found in metadata")
	}
	if _, ok := m.Files[filename].hashes[offset]; !ok {
		return fmt.Errorf("offset not found in metadata")
	}
	delete(m.Files[filename].hashes, offset)
	return nil
}

// hashChunk will hash the chunk data and return the hash
func (m *Meta) hashChunk(chunk []byte) string {
	// Calculate the 128-bit XXH3 hash for the chunk
	hash := xxh3.Hash128(chunk)
	// Format the 128-bit hash into a hexadecimal string
	return fmt.Sprintf("%016x%016x", hash.Lo, hash.Hi)
}

func (m *Meta) MetaDataMismatchBatch(done <-chan struct{}) {
	for {
		select {
		case <-m.mismatchQeue:
			// Send the new chunks at the same offsets because a local file has changed
		case <-done:
			// stop listening for messages in queue
			return
		}
	}
}

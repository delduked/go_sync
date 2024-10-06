package servers

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
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
	"github.com/zeebo/xxh3"
)

// MetaInterface defines methods that other services need from Meta
type MetaInterface interface {
	CreateFileMetaData(fileName string, isNewFile bool) error
	DeleteEntireFileMetaData(filename string) (error, error)
	GetEntireFileMetaData(filename string) (map[int64]Hash, error)
	SaveMetaData(filename string, chunk []byte, offset int64, isNewFile bool) error // ... other methods as needed
	SaveMetaDataToDB(filename string, chunk []byte, offset int64) error
	SaveMetaDataToMem(filename string, chunk []byte, offset int64)
	DeleteMetaDataFromDB(filename string, offset int64) error
	DeleteMetaDataFromMem(filename string, offset int64) error

	// Other services need access to the Files map, so we need to expose it using a method
	TotalChunks(filename string) (int64, error)
	SetConn(conn ConnInterface)
}

type Meta struct {
	ChunkSize int64
	Files     map[string]FileMetaData // map[filename][offset]Hash
	// mdns      *Mdns
	db   *badger.DB // BadgerDB instance
	mu   sync.Mutex
	done chan struct{}
	conn ConnInterface
}

type FileMetaData struct {
	hashes       map[int64]Hash
	filesize     int64
	lastModified time.Time
}

func (fm FileMetaData) TotalChunks() int64 {
	return (fm.filesize + conf.AppConfig.ChunkSize - 1) / conf.AppConfig.ChunkSize
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

func NewMeta(db *badger.DB, mdns *Mdns) *Meta {
	return &Meta{
		Files: make(map[string]FileMetaData),
		db:    db,
		mu:    sync.Mutex{},
		// conn:  conn,
		done: make(chan struct{}), // Initialize the done channel
	}
}

func (m *Meta) Scan() error {

	// Walk through the sync folder and process existing files
	err := filepath.Walk(conf.AppConfig.SyncFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Debugf("Processing file: %s", path)
			err := m.CreateFileMetaData(path, false)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
		}
		return nil
	})
	return err
}

func (m *Meta) Start(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(conf.AppConfig.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Warn("Shutting down local metadata scan...")
			return
		case <-ticker.C:
			log.Info("Starting periodic scan of sync folder.")
			err := m.ScanSyncFolder()
			if err != nil {
				log.Errorf("Error during periodic scan: %v", err)
			}
		}
	}
}

func (m *Meta) SetConn(conn ConnInterface) {
	m.conn = conn
}

func (m *Meta) ScanSyncFolder() error {
	return filepath.Walk(conf.AppConfig.SyncFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Debugf("Scanning file: %s", path)
			err := m.CreateFileMetaData(path, false)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
		}
		return nil
	})
}

func (m *Meta) CreateFileMetaData(fileName string, isNewFile bool) error {
	if pkg.IsTemporaryFile(fileName) {
		return nil
	}

	// Open the file
	log.Debugf("Opening file: %s", fileName)
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	} else {
		log.Debugf("Opened file: %s", fileName)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", fileName, err)
	} else {
		log.Debugf("Got file info for: %s", fileName)
	}

	m.mu.Lock()
	fileMeta, exists := m.Files[fileName]
	if !exists {
		// File is new, initialize metadata
		fileMeta = FileMetaData{
			hashes:   make(map[int64]Hash),
			filesize: fileInfo.Size(),
		}
		m.Files[fileName] = fileMeta
		log.Debugf("Initialized metadata for new file: %s", fileName)
	}
	m.mu.Unlock()

	if exists && fileMeta.lastModified.Equal(fileInfo.ModTime()) {
		// File hasn't changed since last processing
		log.Debugf("File %s hasn't changed since last processing", fileName)
		return nil
	}

	// Update lastModified
	m.mu.Lock()
	fileMeta.lastModified = fileInfo.ModTime()
	m.Files[fileName] = fileMeta
	m.mu.Unlock()
	log.Debugf("Updated last modified time for file: %s", fileName)

	// Buffer to hold file chunks
	buffer := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for offset < fileInfo.Size() {
		// Read the chunk from the current offset
		bytesToRead := min(conf.AppConfig.ChunkSize, fileInfo.Size()-offset)
		log.Debugf("Reading chunk of %d bytes at offset %d", bytesToRead, offset)
		bytesRead, err := file.ReadAt(buffer[:bytesToRead], offset)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading file %s at offset %d: %w", fileName, offset, err)
		} else {
			log.Debugf("Read %d bytes at offset %d", bytesRead, offset)
		}

		if bytesRead == 0 {
			log.Debugf("End of file reached: %s", fileName)
			break // End of file
		}

		// Compute hashes
		newWeakHash := pkg.NewRollingChecksum(buffer[:bytesRead]).Sum()
		log.Debugf("Weak hash: %d", newWeakHash)
		newStrongHash := m.hashChunk(buffer[:bytesRead])
		log.Debugf("Strong hash: %s", newStrongHash)

		// Compare with existing hash
		m.mu.Lock()
		oldHash, hasOldHash := fileMeta.hashes[offset]
		m.mu.Unlock()
		log.Debugf("Old hash: %v", oldHash)

		chunkModified := !hasOldHash || oldHash.Weakhash != newWeakHash || oldHash.Stronghash != newStrongHash
		if chunkModified {
			log.Debugf("Chunk modified at offset %d", offset)
			// Update metadata
			fileMeta.hashes[offset] = Hash{
				Stronghash: newStrongHash,
				Weakhash:   newWeakHash,
			}
			// Save metadata to DB
			err := m.SaveMetaData(fileName, buffer[:bytesRead], offset, isNewFile)
			if err != nil {
				return fmt.Errorf("failed to save metadata: %w", err)
			}
		}

		// After the first chunk, set isNewFile to false
		isNewFile = false
		log.Debugf("isNewFile set to false")

		// Move to the next chunk
		offset += int64(bytesRead)
		log.Debugf("Moved to next chunk at offset %d", offset)
	}

	// Remove any chunks beyond the current file size
	m.mu.Lock()
	for oldOffset := range fileMeta.hashes {
		if oldOffset >= fileInfo.Size() {
			m.DeleteMetaData(fileName, oldOffset)
		}
	}
	// Update file size
	fileMeta.filesize = fileInfo.Size()
	m.Files[fileName] = fileMeta
	m.mu.Unlock()

	log.Debugf("Updated file size for file %s: %d", fileName, fileInfo.Size())
	return nil
}

// Helper function to get the minimum of two int64 values
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// meta.go

func (m *Meta) SaveMetaData(filename string, chunk []byte, offset int64, isNewFile bool) error {
	if pkg.IsTemporaryFile(filename) {
		return nil
	}

	// Save new metadata
	log.Debug("Saving metadata...")
	m.SaveMetaDataToMem(filename, chunk, offset)
	m.SaveMetaDataToDB(filename, chunk, offset)

	// Get the relative file path
	relativePath, err := filepath.Rel(conf.AppConfig.SyncFolder, filename)
	if err != nil {
		log.Errorf("Error getting relative path for %s: %v", filename, err)
		return err
	}

	log.Debug("Sending file at path %s metadata to peers...", relativePath)
	m.conn.SendMessage(&pb.FileSyncRequest{
		Request: &pb.FileSyncRequest_FileChunk{
			FileChunk: &pb.FileChunk{
				FileName:    relativePath, // Use relative path
				ChunkData:   chunk,
				Offset:      offset,
				IsNewFile:   isNewFile,
				TotalChunks: m.Files[filename].TotalChunks(),
				TotalSize:   m.Files[filename].filesize,
			},
		},
	})

	return nil
}

// saveMetaDataToDB will save the metadata to the database using the filename and the offset to determine the chunk position
func (m *Meta) SaveMetaDataToDB(filename string, chunk []byte, offset int64) error {

	err := m.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(&badger.Entry{
			Key: []byte(fmt.Sprintf("%s_%d", filename, offset)),
			Value: Hash{
				Stronghash: m.hashChunk(chunk),
				Weakhash:   pkg.NewRollingChecksum(chunk).Sum(),
			}.Bytes(),
		})
	})
	if err != nil {
		return err
	} else {
		log.Debugf("Saved metadata to BadgerDB for file %s at offset %d", filename, offset)
	}
	return nil
}

// saveMetaDataToMem will save the metadata to the in-memory map using the filename and the offset to determine the chunk position
func (m *Meta) SaveMetaDataToMem(filename string, chunk []byte, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Debug("Saving metadata to in-memory map...")
	if _, ok := m.Files[filename]; !ok {
		log.Debugf("File not found in metadata, creating new entry: %s", filename)
		m.Files[filename] = FileMetaData{
			hashes:   make(map[int64]Hash),
			filesize: 0,
		}
	} else {
		log.Debugf("File found in metadata: %s", filename)
	}
	log.Debug("m.Files[filename].hashes[offset] = Hash{}")
	m.Files[filename].hashes[offset] = Hash{
		Stronghash: m.hashChunk(chunk),
		Weakhash:   pkg.NewRollingChecksum(chunk).Sum(),
	}
	// Do not use UpdateFileSize; set it to the actual file size elsewhere
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
	} else {
		log.Debugf("Metadata not found in memory for file %s at offset %d", filename, offset)
	}
	h, err = m.getMetaDataFromDB(filename, offset)
	if err != nil {
		return h, fmt.Errorf("failed to get metadata for file %s: %v", filename, err)
	} else {
		log.Debugf("Retrieved metadata from BadgerDB for file %s at offset %d", filename, offset)
	}
	return h, nil
}

// getMetaDataFromDB will retrieve the metdata using the filename and the offset determining the chunk position in thhe database
func (m *Meta) getMetaDataFromDB(filename string, offset int64) (Hash, error) {
	var h Hash
	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("%s_%d", filename, offset)))
		if err != nil {
			return err
		} else {
			log.Debugf("Found metadata in BadgerDB for file %s at offset %d", filename)
		}
		return item.Value(func(val []byte) error {
			dec := gob.NewDecoder(bytes.NewReader(val))
			err := dec.Decode(&h)
			if err != nil {
				return err
			} else {
				log.Debugf("Decoded metadata from BadgerDB for file %s at offset %d", filename, offset)
			}
			return nil
		})
	})
	if err != nil {
		return h, fmt.Errorf("failed to get metadata from BadgerDB: %v", err)
	} else {
		log.Debugf("Retrieved metadata from BadgerDB for file %s at offset %d", filename, offset)
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
	} else {
		log.Debugf("Found file in metadata: %s", filename)
	}
	if _, ok := m.Files[filename].hashes[offset]; !ok {
		return h, fmt.Errorf("offset not found in metadata for file %s", filename)
	} else {
		log.Debugf("Found offset in metadata for file %s: %d", filename, offset)
	}
	h = m.Files[filename].hashes[offset]
	return h, nil
}

func (m *Meta) DeleteEntireFileMetaData(filename string) (error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var err1, err2 error
	for offset := range m.Files[filename].hashes {
		err1 = m.DeleteMetaDataFromDB(filename, offset)
	}
	if _, exists := m.Files[filename]; exists {
		log.Debugf("Deleting metadata from in-memory map for file %s", filename)
		delete(m.Files, filename)
	} else {
		err2 = fmt.Errorf("metadata for file %s not found in memory", filename)
		return err1, err2
	}

	return err1, err2
}

func (m *Meta) GetEntireFileMetaData(filename string) (map[int64]Hash, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Files[filename]; !ok {
		return nil, fmt.Errorf("file not found in metadata")
	} else {
		log.Debugf("Found file in metadata: %s", filename)
	}
	return m.Files[filename].hashes, nil
}

// DeleteMetaData will delete the metadata from both the in-memory map and the database
func (m *Meta) DeleteMetaData(filename string, offset int64) (error, error) {
	if pkg.IsTemporaryFile(filename) {
		return nil, nil
	}

	m.conn.SendMessage(&pb.FileSyncRequest{
		Request: &pb.FileSyncRequest_FileDelete{
			FileDelete: &pb.FileDelete{
				FileName: filename,
				Offset:   offset,
			},
		},
	})

	var err1, err2 error
	if err1 := m.DeleteMetaDataFromMem(filename, offset); err1 != nil {
		log.Errorf("Failed to delete metadata from in-memory map: %v", err1)
	} else {
		log.Debugf("Deleted metadata from in-memory map for file %s at offset %d", filename, offset)
	}
	if err2 := m.DeleteMetaDataFromDB(filename, offset); err2 != nil {
		log.Errorf("Failed to delete metadata from BadgerDB: %v", err2)
	} else {
		log.Debugf("Deleted metadata from BadgerDB for file %s at offset %d", filename, offset)
	}
	return err1, err2
}

// deleteMetaDataFromDB will delete the metadata from the database using the filename and the offset to determine the chunk position
func (m *Meta) DeleteMetaDataFromDB(filename string, offset int64) error {
	err := m.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(fmt.Sprintf("%s_%d", filename, offset)))
	})
	if err != nil {
		return err
	} else {
		log.Debugf("Deleted metadata from BadgerDB for file %s at offset %d", filename, offset)
	}
	return nil
}

// deleteMetaDataFromMem will delete the metadata from the in-memory map using the filename and the offset to determine the chunk position
func (m *Meta) DeleteMetaDataFromMem(filename string, offset int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[filename]; !ok {
		return fmt.Errorf("file not found in metadata")
	} else {
		log.Debugf("Found file in metadata: %s", filename)
	}
	if _, ok := m.Files[filename].hashes[offset]; !ok {
		return fmt.Errorf("offset not found in metadata")
	} else {
		log.Debugf("Found offset in metadata for file %s: %d", filename, offset)
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

// In MetaInterface
func (m *Meta) TotalChunks(filename string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fileMeta, exists := m.Files[filename]
	if !exists {
		return 0, fmt.Errorf("file %s not found in metadata", filename)
	} else {
		log.Debugf("Found file in metadata: %s", filename)
	}
	return fileMeta.TotalChunks(), nil
}

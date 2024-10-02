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
	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
	"github.com/zeebo/xxh3"
)

type Meta struct {
	ChunkSize           int64
	Files               map[string]FileMetaData // map[filename][offset]Hash
	mdns                *Mdns
	db                  *badger.DB // BadgerDB instance
	mu                  sync.Mutex
	lastProcessedOffset map[string]int64 // Map of filename to last processed offset
	done                chan struct{}
}

// Used for comparing metadata between old scan and new scan
// If the metadata was found to be different
type MetaData struct {
	filename string
	offset   int64
	filesize int64
}

type FileMetaData struct {
	hashes       map[int64]Hash
	filesize     int64
	lastModified time.Time
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
		// SaveChunks:   make(chan MetaData, 100), // Buffered to prevent blocking
		// DeleteChunks: make(chan MetaData, 100),
		mdns:                mdns,
		db:                  db,
		mu:                  sync.Mutex{},
		lastProcessedOffset: make(map[string]int64),
		done:                make(chan struct{}), // Initialize the done channel
	}
}

func (m *Meta) Start(ctx context.Context, wg *sync.WaitGroup) error {

	// Walk through the sync folder and process existing files
	err := filepath.Walk(conf.AppConfig.SyncFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Printf("Processing file: %s", path)
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

func (m *Meta) Scan(wg *sync.WaitGroup, ctx context.Context) {
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

func (m *Meta) ScanSyncFolder() error {
	return filepath.Walk(conf.AppConfig.SyncFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			log.Printf("Scanning file: %s", path)
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
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", fileName, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get existing file metadata
	fileMeta, exists := m.Files[fileName]
	if !exists {
		// File is new, initialize metadata
		fileMeta = FileMetaData{
			hashes:   make(map[int64]Hash),
			filesize: 0,
		}
		m.Files[fileName] = fileMeta
	}

	if exists && fileMeta.lastModified.Equal(fileInfo.ModTime()) {
		// File hasn't changed since last processing
		return nil
	}

	// Update lastModified
	fileMeta.lastModified = fileInfo.ModTime()

	// Buffer to hold file chunks
	buffer := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for offset < fileInfo.Size() {
		// Read the chunk from the current offset
		bytesToRead := min(conf.AppConfig.ChunkSize, fileInfo.Size()-offset)
		bytesRead, err := file.ReadAt(buffer[:bytesToRead], offset)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading file %s at offset %d: %w", fileName, offset, err)
		}

		if bytesRead == 0 {
			break // End of file
		}

		// Compute hashes
		newWeakHash := pkg.NewRollingChecksum(buffer[:bytesRead]).Sum()
		newStrongHash := m.hashChunk(buffer[:bytesRead])

		// Compare with existing hash
		oldHash, hasOldHash := fileMeta.hashes[offset]
		chunkModified := !hasOldHash || oldHash.Weakhash != newWeakHash || oldHash.Stronghash != newStrongHash

		if chunkModified {
			// Update metadata
			fileMeta.hashes[offset] = Hash{
				Stronghash: newStrongHash,
				Weakhash:   newWeakHash,
			}
			// Save metadata to DB
			m.SaveMetaData(fileName, buffer[:bytesRead], offset, isNewFile)
		}

		// After the first chunk, set isNewFile to false
		isNewFile = false

		// Move to the next chunk
		offset += int64(bytesRead)
	}

	// Remove any chunks beyond the current file size
	for oldOffset := range fileMeta.hashes {
		if oldOffset >= fileInfo.Size() {
			m.DeleteMetaData(fileName, oldOffset)
		}
	}

	// Update file size
	fileMeta.filesize = fileInfo.Size()
	m.Files[fileName] = fileMeta

	return nil
}

// Helper function to get the minimum of two int64 values
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// SaveMetaData will save the metadata to both in-memory map and the database
func (m *Meta) SaveMetaData(filename string, chunk []byte, offset int64, isNewFile bool) error {
	if pkg.IsTemporaryFile(filename) {
		return nil
	}

	// oldMeta, err := m.GetMetaData(filename, offset)
	// newWeakHash := pkg.NewRollingChecksum(chunk).Sum()
	// newStrongHash := m.hashChunk(chunk)

	// if err == nil {
	// 	// Existing metadata found; compare hashes
	// 	if oldMeta.Weakhash == newWeakHash && oldMeta.Stronghash == newStrongHash {
	// 		// No change; skip processing
	// 		return nil
	// 	}
	// }

	// Save new metadata
	m.saveMetaDataToMem(filename, chunk, offset)
	m.saveMetaDataToDB(filename, chunk, offset)

	// Send chunk to peers
	m.SendSaveToPeers(MetaData{
		filename: filename,
		offset:   offset,
	}, chunk, isNewFile)

	return nil
}

// saveMetaDataToDB will save the metadata to the database using the filename and the offset to determine the chunk position
func (m *Meta) saveMetaDataToDB(filename string, chunk []byte, offset int64) error {

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
		item, err := txn.Get([]byte(fmt.Sprintf("%s_%d", filename, offset)))
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
func (m *Meta) DeleteEntireFileMetaData(filename string) (error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var err1, err2 error
	for offset := range m.Files[filename].hashes {
		err1 = m.deleteMetaDataFromDB(filename, offset)
	}
	if _, exists := m.Files[filename]; exists {
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
	}
	return m.Files[filename].hashes, nil
}

// DeleteMetaData will delete the metadata from both the in-memory map and the database
func (m *Meta) DeleteMetaData(filename string, offset int64) (error, error) {
	if pkg.IsTemporaryFile(filename) {
		return nil, nil
	}

	go m.SendDeleteToPeers(MetaData{
		filename: filename,
		offset:   offset,
	})

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
		return txn.Delete([]byte(fmt.Sprintf("%s_%d", filename, offset)))
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

func (m *Meta) DeleteChunkOnPeer(req MetaData) {
	for _, conn := range m.mdns.Clients {
		stream, err := clients.SyncConn(conn)
		if err != nil {
			log.Errorf("Failed to open SyncFile stream on %s: %v", conn.Target(), err)
			continue
		}
		go func() {
			for {
				recv, err := stream.Recv()
				if err != nil {
					log.Errorf("Failed to receive response from %s: %v", conn.Target(), err)
					break
				}
				log.Infof(recv.Message)
			}
		}()
		stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: req.filename,
					Offset:   req.offset,
				},
			},
		})
	}
}
func (m *Meta) SaveChunkOnPeer(req MetaData) {
	for _, conn := range m.mdns.Clients {
		stream, err := clients.SyncConn(conn)
		if err != nil {
			log.Errorf("Failed to open SyncFile stream on %s: %v", conn.Target(), err)
			continue
		}
		go func() {
			for {
				recv, err := stream.Recv()
				if err != nil {
					if err == io.EOF {
						// Stream closed gracefully
						return
					}
					log.Errorf("Failed to receive response from %s: %v", conn.Target(), err)
					break
				}
				log.Infof(recv.Message)
			}
		}()
		stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName: req.filename,
					Offset:   req.offset,
				},
			},
		})
	}
}

// SendDeleteToPeers enqueues delete requests to each peer's send channel.
func (m *Meta) SendDeleteToPeers(req MetaData) {
	m.mdns.mu.Lock()
	defer m.mdns.mu.Unlock()

	for peerID, sendChan := range m.mdns.sendChannels {
		// Prepare the delete request
		deleteReq := pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: req.filename,
					Offset:   req.offset, // 0 signifies entire file deletion
				},
			},
		}

		// Enqueue the delete request
		select {
		case sendChan <- &deleteReq:
			// Successfully enqueued
		default:
			log.Warnf("Send channel for peer %s is full, dropping delete request", peerID)
		}
	}
}

// SendSaveToPeers enqueues save chunk requests to each peer's send channel.
func (m *Meta) SendSaveToPeers(req MetaData, chunk []byte, isNewFile bool) {
	m.mdns.mu.Lock()
	defer m.mdns.mu.Unlock()

	for peerID, sendChan := range m.mdns.sendChannels {
		// Prepare the file chunk request
		chunkReq := pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    req.filename,
					ChunkData:   chunk,
					Offset:      req.offset,
					IsNewFile:   isNewFile,
					TotalChunks: (m.Files[req.filename].filesize + conf.AppConfig.ChunkSize - 1) / conf.AppConfig.ChunkSize,
					TotalSize:   m.Files[req.filename].filesize,
				},
			},
		}

		// Enqueue the chunk request
		select {
		case sendChan <- &chunkReq:
			// Successfully enqueued
		default:
			log.Warnf("Send channel for peer %s is full, dropping chunk request", peerID)
		}
	}
}

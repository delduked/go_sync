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
	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
	"github.com/zeebo/xxh3"
)

type Meta struct {
	ChunkSize    int64
	Files        map[string]FileMetaData // map[filename][offset]Hash
	SaveChunks   chan MetaData
	DeleteChunks chan MetaData
	mdns         *Mdns
	db           *badger.DB // BadgerDB instance
	mu           sync.Mutex
	done         chan struct{}

	activeStreams    map[string]pb.FileSyncService_SyncFileClient // map[peerID]stream
	streamMu         sync.Mutex
	peerSendChannels map[string]chan pb.FileSyncRequest // map[peerID]sendChannel
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

// func (f FileMetaData) CurrentSize() int64 {
// 	return int64(len(f.hashes)) * conf.AppConfig.ChunkSize
// }
// func (f FileMetaData) UpdateFileSize() {
// 	f.filesize = int64(len(f.hashes)) * conf.AppConfig.ChunkSize
// }

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
		Files:        make(map[string]FileMetaData),
		SaveChunks:   make(chan MetaData, 100), // Buffered to prevent blocking
		DeleteChunks: make(chan MetaData, 100),
		mdns:         mdns,
		db:           db,
		mu:           sync.Mutex{},
		done:         make(chan struct{}), // Initialize the done channel

	}
}

func (m *Meta) Start(ctx context.Context, wg *sync.WaitGroup) error {
	// Initialize maps
	m.activeStreams = make(map[string]pb.FileSyncService_SyncFileClient)
	m.peerSendChannels = make(map[string]chan pb.FileSyncRequest)

	// Launch the ChunksToPeer goroutine only once
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.ChunksToPeer(ctx)
	}()

	// Initialize streams to all current peers
	for _, conn := range m.mdns.Clients {
		if conn.Target() == m.mdns.LocalIP {
			continue
		}
		peerID := conn.Target()

		stream, err := clients.SyncConn(conn)
		if err != nil {
			log.Errorf("Failed to open SyncFile stream on %s: %v", peerID, err)
			continue
		}

		m.streamMu.Lock()
		m.activeStreams[peerID] = stream
		m.streamMu.Unlock()

		// Create a send channel for the peer
		sendChan := make(chan pb.FileSyncRequest, 100) // Buffered to prevent blocking
		m.peerSendChannels[peerID] = sendChan

		// Launch a sender goroutine for the peer
		wg.Add(1)
		go func(peerID string, stream pb.FileSyncService_SyncFileClient, sendChan chan pb.FileSyncRequest) {
			defer wg.Done()
			m.peerSender(peerID, stream, sendChan)
		}(peerID, stream, sendChan)

		// Start a receiver goroutine for the stream
		wg.Add(1)
		go func(peerID string, stream pb.FileSyncService_SyncFileClient) {
			defer wg.Done()
			m.receiveResponses(peerID, stream)
		}(peerID, stream)
	}

	// Walk through the sync folder and process existing files
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
			err := m.CreateFileMetaData(path)
			if err != nil {
				log.Errorf("Failed to get metadata for file %s: %v", path, err)
				return nil // Continue scanning even if one file fails
			}
		}
		return nil
	})
}

func (m *Meta) CreateFileMetaData(fileName string) error {
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

	// Lock to prevent concurrent modifications
	m.mu.Lock()
	defer m.mu.Unlock()

	// If the file has shrunk, delete chunks from the metadata that no longer apply
	if fileMeta, exists := m.Files[fileName]; exists {
		if fileInfo.Size() < fileMeta.filesize {
			// File has shrunk, remove extra chunks
			log.Printf("File %s has shrunk from %d to %d", fileName, fileMeta.filesize, fileInfo.Size())
			for offset := fileInfo.Size(); offset < fileMeta.filesize; offset += conf.AppConfig.ChunkSize {
				err1, err2 := m.DeleteMetaData(fileName, offset)
				if err1 != nil || err2 != nil {
					log.Errorf("Failed to delete metadata for shrunk file at offset %d: %v, %v", offset, err1, err2)
				}
			}
		}
	}

	// Buffer to hold file chunks
	buffer := make([]byte, conf.AppConfig.ChunkSize)
	var offset int64 = 0

	for {
		// Move the file pointer to the current offset
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("error seeking to offset %d in file %s: %w", offset, fileName, err)
		}

		// Read the chunk from the current offset
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading file %s at offset %d: %w", fileName, offset, err)
		}

		if bytesRead == 0 {
			break // End of file
		}

		// Save the metadata for this chunk
		err = m.SaveMetaData(fileName, buffer[:bytesRead], offset)
		if err != nil {
			return fmt.Errorf("error saving metadata for file %s at offset %d: %w", fileName, offset, err)
		}

		// Move to the next chunk
		offset += conf.AppConfig.ChunkSize
	}

	// Update the filesize to the actual file size
	if fileMeta, exists := m.Files[fileName]; exists {
		fileMeta.filesize = fileInfo.Size()
		m.Files[fileName] = fileMeta
	} else {
		m.Files[fileName] = FileMetaData{
			hashes:   m.Files[fileName].hashes, // Or initialize accordingly
			filesize: fileInfo.Size(),
		}
	}

	return nil
}

// SaveMetaData will save the metadata to both in-memory map and the database
func (m *Meta) SaveMetaData(filename string, chunk []byte, offset int64) error {
	if pkg.IsTemporaryFile(filename) {
		return nil
	}

	oldMeta, err := m.GetMetaData(filename, offset)
	if err != nil {
		// No existing metadata, proceed to save new chunk
		log.Printf("No existing metadata for %s at offset %d, saving new metadata", filename, offset)
	} else {
		// Compare old and new hashes
		newWeakHash := pkg.NewRollingChecksum(chunk).Sum()
		newStrongHash := m.hashChunk(chunk)

		if oldMeta.Weakhash == newWeakHash && oldMeta.Stronghash == newStrongHash {
			// No change, skip saving this chunk
			log.Printf("Chunk at offset %d for file %s has not changed", offset, filename)
			return nil
		}

		log.Printf("Chunk at offset %d for file %s has changed", offset, filename)
	}

	m.saveMetaDataToMem(filename, chunk, offset)

	m.SaveChunks <- MetaData{
		filename: filename,
		offset:   offset,
	}

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
	m.DeleteChunks <- MetaData{
		filename: filename,
		offset:   offset,
	}
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

// handleStreamFailure manages the failure of a stream to a specific peer.
// It removes the failed stream, closes the send channel, and initiates reconnection attempts.
func (m *Meta) handleStreamFailure(peerID string) {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()

	// Remove the failed stream from activeStreams
	stream, exists := m.activeStreams[peerID]
	if exists {
		// Attempt to close the stream gracefully
		if err := stream.CloseSend(); err != nil {
			log.Errorf("Failed to close stream for peer %s: %v", peerID, err)
		}
		delete(m.activeStreams, peerID)
	}

	// Close the send channel to stop the sender goroutine
	sendChan, exists := m.peerSendChannels[peerID]
	if exists {
		close(sendChan)
		delete(m.peerSendChannels, peerID)
	}

	log.Infof("Handling stream failure for peer %s. Initiating reconnection attempts.", peerID)

	// Start reconnection attempts in a separate goroutine
	go m.attemptReconnection(peerID)
}

// peerSender listens on the send channel and sends messages via the stream.
func (m *Meta) peerSender(peerID string, stream pb.FileSyncService_SyncFileClient, sendChan <-chan pb.FileSyncRequest) {
	for {
		select {
		case msg := <-sendChan:
			if err := stream.Send(&msg); err != nil {
				log.Errorf("Failed to send message to peer %s: %v", peerID, err)
				// Handle stream failure, possibly attempt reconnection
				m.handleStreamFailure(peerID)
				return
			}
		case <-m.done:
			// Gracefully close the stream
			if err := stream.CloseSend(); err != nil {
				log.Errorf("Failed to close stream for peer %s: %v", peerID, err)
			}
			return
		}
	}
}

// SendDeleteToPeers enqueues delete requests to each peer's send channel.
func (m *Meta) SendDeleteToPeers(req MetaData) {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()

	for peerID, sendChan := range m.peerSendChannels {
		// Prepare the delete request
		deleteReq := pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: filepath.Base(req.filename),
					Offset:   req.offset, // 0 signifies entire file deletion
				},
			},
		}

		// Enqueue the delete request
		select {
		case sendChan <- deleteReq:
			// Successfully enqueued
		default:
			log.Warnf("Send channel for peer %s is full, dropping delete request", peerID)
		}
	}
}

// SendSaveToPeers enqueues save chunk requests to each peer's send channel.
func (m *Meta) SendSaveToPeers(req MetaData) {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()

	for peerID, sendChan := range m.peerSendChannels {
		// Read the specific chunk data
		chunkData, err := pkg.ReadChunk(req.filename, req.offset, conf.AppConfig.ChunkSize)
		if err != nil {
			log.Errorf("Failed to read chunk for %s at offset %d: %v", req.filename, req.offset, err)
			continue
		}

		// Prepare the file chunk request
		chunkReq := pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    filepath.Base(req.filename),
					ChunkData:   chunkData,
					Offset:      req.offset,
					IsNewFile:   false, // Determine based on context
					TotalChunks: (m.Files[req.filename].filesize + conf.AppConfig.ChunkSize - 1) / conf.AppConfig.ChunkSize,
					TotalSize:   m.Files[req.filename].filesize,
				},
			},
		}

		// Enqueue the chunk request
		select {
		case sendChan <- chunkReq:
			// Successfully enqueued
		default:
			log.Warnf("Send channel for peer %s is full, dropping chunk request", peerID)
		}
	}
}

// receiveResponses handles incoming responses from a specific peer.
func (m *Meta) receiveResponses(peerID string, stream pb.FileSyncService_SyncFileClient) {
	for {
		recv, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Infof("Stream closed gracefully by peer %s", peerID)
			} else {
				log.Errorf("Failed to receive response from %s: %v", peerID, err)
			}
			break
		}
		log.Infof("Received response from %s: %s", peerID, recv.Message)
	}

	// Handle stream closure, possibly attempt reconnection
	m.handleStreamFailure(peerID)
}

// attemptReconnection tries to reconnect to a peer after a failure.
func (m *Meta) attemptReconnection(peerID string) {
	for {
		select {
		case <-m.done:
			return
		default:
			// Attempt to retrieve the connection
			conn := m.mdns.GetConnByTarget(peerID) // Implement this method to retrieve connection
			if conn == nil {
				log.Errorf("No connection found for peer %s during reconnection", peerID)
				time.Sleep(5 * time.Second)
				continue
			}

			// Attempt to open a new stream
			stream, err := clients.SyncConn(conn)
			if err != nil {
				log.Errorf("Failed to reconnect SyncFile stream on %s: %v", peerID, err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Update activeStreams
			m.streamMu.Lock()
			m.activeStreams[peerID] = stream
			m.streamMu.Unlock()

			// Create a new send channel for the peer
			sendChan := make(chan pb.FileSyncRequest, 100) // Buffered
			m.streamMu.Lock()
			m.peerSendChannels[peerID] = sendChan
			m.streamMu.Unlock()

			// Launch a new sender goroutine for the peer
			go m.peerSender(peerID, stream, sendChan)

			// Start a new receiver goroutine for the stream
			go m.receiveResponses(peerID, stream)

			log.Infof("Reconnected SyncFile stream on %s", peerID)
			return
		}
	}
}

func (m *Meta) ChunksToPeer(ctx context.Context) {
	for {
		select {
		case saveChunk := <-m.SaveChunks:
			// Dispatch the save chunk to all peers
			m.SendSaveToPeers(saveChunk)
		case deleteChunk := <-m.DeleteChunks:
			// Dispatch the delete chunk to all peers
			m.SendDeleteToPeers(deleteChunk)
		case <-ctx.Done():
			// Signal all peer senders to terminate by closing their send channels
			log.Info("ChunksToPeer received shutdown signal. Closing all peer send channels.")
			m.streamMu.Lock()
			for peerID, sendChan := range m.peerSendChannels {
				close(sendChan)
				log.Infof("Closed send channel for peer %s", peerID)
				delete(m.peerSendChannels, peerID) // Optional: clean up the map
			}
			m.streamMu.Unlock()
			return
		}
	}
}

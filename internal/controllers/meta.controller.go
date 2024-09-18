package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/TypeTerrors/filesync/go_sync/conf"
	"github.com/TypeTerrors/filesync/go_sync/internal/clients"
	"github.com/TypeTerrors/filesync/go_sync/pkg"
	pb "github.com/TypeTerrors/filesync/go_sync/proto"

	"github.com/charmbracelet/log"
)

type MetaData struct {
	Chunks    map[int32]string // map[chunk position]hash
	ChunkSize int64
}

type Meta struct {
	MetaData map[string]MetaData
	PeerData *PeerData
	mu       sync.Mutex
}

// NewMeta initializes the Meta struct with the provided PeerData.
func NewMeta(peerdata *PeerData) *Meta {
	return &Meta{
		MetaData: make(map[string]MetaData),
		PeerData: peerdata,
	}
}

// AddFileMetaData adds or updates the metadata of a file, including chunk hash and position.
// It calculates the hash for the chunk and stores it in the MetaData map.
func (m *Meta) AddFileMetaData(file string, chunk []byte, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If the file exists, update its chunk hash, otherwise create a new entry
	if _, ok := m.MetaData[file]; ok {
		hash, pos := m.hashPosition(chunk, offset)
		m.MetaData[file].Chunks[pos] = hash
	} else {
		hash, pos := m.hashPosition(chunk, offset)
		m.MetaData[file] = MetaData{
			Chunks:    map[int32]string{pos: hash},
			ChunkSize: int64(len(chunk)),
		}
	}
}

// UpdateLocalMetaData periodically updates the metadata of all local files by reading each file and calculating the hash of its chunks.
func (m *Meta) UpdateLocalMetaData(wg *sync.WaitGroup, ctx context.Context) {
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
				asdf, err := m.getLocalFileMetadata(file, conf.ChunkSize)
				if err != nil {
					continue
				}

				// Skip empty metadata
				if len(asdf.Chunks) == 0 {
					continue
				}

				m.mu.Lock()
				m.MetaData[file] = asdf
				m.mu.Unlock()
			}
		}
	}
}

// UpdateFileMetaData updates a specific file's metadata by checking and hashing its chunks.
// It also writes the chunk to the file if it needs to be updated (e.g., for missing chunks).
func (m *Meta) UpdateFileMetaData(file string, chunkData []byte, offset int32, chunkSize int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metaData, ok := m.MetaData[file]
	if !ok {
		m.MetaData[file] = MetaData{
			Chunks:    make(map[int32]string),
			ChunkSize: chunkSize,
		}
	}

	// Calculate the new hash for the current chunk
	newHash := m.hashChunk(chunkData)

	// Compare the new hash with the old one, and update only if necessary
	if oldHash, exists := metaData.Chunks[offset]; exists && oldHash == newHash {
		return // No need to update if the hash is the same
	}

	metaData.Chunks[offset] = newHash
	m.MetaData[file] = metaData

	m.writeChunkToFile(file, chunkData, offset, chunkSize)
}

// DeleteFileMetaData removes the metadata of a file from the MetaData map.
func (m *Meta) DeleteFileMetaData(file string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.MetaData, file)
}

// ModifyPeerFile retrieves missing chunks for a peer's file and sends the missing chunks to the peer.
// This function compares the file metadata and sends chunks that are missing from the peer.
func (m *Meta) ModifyPeerFile(filename string, missingChunks []int32) error {
	fileMeta, ok := m.MetaData[filename]
	if !ok {
		return fmt.Errorf("file metadata not found for file: %s", filename)
	}

	m.PeerData.markFileAsInProgress(filename)

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	go func() {
		for _, chunkPos := range missingChunks {
			if _, exists := fileMeta.Chunks[chunkPos]; !exists {
				log.Errorf("chunk at position %d not found in metadata", chunkPos)
				return
			}

			chunkSize := fileMeta.ChunkSize
			offset := int64(chunkPos) * chunkSize

			chunkData := make([]byte, chunkSize)
			_, err := file.ReadAt(chunkData, offset)
			if err != nil {
				log.Errorf("failed to read chunk at position %d: %v", chunkPos, err)
				return
			}

			m.sendChunkToPeer(chunkData, chunkPos, chunkSize)
		}
	}()

	return nil
}

// PeerMetaData periodically retrieves metadata from peers and compares it with local files to identify and handle missing chunks.
func (m *Meta) PeerMetaData(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(20 * time.Second)
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
				peerChunks := m.getPeerFileMetaData(file)
				missingChunks := m.missingChunks(file, peerChunks)

				if len(missingChunks) > 0 {
					err := m.ModifyPeerFile(file, missingChunks)
					if err != nil {
						log.Errorf("failed to modify peer file: %v", err)
					}
				}
			}
		}
	}
}

// getPeerFileMetaData retrieves metadata for a given file from all peers.
func (m *Meta) getPeerFileMetaData(file string) map[string]MetaData {
	var peerFileMetaData []*pb.FileMetaData
	metas := make(map[string]MetaData)
	var wg sync.WaitGroup

	for _, ip := range m.PeerData.Clients {
		wg.Add(1)

		go func(ip string) {
			defer wg.Done()

			stream, err := clients.MetaDataStream(ip)
			if err != nil {
				log.Errorf("failed to open stream for metadata request on %s: %v", ip, err)
				return
			}

			for {
				recv, err := stream.Recv()
				if err != nil {
					if err == io.EOF {
						break
					} else {
						log.Errorf("Failed to receive metadata from peer %s: %v", ip, err)
					}
					return
				}

				log.Printf("Received metadata from peer %s: %v", ip, recv)

				m.mu.Lock()
				peerFileMetaData = append(peerFileMetaData, recv)

				metaData := metas[ip]

				if metaData.Chunks == nil {
					metaData.Chunks = make(map[int32]string)
				}

				metaData.Chunks[recv.ChunkNumber] = recv.ChunkHash
				metaData.ChunkSize = recv.ChunkSize

				metas[ip] = metaData
				m.mu.Unlock()
			}

			err = stream.Send(&pb.MetaDataReq{
				FileName: file,
			})
			if err != nil {
				log.Errorf("failed to send metadata request to peer %s: %v", ip, err)
				return
			}
		}(ip)
	}

	wg.Wait()
	return metas
}

// sendChunkToPeer sends a specific chunk of data to all peers.
func (m *Meta) sendChunkToPeer(chunk []byte, chunkPos int32, chunkSize int64) {
	for _, ip := range m.PeerData.Clients {
		stream, err := clients.ModifyStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", ip, err)
			return
		}

		go func() {
			recv, err := stream.Recv()
			if err != nil {
				log.Printf("Error receiving message from peer %s: %v", ip, err)
				return
			}
			if err == io.EOF {
				log.Printf("Stream closed by peer %s", ip)
				return
			}

			log.Printf("Received message from peer %s: %v", ip, recv.Message)
		}()

		err = stream.Send(&pb.FileChunk{
			ChunkData:   chunk,
			ChunkNumber: chunkPos,
			ChunkSize:   chunkSize,
		})
		if err != nil {
			log.Printf("Error sending chunk to peer %s: %v", ip, err)
			return
		}
	}
}

// missingChunks compares local file chunks with a peer's chunks and identifies missing chunks.
func (m *Meta) missingChunks(file string, peer map[string]MetaData) []int32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	localMetaData, okA := m.MetaData[file]
	peerMetaData, okB := peer[file]

	if !okA || !okB {
		return nil
	}

	var missingChunks []int32

	for posA, hashA := range localMetaData.Chunks {
		if hashB, existsInB := peerMetaData.Chunks[posA]; !existsInB || hashA != hashB {
			missingChunks = append(missingChunks, posA)
		}
	}

	return missingChunks
}

// writeChunkToFile writes a chunk to the appropriate position in a file based on its chunk number.
func (m *Meta) writeChunkToFile(file string, chunkData []byte, chunkPosition int32, chunkSize int64) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	offset := int64(chunkPosition) * chunkSize

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

// hashPosition calculates the hash for a file chunk and returns its hash and position.
func (m *Meta) hashPosition(chunk []byte, offset int64) (string, int32) {
	hash := m.hashChunk(chunk)
	pos := m.chunkPosition(chunk, offset)
	return hash, pos
}

// hashChunk calculates the SHA-256 hash of a given chunk.
func (m *Meta) hashChunk(chunk []byte) string {
	hasher := sha256.New()
	hasher.Write(chunk)
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)
}

// chunkPosition calculates the position of a chunk based on its offset in the file.
func (m *Meta) chunkPosition(chunk []byte, offset int64) int32 {
	return int32(offset / int64(len(chunk)))
}

// getLocalFileMetadata retrieves metadata for a local file by reading it chunk by chunk.
func (m *Meta) getLocalFileMetadata(fileName string, chunkSize int64) (MetaData, error) {
	var res MetaData

	// Open the file for reading
	file, err := os.Open(fileName)
	if err != nil {
		return res, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	// Initialize metadata for the file
	fileMeta := MetaData{
		Chunks:    make(map[int32]string),
		ChunkSize: chunkSize,
	}

	buffer := make([]byte, chunkSize)
	var chunkIndex int32 = 0

	// Read the file chunk by chunk
	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return res, fmt.Errorf("error reading file: %w", err)
		}
		if bytesRead == 0 {
			break // End of file
		}

		// Hash the chunk and store it in the metadata
		hasher := sha256.New()
		hasher.Write(buffer[:bytesRead])
		hash := hex.EncodeToString(hasher.Sum(nil))

		// Store the chunk hash in the metadata
		fileMeta.Chunks[chunkIndex] = hash

		// Move to the next chunk
		chunkIndex++
	}

	return fileMeta, nil
}

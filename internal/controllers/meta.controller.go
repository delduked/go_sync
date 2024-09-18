package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	pb "go_sync/filesync"
	"go_sync/internal/clients"
	"go_sync/pkg"
	"io"
	"os"
	"sync"
	"time"

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

func NewMeta(peerdata *PeerData) *Meta {
	return &Meta{
		MetaData: make(map[string]MetaData),
		PeerData: peerdata,
	}
}

// Add new file with metadata to the MetaData map
func (m *Meta) AddFileMetaData(file string, chunk []byte, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

func (m *Meta) UpdateFileChunks(file string, chunkData []byte, offset int32, chunkSize int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metaData, ok := m.MetaData[file]
	if !ok {
		m.MetaData[file] = MetaData{
			Chunks:    make(map[int32]string),
			ChunkSize: chunkSize,
		}
	}

	newHash := m.hashChunk(chunkData)

	if oldHash, exists := metaData.Chunks[offset]; exists && oldHash == newHash {
		return
	}

	metaData.Chunks[offset] = newHash
	m.MetaData[file] = metaData

	m.writeChunkToFile(file, chunkData, offset, chunkSize)
}

func (m *Meta) DeleteFileMetaData(file string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.MetaData, file)
}

// compares the missing chunks between two files from two different peers
func (m *Meta) ModifyPeerFile(filename string, missingChunks []int32) error {
	// Retrieve file metadata
	fileMeta, ok := m.MetaData[filename]
	if !ok {
		return fmt.Errorf("file metadata not found for file: %s", filename)
	}

	m.PeerData.markFileAsInProgress(filename)

	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	go func() {

		// Iterate over the missing chunks
		for _, chunkPos := range missingChunks {
			// Check if the chunk exists in the file metadata
			if _, exists := fileMeta.Chunks[chunkPos]; !exists {
				log.Errorf("chunk at position %d not found in metadata", chunkPos)
				return
			}

			// Retrieve the chunk size from the metadata
			chunkSize := fileMeta.ChunkSize

			// Use the chunk position as the offset directly
			offset := int64(chunkPos) * chunkSize

			// Read the chunk data from the file
			chunkData := make([]byte, chunkSize)
			_, err := file.ReadAt(chunkData, offset)
			if err != nil {
				log.Errorf("failed to read chunk at position %d: %v", chunkPos, err)
				return
			}

			// Send the chunk to the peer
			m.sendChunkToPeer(chunkData, chunkPos, chunkSize)

		}
	}()

	return nil
}

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

func (m *Meta) getPeerFileMetaData(file string) map[string]MetaData {
	var peerFileMetaData []*pb.FileMetaData
	var wg sync.WaitGroup
	metas := make(map[string]MetaData, 0)
	for _, ip := range m.PeerData.Clients {

		stream, err := clients.MetaDataStream(ip)
		if err != nil {
			log.Errorf("failed to open stream for metadata request on %s: %v", ip, err)
			continue
		}

		wg.Add(1)


		// i messed this up
		// this needs to send a request first and then receiv ethe message
		// because this is a bidi stream


		go func(ip string) {
			defer wg.Done()
			for {
				recv := stream.Send(&pb.MetaDataReq{
					FileName: file,
				})
				if err != nil {
					if err == io.EOF {
						log.Printf("Stream closed by peer %s", ip)
					} else {
						log.Errorf("Failed to receive metadata from peer %s: %v", ip, err)
					}
					return
				}

				log.Printf("Received metadata from peer %s: %v", ip, recv)

				m.mu.Lock()
				peerFileMetaData = append(peerFileMetaData, recv)
				m.mu.Unlock()
				metas[ip] = MetaData{}
			}
		}(ip)
	}

	wg.Wait()

	for _, value := range metas {
		for _, v := range peerFileMetaData {
			value.Chunks[v.ChunkNumber] = v.ChunkHash
			value.ChunkSize = v.ChunkSize
		}
	}
	
	return metas
}

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

func (m *Meta) missingChunks(file string, peer map[string]MetaData) []int32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	localMetaData, okA := m.MetaData[file]
	peerMetaData, okB := peer[file]

	if !okA || !okB {
		return nil
	}

	var missingChunks []int32

	// Compare chunks in MetaData for the file
	for posA, hashA := range localMetaData.Chunks {
		if hashB, existsInB := peerMetaData.Chunks[posA]; !existsInB || hashA != hashB {
			missingChunks = append(missingChunks, posA)
		}
	}

	// Update the other peer with the missing chunks
	return missingChunks
}

func (m *Meta) writeChunkToFile(file string, chunkData []byte, chunkPosition int32, chunkSize int64) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Calculate the byte offset where the chunk should be written
	offset := int64(chunkPosition) * chunkSize

	// Seek to the correct position in the file
	_, err = f.Seek(offset, 0)
	if err != nil {
		return err
	}

	// Write the chunk to the file at the specified offset
	_, err = f.Write(chunkData)
	if err != nil {
		return err
	}

	return nil
}

func (m *Meta) hashPosition(chunk []byte, offset int64) (string, int32) {
	hash := m.hashChunk(chunk)
	pos := m.chunkPosition(chunk, offset)
	return hash, pos
}

func (m *Meta) hashChunk(chunk []byte) string {
	hasher := sha256.New()
	hasher.Write(chunk)
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)
}

func (m *Meta) chunkPosition(chunk []byte, offset int64) int32 {
	return int32(offset / int64(len(chunk)))
}

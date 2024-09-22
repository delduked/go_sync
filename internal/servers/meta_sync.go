package servers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/TypeTerrors/go_sync/internal/clients"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
)

// ModifyPeerFile retrieves missing chunks for a peer's file and sends the missing chunks to the peer.
func (m *Meta) ModifyPeerFile(filename string, missingOffsets []int64) error {
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
		for _, offset := range missingOffsets {
			if _, exists := fileMeta.Chunks[offset]; !exists {
				log.Errorf("chunk at offset %d not found in metadata", offset)
				return
			}
			chunkSize := fileMeta.ChunkSize
			chunkData := make([]byte, chunkSize)
			_, err := file.ReadAt(chunkData, offset)
			if err != nil {
				log.Errorf("failed to read chunk at offset %d: %v", offset, err)
				return
			}
			m.sendChunkToPeer(chunkData, offset, chunkSize)
		}
	}()
	return nil
}

// sendChunkToPeer sends a specific chunk of data to all peers.
func (m *Meta) sendChunkToPeer(chunk []byte, offset int64, chunkSize int64) {
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

			peerChunkPos := recv.Offset
			peerChunkHash := recv.ChunkHash

			if m.MetaData[recv.FileName].Chunks[peerChunkPos] == peerChunkHash {
				log.Printf("Chunk %d matches on peer %s", peerChunkPos, ip)
				return
			} else {
				log.Printf("Chunk %d does not match on peer %s", peerChunkPos, ip)
			}
		}()

		err = stream.Send(&pb.MetaDataChunk{
			ChunkData: chunk,
			Offset:    offset,
			ChunkSize: chunkSize,
		})
		if err != nil {
			log.Printf("Error sending chunk to peer %s: %v", ip, err)
			return
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
			go func() {
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
						metaData.Chunks = make(map[int64]string)
					}

					metaData.Chunks[recv.Offset] = recv.ChunkHash
					metaData.ChunkSize = recv.ChunkSize

					metas[ip] = metaData
					m.mu.Unlock()
				}
			}()

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

// missingChunks compares local file chunks with a peer's chunks and identifies missing chunks.
func (m *Meta) missingChunks(file string, peer map[string]MetaData) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	localMetaData, okA := m.MetaData[file]
	peerMetaData, okB := peer[file]

	if !okA || !okB {
		return nil
	}

	var missingChunks []int64

	for posA, hashA := range localMetaData.Chunks {
		if hashB, existsInB := peerMetaData.Chunks[posA]; !existsInB || hashA != hashB {
			missingChunks = append(missingChunks, posA)
		}
	}

	return missingChunks
}

// getLocalFileMetadata retrieves metadata for a local file by reading it chunk by chunk.
func (m *Meta) getLocalFileMetadata(fileName string, chunkSize int64) (MetaData, error) {
	var res MetaData
	file, err := os.Open(fileName)
	if err != nil {
		return res, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	fileMeta := MetaData{
		Chunks:    make(map[int64]string),
		ChunkSize: chunkSize,
	}

	buffer := make([]byte, chunkSize)
	var chunkIndex int64 = 0

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return res, fmt.Errorf("error reading file: %w", err)
		}
		if bytesRead == 0 {
			break // End of file
		}

		hasher := sha256.New()
		hasher.Write(buffer[:bytesRead])
		hash := hex.EncodeToString(hasher.Sum(nil))

		// Store the chunk hash in the metadata
		fileMeta.Chunks[chunkIndex] = hash
		chunkIndex++
	}

	return fileMeta, nil
}

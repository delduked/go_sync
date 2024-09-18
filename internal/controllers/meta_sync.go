package controllers

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/TypeTerrors/go_sync/internal/clients"
	"github.com/TypeTerrors/go_sync/pkg"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
)

// ModifyPeerFile retrieves missing chunks for a peer's file and sends the missing chunks to the peer.
func (m *Meta) ModifyPeerFile(filename string, missingChunks []int64) error {
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
			chunkData := make([]byte, chunkSize)
			_, err := file.ReadAt(chunkData, chunkPos)
			if err != nil {
				log.Errorf("failed to read chunk at offset %d: %v", chunkPos, err)
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
				if m.PeerData.IsFileInProgress(file) {
					continue
				}
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

// sendChunkToPeer sends a specific chunk of data to all peers.
func (m *Meta) sendChunkToPeer(chunk []byte, chunkPos int64, chunkSize int64) {
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

		err = stream.Send(&pb.MetaDataChunk{
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

					metaData.Chunks[recv.ChunkNumber] = recv.ChunkHash
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

package controllers

import (
	"io"
	"os"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	"github.com/TypeTerrors/go_sync/internal/clients"
	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
)

// Start streaming a file to all peers
// Stream the file as it's being written
func (s *State) startStreamingFileInChunks(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Error getting file info for %s: %v", filePath, err)
		return
	}

	var offset int64 = 0

	// Stream file in chunks
	for {
		buffer := make([]byte, conf.ChunkSize)
		bytesRead, err := file.ReadAt(buffer, offset)

		if err != nil && err != io.EOF {
			log.Printf("Error reading chunk from file %s: %v", filePath, err)
			return
		}

		if bytesRead == 0 {
			// File is not growing, check if writing is done
			newFileInfo, err := file.Stat()
			if err != nil {
				log.Printf("Error getting updated file info for %s: %v", filePath, err)
				return
			}

			if newFileInfo.Size() == fileInfo.Size() {
				// File size has not changed, likely done
				log.Printf("File %s fully streamed", filePath)
				s.sharedData.markFileAsComplete(filePath)
				//s.sharedData.IsFileInProgress(filePath)
				break
			} else {
				// File is still growing, update file size info and continue
				fileInfo = newFileInfo
			}

			time.Sleep(1 * time.Second) // Wait before checking for more chunks
			continue
		}

		// Send the chunk to peers
		s.sendChunkToPeers(filePath, buffer[:bytesRead], offset, fileInfo.Size())

		// Update offset for next chunk
		offset += int64(bytesRead)
	}
}

func (s *State) sendChunkToPeers(fileName string, chunk []byte, offset, fileSize int64) {
	log.Printf("Sending chunk of file %s at offset %d", fileName, offset)

	s.sharedData.mu.RLock()
	peers := make([]string, len(s.sharedData.Clients))
	copy(peers, s.sharedData.Clients)
	s.sharedData.mu.RUnlock()

	for _, ip := range peers {

		stream, err := clients.SyncStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %s: %v", ip, err)
			continue
		}

		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileChunk{
				FileChunk: &pb.FileChunk{
					FileName:    fileName,
					ChunkData:   chunk,
					ChunkNumber: int32(offset / int64(len(chunk))),
					TotalChunks: int32((fileSize + int64(len(chunk)) - 1) / int64(len(chunk))),
				},
			},
		})
		if err != nil {
			log.Printf("Error sending chunk to peer %s: %v", ip, err)
		}
	}
}

func (s *State) streamDelete(fileName string) {
	log.Printf("Processing delete for file: %s", fileName)

	// Ensure we mark the file as completed, to stop any further transfers
	s.sharedData.markFileAsComplete(fileName)

	for peer, ip := range s.sharedData.Clients {
		log.Printf("Deleting file %v on peer %v", fileName, ip)

		stream, err := clients.SyncStream(ip)
		if err != nil {
			log.Printf("Error starting stream to peer %v: %v", ip, err)
			continue
		}

		go func() {
			for {
				recv, err := stream.Recv()
				if err != nil {
					log.Printf("Error receiving response from %v: %v", ip, err)
					break
				}
				if err == io.EOF {
					log.Printf("Stream closed by %v", ip)
					break
				}

				log.Printf("Received response from %v: %v", ip, recv.Message)
			}
		}()

		// Send delete request to peer
		err = stream.Send(&pb.FileSyncRequest{
			Request: &pb.FileSyncRequest_FileDelete{
				FileDelete: &pb.FileDelete{
					FileName: fileName,
				},
			},
		})
		if err != nil {
			log.Printf("Error sending delete request to peer %v: %v", peer, err)
		}
	}
}

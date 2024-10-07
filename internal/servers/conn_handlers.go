package servers

import (
	"github.com/charmbracelet/log"

	pb "github.com/TypeTerrors/go_sync/proto"
)

func (c *Conn) handleFileSyncResponse(peer *Peer, msg *pb.FileSyncResponse) {
	// Implement your logic here
	log.Infof("Received FileSyncResponse from peer %s: %s", peer.ID, msg.Message)
	// For example, update local state, log, or trigger other actions
}
func (c *Conn) handleHealthCheckResponse(msg *pb.Pong) {
	// Implement your logic here
	log.Debugf("Received pong sent from %s to %s at %v", msg.To, msg.From, msg.Time)
}

func (c *Conn) handleMetadataResponse(peer *Peer, msg *pb.MetadataResponse) {
	// Implement your logic here
	log.Infof("Received MetadataResponse from peer %s for file %s", peer.ID, msg.FileName)
	// Process the metadata, compare with local data, etc.
}

func (c *Conn) handleGetMissingFileResponse(peer *Peer, chunk *pb.FileChunk) error {
	filePath := chunk.FileName
	c.file.markFileAsInProgress(filePath)

	log.Debugf("Received GetMissingFileResponse from peer %s for file %s at offset %d", peer.ID, chunk.FileName, chunk.Offset)

	// Open or get the file
	file, err := c.file.getOrOpenFile(chunk.FileName)
	if err != nil {
		log.Errorf("Failed to get or open file %s: %v", filePath, err)
		return err
	}

	// Write the chunk data at the specified offset
	_, err = file.WriteAt(chunk.ChunkData, chunk.Offset)
	if err != nil {
		log.Errorf("Failed to write to file %s at offset %d: %v", filePath, chunk.Offset, err)
		return err
	} else {
		log.Debugf("Wrote chunk to file %s at offset %d", filePath, chunk.Offset)
	}

	// Update the metadata
	c.SaveMetaData(filePath, chunk.ChunkData, chunk.Offset)

	// Check if all chunks have been received
	if c.meta.allChunksReceived(chunk.FileName, chunk.TotalChunks) {
		log.Debugf("All chunks received for file %s", chunk.FileName)
		c.file.markFileAsComplete(chunk.FileName)
	}

	log.Debugf("Received and wrote chunk for file %s at offset %d", chunk.FileName, chunk.Offset)
	return nil
}

func (c *Conn) handleChunkResponse(peer *Peer, msg *pb.ChunkResponse) {
	// Implement your logic here
	log.Infof("Received ChunkResponse from peer %s for file %s at offset %d", peer.ID, msg.FileName, msg.Offset)
	// Write the chunk data to file, verify integrity, etc.
}

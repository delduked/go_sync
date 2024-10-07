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
	log.Debugf("Received pong sent from %s to %s at %v", msg.From, msg.To, msg.Time)
}

func (c *Conn) handleMetadataResponse(peer *Peer, msg *pb.MetadataResponse) {
	// Implement your logic here
	log.Infof("Received MetadataResponse from peer %s for file %s", peer.ID, msg.FileName)
	// Process the metadata, compare with local data, etc.
}
func (c *Conn) handleGetMissingFileResponse(peer *Peer, msg *pb.FileChunk) {
	// Implement your logic here
	log.Infof("Received ChunkResponse from peer %s for file %s at offset %d", peer.ID, msg.FileName, msg.Offset)
	// Write the chunk data to file, verify integrity, etc.

	c.file.markFileAsInProgress(msg.FileName)
	c.handleFileChunk(msg)
}

func (c *Conn) handleChunkResponse(peer *Peer, msg *pb.ChunkResponse) {
	// Implement your logic here
	log.Infof("Received ChunkResponse from peer %s for file %s at offset %d", peer.ID, msg.FileName, msg.Offset)
	// Write the chunk data to file, verify integrity, etc.
}

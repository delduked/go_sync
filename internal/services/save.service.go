package services

import (
	pb "go_sync/filesync" // Import your protobufs here
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func Save(req *pb.FileChunk, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Join("./sync_folder", req.FileName)
	log.Printf("Saving file chunk: %s", filePath)

	// Open the file (or create if it doesn't exist), and append the chunk
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Error opening file: %v", err)
		return
	}
	defer file.Close()

	// Write the chunk to the file
	_, err = file.Write(req.ChunkData)
	if err != nil {
		log.Errorf("Error writing file: %v", err)
		return
	}

	// Send an acknowledgment back to the client
	err = stream.SendMsg(&pb.FileSyncResponse{
		Message: "File chunk saved successfully",
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}

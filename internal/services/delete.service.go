package services

import (
	pb "go_sync/filesync" // Import your protobufs here
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func Delete(req *pb.FileDelete, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	filePath := filepath.Join("./sync_folder", req.FileName)
	log.Printf("Deleting file: %s", filePath)

	// Attempt to delete the file
	err := os.Remove(filePath)
	if err != nil {
		log.Errorf("Error deleting file: %v", err)
		return
	}

	// Send an acknowledgment back to the client
	err = stream.SendMsg(&pb.FileSyncResponse{
		Message: "File deleted successfully",
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}

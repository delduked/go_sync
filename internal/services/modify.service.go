package services

import (
	pb "go_sync/filesync" // Import your protobufs here
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func Modify(req *pb.FileRename, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	oldPath := filepath.Join("./sync_folder", req.OldName)
	newPath := filepath.Join("./sync_folder", req.NewName)
	log.Printf("Renaming file: %s -> %s", oldPath, newPath)

	// Attempt to rename the file
	err := os.Rename(oldPath, newPath)
	if err != nil {
		log.Errorf("Error renaming file: %v", err)
		return
	}

	// Send an acknowledgment back to the client
	err = stream.SendMsg(&pb.FileSyncResponse{
		Message: "File renamed successfully",
	})
	if err != nil {
		log.Errorf("Error sending acknowledgment: %v", err)
	}
}

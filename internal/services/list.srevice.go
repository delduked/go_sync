package services

import (
	pb "go_sync/filesync"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func List(req *pb.FileList, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	log.Infof("Received file list")


	// Respond to the list request with a status message
	err := stream.SendMsg(&pb.FileSyncResponse{
		Message: "List request received successfully",
	})
	if err != nil {
		log.Errorf("Error sending list response: %v", err)
	}

}

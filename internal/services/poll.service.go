package services

import (
	pb "go_sync/filesync" // Import your protobufs here

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func Poll(req *pb.Poll, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	log.Printf("Received poll request: %s", req.Message)

	// Respond to the poll with a status message
	err := stream.SendMsg(&pb.FileSyncResponse{
		Message: "Poll request received successfully",
	})
	if err != nil {
		log.Errorf("Error sending poll response: %v", err)
	}
}

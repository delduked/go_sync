package services

import (
	pb "go_sync/filesync" // Import your protobufs here

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

func Ack(req *pb.Acknowledgment, stream grpc.BidiStreamingServer[pb.FileSyncRequest, pb.FileSyncResponse]) {
	log.Printf("Acknowledgement from: %s", req.Message)
}

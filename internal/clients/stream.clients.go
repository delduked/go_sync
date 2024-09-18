package clients

import (
	"context"

	pb "github.com/TypeTerrors/filesync/go_sync/proto"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

// SyncStream creates a bidirectional streaming client for syncing files
// between peers
func SyncStream(ip string) (grpc.BidiStreamingClient[pb.FileSyncRequest, pb.FileSyncResponse], error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Errorf("failed to connect to gRPC server at %v: %v", ip, err)
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.SyncFiles(context.Background())
	if err != nil {
		log.Errorf("Failed to open stream for list check on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}

// StateStream creates a bidirectional streaming client for boradcasting the local state
// of the local file system to other peers
func StateStream(ip string) (grpc.ServerStreamingClient[pb.StateRes], error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Errorf("failed to connect to gRPC server at %v: %v", ip, err)
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.State(context.Background(), &pb.StateReq{})
	if err != nil {
		log.Errorf("Failed to open stream for list check on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}

func ModifyStream(ip string) (grpc.BidiStreamingClient[pb.FileChunk, pb.FileSyncResponse], error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Errorf("failed to connect to gRPC server at %v: %v", ip, err)
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.ModifyFiles(context.Background())
	if err != nil {
		log.Errorf("Failed to open stream for list check on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}
func MetaDataStream(ip string) (grpc.BidiStreamingClient[pb.MetaDataReq, pb.FileMetaData], error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Errorf("failed to connect to gRPC server at %v: %v", ip, err)
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.MetaData(context.Background())
	if err != nil {
		log.Errorf("Failed to open stream for list check on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}

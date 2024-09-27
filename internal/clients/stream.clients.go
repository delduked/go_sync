package clients

import (
	"context"

	pb "github.com/TypeTerrors/go_sync/proto"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
)

// func dialGRPC(ip string) (*grpc.ClientConn, error) {
// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()
// 	conn, err := grpc.NewClient(, ip, grpc.WithInsecure(), grpc.WithBlock())
// 	if err != nil {
// 		log.Errorf("Failed to connect to gRPC server at %v: %v", ip, err)
// 		return nil, err
// 	}
// 	return conn, nil
// }

// SyncStream creates a bidirectional streaming client for syncing files between peers
func SyncStream(ip string) (pb.FileSyncService_SyncFileClient, error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.SyncFile(context.Background())
	if err != nil {
		log.Errorf("Failed to open SyncFile stream on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}
func ExchangeMetadataStream(ip string) (pb.FileSyncService_ExchangeMetadataClient, error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.ExchangeMetadata(context.Background())
	if err != nil {
		log.Errorf("Failed to open ExchangeMetadata stream on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}
func RequestChunksStream(ip string) (pb.FileSyncService_RequestChunksClient, error) {
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	client := pb.NewFileSyncServiceClient(conn)
	stream, err := client.RequestChunks(context.Background())
	if err != nil {
		log.Errorf("Failed to open RequestChunks stream on %s: %v", conn.Target(), err)
		return nil, err
	}
	return stream, nil
}

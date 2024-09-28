package clients

import (
	"fmt"
	"sync"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
)

// StreamHandler defines a generic function signature for handling streams with requests and responses.
type StreamHandler[Req any, Res any] func(stream grpc.ClientStream, req Req) error

// NewClientStream handles gRPC streaming, with concurrent sending and receiving using generics.
func NewClientStream[Req any, Res any](
	ip string,
	streamCreator func(pb.FileSyncServiceClient) (grpc.ClientStream, error),
	sendHandler StreamHandler[Req, Res],
	receiveHandler func(res Res) error,
	requests []Req,
) error {
	// Create a new gRPC connection
	conn, err := grpc.NewClient(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server at %s: %v", ip, err)
	}
	defer conn.Close()

	client := pb.NewFileSyncServiceClient(conn)

	stream, err := streamCreator(client)
	if err != nil {
		return fmt.Errorf("failed to create stream: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			var res Res
			err := stream.RecvMsg(&res)
			if err != nil {
				fmt.Printf("Error receiving from stream: %v", err)
				return
			}

			err = receiveHandler(res)
			if err != nil {
				fmt.Printf("Error handling received data: %v", err)
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for _, req := range requests {
			err := sendHandler(stream, req)
			if err != nil {
				fmt.Printf("Error sending request: %v", err)
				return
			}
		}

		stream.CloseSend()
	}()

	wg.Wait()

	return nil
}

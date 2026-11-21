package servers

import (
	"context"
	"io"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const manifestBatchSize = 128

// Bound individual RPC messages in both directions; a large project must not
// cross gRPC's unary 4 MiB limit just by accumulating files or deletion history.
func (s *Service) ExchangeManifest(stream pb.FileSyncService_ExchangeManifestServer) error {
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(batch.KnownPaths) > manifestBatchSize || len(batch.Files) > 0 {
			return status.Error(codes.InvalidArgument, "invalid manifest request batch")
		}
		if err = s.Engine.RestoreDeletionFloors(batch.KnownPaths); err != nil {
			return err
		}
	}
	list, err := s.Engine.Manifest()
	if err != nil {
		return err
	}
	for len(list.Files) > 0 {
		n := min(manifestBatchSize, len(list.Files))
		if err = stream.Send(&pb.ManifestBatch{Files: list.Files[:n]}); err != nil {
			return err
		}
		list.Files = list.Files[n:]
	}
	return nil
}
func fetchManifest(ctx context.Context, client pb.FileSyncServiceClient, known []string) (*pb.FileList, error) {
	list, err := streamManifest(ctx, client, known)
	if status.Code(err) == codes.Unimplemented {
		return client.ListFiles(ctx, &pb.Empty{KnownPaths: known})
	}
	return list, err
}
func streamManifest(ctx context.Context, client pb.FileSyncServiceClient, known []string) (*pb.FileList, error) {
	stream, err := client.ExchangeManifest(ctx)
	if err != nil {
		return nil, err
	}
	for len(known) > 0 {
		n := min(manifestBatchSize, len(known))
		if err = stream.Send(&pb.ManifestBatch{KnownPaths: known[:n]}); err != nil {
			if err == io.EOF {
				_, err = stream.Recv()
			}
			return nil, err
		}
		known = known[n:]
	}
	if err = stream.CloseSend(); err != nil {
		return nil, err
	}
	list := &pb.FileList{}
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return list, nil
		}
		if err != nil {
			return nil, err
		}
		if len(batch.Files) > manifestBatchSize || len(batch.KnownPaths) > 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid manifest response batch")
		}
		list.Files = append(list.Files, batch.Files...)
	}
}

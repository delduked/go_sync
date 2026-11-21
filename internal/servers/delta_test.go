package servers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type measuredClient struct {
	pb.FileSyncServiceClient
	bytes atomic.Int64
}
type measuredStream struct {
	pb.FileSyncService_ReadDeltaClient
	total *atomic.Int64
}

func (c *measuredClient) ReadDelta(ctx context.Context, opts ...grpc.CallOption) (pb.FileSyncService_ReadDeltaClient, error) {
	s, err := c.FileSyncServiceClient.ReadDelta(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &measuredStream{s, &c.bytes}, nil
}
func (s *measuredStream) Recv() (*pb.FileChunk, error) {
	chunk, err := s.FileSyncService_ReadDeltaClient.Recv()
	if err == nil {
		s.total.Add(int64(len(chunk.ChunkData)))
	}
	return chunk, err
}

func TestDeltaTransfersOnlyChangedBlocks(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	client := &measuredClient{FileSyncServiceClient: serve(t, a)}
	a.client = client
	// More than one basis-hash batch, plus an eventual partial final block.
	data := bytes.Repeat([]byte("abcdefgh"), 9*1024*1024/8)
	write(t, a, "large", data)
	pull(t, b, a)
	if got := client.bytes.Swap(0); got != int64(len(data)) {
		t.Fatalf("initial payload: %d", got)
	}
	data[3*defaultBlockSize+17] = 'X'
	write(t, a, "large", data)
	pull(t, b, a)
	equalFile(t, b, "large", data)
	if got := client.bytes.Swap(0); got != defaultBlockSize {
		t.Fatalf("one-byte edit sent %d, want one block", got)
	}
	data = append(data, bytes.Repeat([]byte("y"), 1000)...)
	write(t, a, "large", data)
	pull(t, b, a)
	equalFile(t, b, "large", data)
	if got := client.bytes.Swap(0); got != 1000 {
		t.Fatalf("aligned append sent %d", got)
	}
	data = append(data, bytes.Repeat([]byte("z"), 4000)...)
	write(t, a, "large", data)
	pull(t, b, a)
	equalFile(t, b, "large", data)
	if got := client.bytes.Swap(0); got != 5000 {
		t.Fatalf("partial-block append sent %d", got)
	}
	data = data[:2048]
	write(t, a, "large", data)
	pull(t, b, a)
	equalFile(t, b, "large", data)
	if got := client.bytes.Swap(0); got != 0 {
		t.Fatalf("prefix truncation resent %d bytes", got)
	}
	os.Chmod(filepath.Join(a.dir, "large"), 0755)
	a.e.Scan()
	pull(t, b, a)
	if got := client.bytes.Load(); got != 0 {
		t.Fatalf("permission-only change resent %d bytes", got)
	}
}

func TestAdvertisedPrefixSurvivesNewerAppends(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	client := &measuredClient{FileSyncServiceClient: serve(t, a)}
	a.client = client
	prefix := bytes.Repeat([]byte("a"), defaultBlockSize*2)
	write(t, a, "download", prefix)
	advertised := copyEntry(a.e.files["download"])
	latest := append(append([]byte{}, prefix...), bytes.Repeat([]byte("b"), defaultBlockSize)...)
	write(t, a, "download", latest) // Source metadata has advanced before the receiver starts.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.e.pull(ctx, client, advertised, nil); err != nil {
		t.Fatal(err)
	}
	equalFile(t, b, "download", prefix)
	client.bytes.Store(0)
	pull(t, b, a)
	equalFile(t, b, "download", latest)
	if got := client.bytes.Load(); got != defaultBlockSize {
		t.Fatalf("catch-up sent %d bytes", got)
	}
	backups, _ := filepath.Glob(filepath.Join(b.state, "versions", "*"))
	if len(backups) != 0 {
		t.Fatal("same-origin append archived full previous snapshots")
	}
}

func TestRewrittenPrefixAbortsWithoutPublishing(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	client := serve(t, a)
	original := bytes.Repeat([]byte("a"), defaultBlockSize*2)
	write(t, a, "file", original)
	pull(t, b, a)
	advertised := copyEntry(a.e.files["file"])
	expected := copyEntry(b.e.files["file"])
	write(t, a, "file", bytes.Repeat([]byte("x"), len(original)))
	err := b.e.pull(context.Background(), client, advertised, expected)
	if status.Code(err) != codes.Aborted {
		t.Fatalf("expected stale-prefix abort, got %v", err)
	}
	equalFile(t, b, "file", original)
	pull(t, b, a)
	equalFile(t, b, "file", bytes.Repeat([]byte("x"), len(original)))
}

type deltaStub struct {
	pb.UnimplementedFileSyncServiceServer
	chunks []*pb.FileChunk
	wait   chan struct{}
}

func (s *deltaStub) ReadDelta(stream pb.FileSyncService_ReadDeltaServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	for _, c := range s.chunks {
		if err := stream.Send(c); err != nil {
			return err
		}
	}
	if s.wait != nil {
		close(s.wait)
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	return nil
}
func deltaClient(t *testing.T, s pb.FileSyncServiceServer) pb.FileSyncServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterFileSyncServiceServer(server, s)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///delta", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewFileSyncServiceClient(conn)
}
func TestDeltaValidationAndCancellation(t *testing.T) {
	for name, chunk := range map[string]*pb.FileChunk{
		"wrong hash": {ChunkData: []byte("evil")},
		"unaligned":  {Offset: 1, ChunkData: []byte("goo")},
		"short":      {ChunkData: []byte("bad")},
		"oversize":   {ChunkData: []byte("too long")},
		"negative":   {Offset: -1, ChunkData: []byte("good")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			write(t, f, "file", []byte("base"))
			expected := copyEntry(f.e.files["file"])
			sum := sha256.Sum256([]byte("good"))
			remote := copyEntry(expected)
			remote.Revision++
			remote.Hash = hex.EncodeToString(sum[:])
			client := deltaClient(t, &deltaStub{chunks: []*pb.FileChunk{chunk}})
			if err := f.e.pull(context.Background(), client, remote, expected); err == nil {
				t.Fatal("accepted invalid delta")
			}
			equalFile(t, f, "file", []byte("base"))
			staged, _ := filepath.Glob(filepath.Join(f.dir, stagePrefix+"*"))
			if len(staged) != 0 {
				t.Fatal("staging file leaked")
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		f := newFixture(t)
		write(t, f, "file", []byte("base"))
		expected := copyEntry(f.e.files["file"])
		remote := copyEntry(expected)
		remote.Revision++
		remote.FileSize = 2 * defaultBlockSize
		content := bytes.Repeat([]byte("x"), int(remote.FileSize))
		sum := sha256.Sum256(content)
		remote.Hash = hex.EncodeToString(sum[:])
		sent := make(chan struct{})
		client := deltaClient(t, &deltaStub{wait: sent, chunks: []*pb.FileChunk{{ChunkData: content[:defaultBlockSize]}}})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- f.e.pull(ctx, client, remote, expected) }()
		select {
		case <-sent:
			cancel()
		case <-ctx.Done():
			t.Fatal("server did not receive request")
		}
		if err := <-done; err == nil {
			t.Fatal("cancelled transfer succeeded")
		}
		equalFile(t, f, "file", []byte("base"))
	})
}

func TestDeltaRejectsMalformedBasis(t *testing.T) {
	a := newFixture(t)
	client := serve(t, a)
	write(t, a, "file", []byte("data"))
	entry := copyEntry(a.e.files["file"])
	for name, req := range map[string]*pb.DeltaRequest{
		"zero block":      {File: entry},
		"large block":     {File: entry, BlockSize: 2 * 1024 * 1024},
		"negative offset": {File: entry, BlockSize: 1024, Basis: []*pb.ChunkHash{{Offset: -1, Hash: make([]byte, 32)}}},
		"outside file":    {File: entry, BlockSize: 1024, Basis: []*pb.ChunkHash{{Offset: 1024, Hash: make([]byte, 32)}}},
		"invalid hash":    {File: entry, BlockSize: 1024, Basis: []*pb.ChunkHash{{Hash: []byte("bad")}}},
		"duplicate":       {File: entry, BlockSize: 1024, Basis: []*pb.ChunkHash{{Hash: make([]byte, 32)}, {Hash: make([]byte, 32)}}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stream, err := client.ReadDelta(ctx)
			if err != nil {
				t.Fatal(err)
			}
			stream.Send(req)
			stream.CloseSend()
			_, err = stream.Recv()
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type legacyService struct {
	pb.UnimplementedFileSyncServiceServer
	service *Service
}

func (s *legacyService) ListFiles(ctx context.Context, req *pb.Empty) (*pb.FileList, error) {
	return s.service.ListFiles(ctx, req)
}
func (s *legacyService) ReadFile(req *pb.FileEntry, stream pb.FileSyncService_ReadFileServer) error {
	return s.service.ReadFile(req, stream)
}
func TestFallbackToPreviousV2Peer(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	a.client = deltaClient(t, &legacyService{service: &Service{Engine: a.e, ChunkSize: 4096}})
	content := bytes.Repeat([]byte("legacy"), 50000)
	write(t, a, "file", content)
	pull(t, b, a)
	equalFile(t, b, "file", content)
	content[17] = 'X'
	write(t, a, "file", content)
	pull(t, b, a)
	equalFile(t, b, "file", content)
}

func TestReceiverChoosesBlockSize(t *testing.T) {
	for _, size := range []int{1024, 256 * 1024, 1024 * 1024} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a, b := newFixture(t), newFixture(t)
			b.e.BlockSize = size
			client := &measuredClient{FileSyncServiceClient: serve(t, a)}
			a.client = client
			content := bytes.Repeat([]byte("a"), 3*size+19)
			write(t, a, "file", content)
			pull(t, b, a)
			client.bytes.Store(0)
			content[size+7] = 'x'
			write(t, a, "file", content)
			pull(t, b, a)
			equalFile(t, b, "file", content)
			if got := client.bytes.Load(); got != int64(size) {
				t.Fatalf("requested %d-byte blocks, received %d bytes", size, got)
			}
		})
	}
}

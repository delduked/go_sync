package servers

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type interruptedSource struct {
	pb.UnimplementedFileSyncServiceServer
	data []byte
}

func (s *interruptedSource) ReadDelta(stream pb.FileSyncService_ReadDeltaServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	for i := 0; i < 2; i++ {
		if err := stream.Send(&pb.FileChunk{Offset: int64(i * defaultBlockSize), ChunkData: s.data[i*defaultBlockSize : (i+1)*defaultBlockSize]}); err != nil {
			return err
		}
	}
	return status.Error(codes.Unavailable, "simulated network interruption")
}
func partialTransfer(t *testing.T, a, b *fixture, data []byte) {
	t.Helper()
	write(t, a, "file", data)
	client := deltaClient(t, &interruptedSource{data: data})
	err := b.e.pull(context.Background(), client, copyEntry(a.e.files["file"]), nil)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected interrupted transfer: %v", err)
	}
	absent(t, b, "file")
	stage, err := os.ReadFile(filepath.Join(b.dir, resumeName("file")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stage[:2*defaultBlockSize], data[:2*defaultBlockSize]) {
		t.Fatal("received chunks were not retained")
	}
}
func TestResumeAcrossRestartRevalidatesPartialData(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		name := "valid"
		if tamper {
			name = "corrupted cache"
		}
		t.Run(name, func(t *testing.T) {
			a, b := newFixture(t), newFixture(t)
			data := bytes.Repeat([]byte("abcd"), defaultBlockSize)
			partialTransfer(t, a, b, data)
			if tamper {
				file, err := os.OpenFile(filepath.Join(b.dir, resumeName("file")), os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				file.WriteAt([]byte("bad"), 0)
				file.Close()
			}
			b.e.Close()
			var err error
			b.e, err = NewEngine(b.dir, b.state)
			if err != nil {
				t.Fatal(err)
			}
			measured := &measuredClient{FileSyncServiceClient: serve(t, a)}
			a.client = measured
			pull(t, b, a)
			equalFile(t, b, "file", data)
			want := int64(2 * defaultBlockSize)
			if tamper {
				want += defaultBlockSize
			}
			if got := measured.bytes.Load(); got != want {
				t.Fatalf("resumed payload=%d want=%d", got, want)
			}
			absent(t, b, resumeName("file"))
			if _, err = os.Stat(b.e.resumeJournal("file")); !os.IsNotExist(err) {
				t.Fatal("completed journal retained")
			}
		})
	}
}
func TestNewRevisionCanReuseInterruptedPrefix(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	data := bytes.Repeat([]byte("abcd"), defaultBlockSize)
	partialTransfer(t, a, b, data)
	data = append(data, bytes.Repeat([]byte("e"), defaultBlockSize)...)
	write(t, a, "file", data)
	measured := &measuredClient{FileSyncServiceClient: serve(t, a)}
	a.client = measured
	pull(t, b, a)
	equalFile(t, b, "file", data)
	if measured.bytes.Load() != 3*defaultBlockSize {
		t.Fatalf("old prefix not reused: %d", measured.bytes.Load())
	}
}
func TestDeletionDiscardsPendingTransfer(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	data := bytes.Repeat([]byte("abcd"), defaultBlockSize)
	partialTransfer(t, a, b, data)
	os.Remove(filepath.Join(a.dir, "file"))
	a.e.Scan()
	serve(t, a)
	pull(t, b, a)
	absent(t, b, "file")
	absent(t, b, resumeName("file"))
	if _, err := os.Stat(b.e.resumeJournal("file")); !os.IsNotExist(err) {
		t.Fatal("deleted transfer journal retained")
	}
}

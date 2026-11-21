package servers

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestFolderIdentityPersistsAndCannotBeChanged(t *testing.T) {
	base := t.TempDir()
	root, state := filepath.Join(base, "files"), filepath.Join(base, "state")
	e, err := NewEngine(root, state, WithFolderID("project-a"))
	if err != nil {
		t.Fatal(err)
	}
	id := e.ID()
	e.Close()
	e, err = NewEngine(root, state)
	if err != nil {
		t.Fatal(err)
	}
	if e.FolderID() != "project-a" || e.ID() != id {
		t.Fatal("identity changed on restart")
	}
	e.Close()
	if e, err = NewEngine(root, state, WithFolderID("project-b")); err == nil {
		e.Close()
		t.Fatal("accepted wrong folder state")
	}
}
func TestFolderNamespaceRejectsUnaryAndStreamingRequests(t *testing.T) {
	f := newFixture(t)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(FolderServerOptions("project-a")...)
	pb.RegisterFileSyncServiceServer(server, &Service{Engine: f.e})
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewFileSyncServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = client.Announce(folderContext(ctx, "project-a"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Announce(folderContext(ctx, "project-b"), &pb.Empty{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unary isolation: %v", err)
	}
	stream, err := client.ExchangeManifest(folderContext(ctx, "project-b"))
	if err != nil {
		t.Fatal(err)
	}
	stream.CloseSend()
	_, err = stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stream isolation: %v", err)
	}
}

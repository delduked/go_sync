package servers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type fixture struct {
	e          *Engine
	dir, state string
	client     pb.FileSyncServiceClient
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	base := t.TempDir()
	dir, state := filepath.Join(base, "files"), filepath.Join(base, "state")
	e, err := NewEngine(dir, state)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{e: e, dir: dir, state: state}
	t.Cleanup(func() { f.e.Close() })
	return f
}
func serve(t testing.TB, f *fixture) pb.FileSyncServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterFileSyncServiceServer(s, &Service{Engine: f.e, ChunkSize: 64 * 1024})
	go s.Serve(listener)
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	f.client = pb.NewFileSyncServiceClient(conn)
	return f.client
}
func write(t testing.TB, f *fixture, name string, data []byte) {
	t.Helper()
	p := filepath.Join(f.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.e.Scan(); err != nil {
		t.Fatal(err)
	}
}
func pull(t testing.TB, to, from *fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := to.e.Reconcile(ctx, from.client); err != nil {
		t.Fatal(err)
	}
}
func equalFile(t testing.TB, f *fixture, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s: got %d bytes, want %d, err=%v", name, len(got), len(want), err)
	}
}
func absent(t testing.TB, f *fixture, name string) {
	t.Helper()
	_, err := os.Lstat(filepath.Join(f.dir, name))
	if !os.IsNotExist(err) {
		t.Fatalf("%s should be absent: %v", name, err)
	}
}

func TestRoundTripFiles(t *testing.T) {
	a, b, c := newFixture(t), newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	serve(t, c)
	cases := map[string][]byte{"text.txt": []byte("abcdefghij"), "binary.bin": append(bytes.Repeat([]byte{0, 255, 128, 1}, 65536), []byte("tail")...), "empty": {}, "src/nested/index.ts": []byte("export {}\n"), ".env": []byte("KEY=value\n"), "other/index.ts": []byte("different\n")}
	for name, data := range cases {
		write(t, a, name, data)
	}
	pull(t, b, a)
	pull(t, c, b)
	for name, data := range cases {
		equalFile(t, b, name, data)
		equalFile(t, c, name, data)
	}
	// Shorten within a chunk, across a chunk boundary, and to zero bytes.
	for _, data := range [][]byte{[]byte("abc"), bytes.Repeat([]byte("x"), 200000), []byte("short"), {}} {
		write(t, b, "text.txt", data)
		pull(t, a, b)
		pull(t, c, b)
		pull(t, b, a)
		for _, f := range []*fixture{a, b, c} {
			equalFile(t, f, "text.txt", data)
		}
	}
	if err := os.Chmod(filepath.Join(c.dir, "binary.bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := c.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, a, c)
	st, err := os.Stat(filepath.Join(a.dir, "binary.bin"))
	if err != nil || st.Mode().Perm() != 0755 {
		t.Fatalf("executable mode: %v %v", st, err)
	}
	// Reconciliation must be idempotent; it must not synthesize new edits.
	before, _ := a.e.Manifest()
	pull(t, a, c)
	after, _ := a.e.Manifest()
	if len(before.Files) != len(after.Files) {
		t.Fatal("manifest grew")
	}
	for _, x := range before.Files {
		if !sameVersion(x, a.e.files[x.FileName]) {
			t.Fatalf("echo revision for %s", x.FileName)
		}
	}
}
func TestRenameDeletionAndOfflineRestart(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "folder/before", []byte("original"))
	pull(t, b, a)
	if err := os.Rename(filepath.Join(a.dir, "folder"), filepath.Join(a.dir, "renamed")); err != nil {
		t.Fatal(err)
	}
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	absent(t, b, "folder")
	equalFile(t, b, "renamed/before", []byte("original"))
	// B is offline while A deletes; restart A before B reconnects.
	if err := os.RemoveAll(filepath.Join(a.dir, "renamed")); err != nil {
		t.Fatal(err)
	}
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	id := a.e.ID()
	a.e.Close()
	var err error
	a.e, err = NewEngine(a.dir, a.state)
	if err != nil {
		t.Fatal(err)
	}
	serve(t, a)
	if a.e.ID() != id {
		t.Fatal("identity changed on restart")
	}
	pull(t, a, b)
	pull(t, b, a)
	pull(t, a, b)
	absent(t, a, "renamed")
	absent(t, b, "renamed")
}
func TestDifferentLocalRootsAndEmptyDirectories(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	os.MkdirAll(filepath.Join(a.dir, "empty/subdir"), 0700)
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	info, err := os.Stat(filepath.Join(b.dir, "empty/subdir"))
	if err != nil || !info.IsDir() {
		t.Fatalf("empty directory missing: %v", err)
	}
}
func TestPathConfinement(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"../escape", "/tmp/escape", "a/../../escape", "a\\..\\escape", "C:/escape", ".", "", "a//b", stagePrefix + "x"} {
		if validName(name) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "victim"), []byte("safe"), 0644)
	if err := os.Symlink(outside, filepath.Join(f.dir, "link")); err != nil {
		t.Fatal(err)
	}
	if file, _, err := f.e.stage("link/victim"); err == nil {
		file.Close()
		t.Fatal("escaped through symlink")
	}
	data, _ := os.ReadFile(filepath.Join(outside, "victim"))
	if string(data) != "safe" {
		t.Fatal("modified outside root")
	}

	if e, err := NewEngine(f.dir, filepath.Join(f.dir, "state")); err == nil {
		e.Close()
		t.Fatal("accepted state inside root")
	}
}
func TestHealthCheckCancellation(t *testing.T) {
	a := newFixture(t)
	client := serve(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = stream.Send(&pb.Ping{Message: "alive"}); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatal(err)
	}
	cancel()
	_, _ = stream.Recv()
	// The server must still serve RPCs after an abruptly canceled stream.
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if _, err = client.ListFiles(ctx, &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
}

type corruptServer struct {
	pb.UnimplementedFileSyncServiceServer
	chunk *pb.FileChunk
}

func (s *corruptServer) ReadFile(_ *pb.FileEntry, stream pb.FileSyncService_ReadFileServer) error {
	return stream.Send(s.chunk)
}
func TestCorruptTransferLeavesDestinationIntact(t *testing.T) {
	for name, chunk := range map[string]*pb.FileChunk{
		"partial":      {ChunkData: []byte("bad")},
		"wrong hash":   {ChunkData: []byte("evil")},
		"wrong offset": {ChunkData: []byte("good"), Offset: 1},
		"oversized":    {ChunkData: []byte("too long")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			write(t, f, "file", []byte("original"))
			expected := copyEntry(f.e.files["file"])
			sum := sha256.Sum256([]byte("good"))
			remote := &pb.FileEntry{FileName: "file", FileSize: 4, Hash: hex.EncodeToString(sum[:]), Revision: expected.Revision + 1, Origin: expected.Origin, Mode: 0644}
			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer()
			pb.RegisterFileSyncServiceServer(server, &corruptServer{chunk: chunk})
			go server.Serve(listener)
			defer server.Stop()
			conn, err := grpc.NewClient("passthrough:///bad", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err = f.e.pull(context.Background(), pb.NewFileSyncServiceClient(conn), remote, expected); err == nil {
				t.Fatal("accepted corrupt transfer")
			}
			equalFile(t, f, "file", []byte("original"))
			if !sameVersion(expected, f.e.files["file"]) {
				t.Fatal("advanced metadata despite failed transfer")
			}
			names, _ := os.ReadDir(f.dir)
			if len(names) != 1 {
				t.Fatal("left staged files behind")
			}
		})
	}
}
func TestEditDuringTransferAndVersionBackups(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "file", []byte("base"))
	pull(t, b, a)
	expected := copyEntry(b.e.files["file"])
	write(t, a, "file", []byte("remote"))
	remote := copyEntry(a.e.files["file"])
	tmp, name, err := b.e.stage("file")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(tmp, "remote")
	tmp.Close()
	defer b.e.root.Remove(name)
	write(t, b, "file", []byte("local"))
	if err = b.e.apply(remote, expected, name); err == nil {
		t.Fatal("overwrote in-flight local edit")
	}
	equalFile(t, b, "file", []byte("local"))
	// Concurrent same-path edits converge deterministically; the losing bytes
	// remain in the overwritten peer's local version archive.
	pull(t, a, b)
	pull(t, b, a)
	left, _ := os.ReadFile(filepath.Join(a.dir, "file"))
	right, _ := os.ReadFile(filepath.Join(b.dir, "file"))
	if !bytes.Equal(left, right) {
		t.Fatal("did not converge")
	}
	found := false
	for _, f := range []*fixture{a, b} {
		paths, _ := filepath.Glob(filepath.Join(f.state, "versions", "*"))
		for _, p := range paths {
			data, _ := os.ReadFile(p)
			if (string(left) == "local" && string(data) == "remote") || (string(left) == "remote" && string(data) == "local") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("losing edit not archived")
	}
}

func TestConcurrentWatchReconcileAndShutdown(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterFileSyncServiceServer(server, &Service{Engine: a.e})
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.e.Watch(ctx, 20*time.Millisecond); close(done) }()
	conns := NewConnections(ctx, b.e, 20*time.Millisecond, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer func() { cancel(); conns.Close(); <-done }()
	conns.AddPeer(listener.Addr().String())
	conns.AddPeer(listener.Addr().String())
	for i := 0; i < 5; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 70000+i)
		if err = os.WriteFile(filepath.Join(a.dir, "hot"), data, 0644); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		matched := false
		for time.Now().Before(deadline) {
			got, _ := os.ReadFile(filepath.Join(b.dir, "hot"))
			if bytes.Equal(got, data) {
				matched = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !matched {
			t.Fatal("watch/reconcile failed to converge")
		}
	}
	// An unreachable peer must not hold up healthy peers or cancellation.
	conns.AddPeer("127.0.0.1:1")
}

func TestFileDirectoryReplacement(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	write(t, a, "item", []byte("old"))
	pull(t, b, a)
	os.Remove(filepath.Join(a.dir, "item"))
	os.Mkdir(filepath.Join(a.dir, "item"), 0755)
	write(t, a, "item/child", []byte("nested"))
	pull(t, b, a)
	equalFile(t, b, "item/child", []byte("nested"))
	os.RemoveAll(filepath.Join(a.dir, "item"))
	write(t, a, "item", []byte("regular again"))
	pull(t, b, a)
	equalFile(t, b, "item", []byte("regular again"))
}

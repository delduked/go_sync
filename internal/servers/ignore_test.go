package servers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProjectExclusions(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, ".gosyncignore", []byte("# shared project policy\nnode_modules/\n.git/\n/dist/\n**/*.tmp\n"))
	for _, name := range []string{"node_modules/pkg/index.js", "src/node_modules/pkg/data", ".git/config", "dist/app.js", "src/editor.tmp", "src/main.ts", "src/dist/kept.ts"} {
		write(t, a, name, []byte(name))
	}
	pull(t, b, a)
	for _, name := range []string{"node_modules/pkg/index.js", "src/node_modules/pkg/data", ".git/config", "dist/app.js", "src/editor.tmp"} {
		absent(t, b, name)
	}
	for _, name := range []string{"src/main.ts", "src/dist/kept.ts"} {
		equalFile(t, b, name, []byte(name))
	}
	write(t, b, "node_modules/local", []byte("local install"))
	pull(t, a, b)
	absent(t, a, "node_modules/local")
	// Reloading a shared policy must not delete pre-existing files on either peer.
	write(t, a, "old.txt", []byte("keep local"))
	pull(t, b, a)
	before, _ := a.e.Manifest()
	var old *pb.FileEntry
	for _, f := range before.Files {
		if f.FileName == "old.txt" {
			old = f
		}
	}
	write(t, a, ".gosyncignore", []byte("old.txt\nnode_modules/\n.git/\n/dist/\n**/*.tmp\n"))
	pull(t, b, a)
	if err := os.Remove(filepath.Join(a.dir, "old.txt")); err != nil {
		t.Fatal(err)
	}
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	equalFile(t, b, "old.txt", []byte("keep local"))
	// A known revision cannot bypass the serving policy.
	stream, err := a.client.ReadFile(context.Background(), old)
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("read excluded: %v", err)
	}
	delta, err := a.client.ReadDelta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = delta.Send(&pb.DeltaRequest{File: old, BlockSize: defaultBlockSize}); err != nil {
		t.Fatal(err)
	}
	delta.CloseSend()
	_, err = delta.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("delta excluded: %v", err)
	}
	// Removing the policy makes the local excluded data eligible again.
	write(t, a, ".gosyncignore", nil)
	converge(t, a, b)
	equalFile(t, b, "dist/app.js", []byte("dist/app.js"))
}
func TestIgnoreRulesAndInvalidPolicy(t *testing.T) {
	f := newFixture(t)
	write(t, f, ".gosyncignore", []byte("*.log\n/cache/\na/**/b?.txt\n.gosync*\n"))
	for _, tc := range []struct {
		name      string
		dir, want bool
	}{{"a/log.log", false, true}, {"cache/a", false, true}, {"x/cache/a", false, false}, {"a/b1.txt", false, true}, {"a/x/y/b2.txt", false, true}, {"a/b23.txt", false, false}, {".gosyncignore", false, false}, {".gosync-conflicts/id/data", false, false}} {
		if got := f.e.ignoredLocked(tc.name, tc.dir); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
	if err := os.WriteFile(filepath.Join(f.dir, ".gosyncignore"), []byte("!unsupported"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Manifest(); err == nil {
		t.Fatal("invalid policy accepted")
	}
}

func TestIgnorePolicyCannotBeASymlink(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.dir, "policy"), []byte("private/"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("policy", filepath.Join(f.dir, ignoreFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Manifest(); err == nil {
		t.Fatal("symlink policy accepted but could not be shared")
	}
}

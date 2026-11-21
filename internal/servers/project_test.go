package servers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestLargeManifestStreamsBeyondUnaryLimit(t *testing.T) {
	f := newFixture(t)
	serve(t, f)
	for i := 0; i < 10000; i++ {
		name := fmt.Sprintf("%s/%s/%05d", strings.Repeat("a", 200), strings.Repeat("b", 200), i)
		f.e.files[name] = &pb.FileEntry{FileName: name, IsDeleted: true, Revision: 1, Origin: f.e.id, Version: map[string]uint64{f.e.id: 1}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := f.e.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if proto.Size(list) <= 4<<20 {
		t.Fatal("fixture does not exceed unary message limit")
	}
	if _, err = f.client.ListFiles(ctx, &pb.Empty{}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("unary size limit: %v", err)
	}
	known := make([]string, 0, len(list.Files))
	for _, entry := range list.Files {
		known = append(known, entry.FileName)
	}
	received, err := fetchManifest(ctx, f.client, known)
	if err != nil || len(received.Files) != len(list.Files) {
		t.Fatalf("streamed manifest: %d entries, %v", len(received.GetFiles()), err)
	}
	for _, entry := range received.Files {
		if !sameVersion(entry, f.e.files[entry.FileName]) {
			t.Fatal("manifest metadata changed")
		}
	}
}
func TestProjectEditorAndBuildCycles(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, ".gosyncignore", []byte("node_modules/\ndist/\n.git/\n*.tmp\n"))
	for i := 0; i < 600; i++ {
		name := fmt.Sprintf("src/package%d/module%d.ts", i/20, i)
		p := filepath.Join(a.dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(fmt.Sprintf("export const value = %d;\n", i)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(t, a, "go.mod", []byte("module example.com/project\n\ngo 1.25\n"))
	write(t, a, "cmd/main.go", []byte("package main\nfunc main(){}\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := b.e.Reconcile(ctx, a.client); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 3; cycle++ {
		for i := 0; i < 30; i++ {
			name := fmt.Sprintf("src/package%d/module%d.ts", i/20, i)
			tmp := filepath.Join(b.dir, name+".tmp")
			if err := os.WriteFile(tmp, []byte(fmt.Sprintf("// cycle %d, module %d\n", cycle, i)), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(tmp, filepath.Join(b.dir, name)); err != nil {
				t.Fatal(err)
			}
		}
		write(t, b, "dist/bundle.js", []byte("generated output"))
		write(t, b, "node_modules/dependency/index.js", []byte("local dependency"))
		if err := a.e.Reconcile(ctx, b.client); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 600; i++ {
		name := fmt.Sprintf("src/package%d/module%d.ts", i/20, i)
		data, err := os.ReadFile(filepath.Join(b.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		equalFile(t, a, name, data)
	}
	absent(t, a, "dist")
	absent(t, a, "node_modules")
	before := a.e.Metrics().PayloadReceived
	if err := a.e.Reconcile(ctx, b.client); err != nil {
		t.Fatal(err)
	}
	if a.e.Metrics().PayloadReceived != before {
		t.Fatal("idle project transferred data")
	}
}

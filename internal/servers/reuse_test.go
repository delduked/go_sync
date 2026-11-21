package servers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameAndCopyReuseContent(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	data := bytes.Repeat([]byte("large binary\x00\xff"), 200000)
	write(t, a, "old/sub/file.bin", data)
	pull(t, b, a)
	before := a.e.Metrics().PayloadSent
	if err := os.Rename(filepath.Join(a.dir, "old"), filepath.Join(a.dir, "new")); err != nil {
		t.Fatal(err)
	}
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	equalFile(t, b, "new/sub/file.bin", data)
	absent(t, b, "old")
	if sent := a.e.Metrics().PayloadSent - before; sent != 0 {
		t.Fatalf("unchanged move sent %d bytes", sent)
	}
	write(t, a, "copy.bin", data)
	pull(t, b, a)
	equalFile(t, b, "copy.bin", data)
	if sent := a.e.Metrics().PayloadSent - before; sent != 0 {
		t.Fatalf("copy sent %d bytes", sent)
	}
	// Stale content-index hints cannot corrupt a result.
	if err := os.WriteFile(filepath.Join(b.dir, "copy.bin"), bytes.Repeat([]byte("X"), len(data)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.dir, "new/sub/file.bin"), bytes.Repeat([]byte("Y"), len(data)), 0644); err != nil {
		t.Fatal(err)
	}
	write(t, a, "third.bin", data)
	pull(t, b, a)
	equalFile(t, b, "third.bin", data)
}

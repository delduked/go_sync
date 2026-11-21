package servers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTimestampAndTouch(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "tool", []byte("#!/bin/sh\necho ok\n"))
	stamp := time.UnixMilli(1700000000123)
	if err := os.Chtimes(filepath.Join(a.dir, "tool"), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(a.dir, "tool"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	info, err := os.Stat(filepath.Join(b.dir, "tool"))
	if err != nil || !info.ModTime().Equal(stamp) || info.Mode().Perm() != 0755 {
		t.Fatalf("metadata: %v %v", info, err)
	}
	before := a.e.Metrics().PayloadSent
	stamp = stamp.Add(time.Hour)
	if err = os.Chtimes(filepath.Join(a.dir, "tool"), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err = a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	pull(t, a, b)
	info, err = os.Stat(filepath.Join(b.dir, "tool"))
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("touch: %v %v", info, err)
	}
	if a.e.Metrics().PayloadSent != before {
		t.Fatal("touch transferred file contents")
	}
}
func TestSymlinkStaysLocalAndDoesNotBlockOtherFiles(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "alias", []byte("original"))
	pull(t, b, a)
	write(t, a, "target", []byte("private local target"))
	if err := os.Remove(filepath.Join(a.dir, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(a.dir, "alias")); err != nil {
		t.Fatal(err)
	}
	write(t, a, "ordinary", []byte("still synchronized"))
	pull(t, b, a)
	equalFile(t, b, "ordinary", []byte("still synchronized"))
	manifest, err := a.e.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range manifest.Files {
		if f.FileName == "alias" {
			t.Fatal("advertised stale symlink metadata")
		}
	}
	write(t, b, "alias", []byte("remote edit"))
	if err = a.e.Reconcile(context.Background(), b.client); err == nil {
		t.Fatal("overwrote local symlink")
	}
	info, err := os.Lstat(filepath.Join(a.dir, "alias"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink removed")
	}
	equalFile(t, a, "target", []byte("private local target"))
}

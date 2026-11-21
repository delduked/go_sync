package servers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerifyDoesNotAdoptUnverifiedDiskChanges(t *testing.T) {
	f := newFixture(t)
	write(t, f, "file", []byte("saved"))
	before := copyEntry(f.e.files["file"])
	f.e.Close()
	if err := os.WriteFile(filepath.Join(f.dir, "file"), []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	var err error
	f.e, err = NewEngine(f.dir, f.state, WithStoredState())
	if err != nil {
		t.Fatal(err)
	}
	issues, err := f.e.Verify()
	if err != nil || len(issues) != 1 {
		t.Fatalf("verify: %v %v", issues, err)
	}
	if !sameVersion(f.e.files["file"], before) {
		t.Fatal("verification rewrote history")
	}
	if err = f.e.Rescan(); err != nil {
		t.Fatal(err)
	}
	issues, err = f.e.Verify()
	if err != nil || len(issues) != 0 {
		t.Fatalf("rescan verify: %v %v", issues, err)
	}
}
func TestRestoreRetainedBytesAsNewSharedRevision(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "file", []byte("original"))
	pull(t, b, a)
	write(t, a, "file", []byte("new contents"))
	pull(t, b, a)
	versions, err := b.e.History("file")
	if err != nil || len(versions) != 1 {
		t.Fatalf("history: %v %v", versions, err)
	}
	if err = b.e.RestoreVersion("file", versions[0].ID); err != nil {
		t.Fatal(err)
	}
	pull(t, a, b)
	equalFile(t, a, "file", []byte("original"))
	equalFile(t, b, "file", []byte("original"))
	versions, err = b.e.History("file")
	if err != nil || len(versions) != 2 {
		t.Fatal("restore failed to retain replaced bytes")
	}
	for _, version := range versions {
		if err = os.WriteFile(filepath.Join(b.state, "versions", version.ID), []byte("corrupt"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = b.e.RestoreVersion("file", versions[0].ID); err == nil {
		t.Fatal("restored corrupt backup")
	}
	equalFile(t, b, "file", []byte("original"))
}
func TestRestoreVisibleConflict(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "file", []byte("one"))
	write(t, b, "file", []byte("two"))
	converge(t, a, b)
	directories, err := os.ReadDir(filepath.Join(a.dir, conflictDirectory))
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, dir := range directories {
		if _, err = os.Stat(filepath.Join(a.dir, conflictDirectory, dir.Name(), "data")); err == nil {
			id = dir.Name()
			break
		}
	}
	if id == "" {
		t.Fatal("no preserved data")
	}
	expected, err := os.ReadFile(filepath.Join(a.dir, conflictDirectory, id, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err = a.e.RestoreConflict(id); err != nil {
		t.Fatal(err)
	}
	pull(t, b, a)
	equalFile(t, b, "file", expected)
}
func TestWatchAcknowledgesFullRescanRequests(t *testing.T) {
	f := newFixture(t)
	write(t, f, "file", []byte("before"))
	dir := filepath.Join(f.state, "commands")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	request := filepath.Join(dir, "rescan-test.request")
	if err := os.WriteFile(request, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.e.Watch(ctx, 50*time.Millisecond) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(request); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("rescan request was not acknowledged")
}

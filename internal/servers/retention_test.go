package servers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompactedDeletionProtectsOfflinePeerAfterRestart(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	write(t, a, "file", []byte("old"))
	pull(t, b, a)
	os.Remove(filepath.Join(a.dir, "file"))
	a.e.Scan()
	revision := a.e.files["file"].Revision
	result, err := a.e.Prune(time.Now().Add(time.Second))
	if err != nil || result.TombstonesCompacted != 1 {
		t.Fatalf("prune: %+v %v", result, err)
	}
	list, err := a.e.Manifest()
	if err != nil || len(list.Files) != 0 {
		t.Fatal("compacted deletion still in ordinary manifest", err)
	}
	a.e.Close()
	a.e, err = NewEngine(a.dir, a.state)
	if err != nil {
		t.Fatal(err)
	}
	serve(t, a)
	if a.e.clock < revision {
		t.Fatal("logical clock went backwards")
	}
	// Only the stale peer pulls: known_paths must reactivate the deletion floor.
	pull(t, b, a)
	absent(t, b, "file")
	// A deliberate new edit inherits the deletion history and can be recreated.
	write(t, b, "file", []byte("intentional new version"))
	serve(t, b)
	pull(t, a, b)
	equalFile(t, a, "file", []byte("intentional new version"))
}
func TestPruneKeepsActivePartialAndRemovesExpiredHistory(t *testing.T) {
	e := newFixture(t).e
	temp, name, _, err := e.resumeStage("pending")
	if err != nil {
		t.Fatal(err)
	}
	temp.WriteString("partial")
	temp.Close()
	record := resumeRecord{Name: "pending", Updated: time.Now().Add(-48 * time.Hour)}
	raw, _ := json.Marshal(record)
	os.WriteFile(e.resumeJournal("pending"), raw, 0600)
	backupDir := filepath.Join(e.stateDir, "versions")
	os.MkdirAll(backupDir, 0700)
	backup := filepath.Join(backupDir, "old-version")
	os.WriteFile(backup, []byte("old"), 0600)
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(backup, past, past)
	unlock := e.lockTransfer("pending")
	result, err := e.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.PartialsRemoved != 0 || result.BackupsRemoved != 1 {
		t.Fatalf("unsafe retention: %+v", result)
	}
	if _, err = e.root.Stat(name); err != nil {
		t.Fatal("active staging removed")
	}
	unlock()
	result, err = e.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil || result.PartialsRemoved != 1 {
		t.Fatalf("expired partial not removed: %+v %v", result, err)
	}
	if _, err = e.root.Stat(name); !os.IsNotExist(err) {
		t.Fatal("expired staging retained")
	}
}
func TestNewLocalWriteInheritsCompactedHistory(t *testing.T) {
	f := newFixture(t)
	write(t, f, "file", []byte("old"))
	os.Remove(filepath.Join(f.dir, "file"))
	f.e.Scan()
	deleted := copyEntry(f.e.files["file"])
	f.e.Prune(time.Now().Add(time.Second))
	write(t, f, "file", []byte("new"))
	if compare(f.e.files["file"], deleted) != after {
		t.Fatal("recreated file lost causal history")
	}
}

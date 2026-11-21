package servers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingFilesRequireExplicitBootstrap(t *testing.T) {
	base := t.TempDir()
	folder, state := filepath.Join(base, "files"), filepath.Join(base, "state")
	os.Mkdir(folder, 0755)
	os.WriteFile(filepath.Join(folder, "old"), []byte("valuable"), 0644)
	if e, err := NewEngine(folder, state); err == nil {
		e.Close()
		t.Fatal("implicitly published existing files")
	}
	data, _ := os.ReadFile(filepath.Join(folder, "old"))
	if string(data) != "valuable" {
		t.Fatal("failed bootstrap changed files")
	}
	e, err := NewEngine(folder, state, WithBootstrap("seed"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.files["old"] == nil {
		t.Fatal("explicit seed did not publish files")
	}
}
func TestJoinPreservesOldTreeWithoutResurrectingIt(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	write(t, a, "old", []byte("old copy"))
	pull(t, b, a)
	os.Remove(filepath.Join(a.dir, "old"))
	a.e.Scan()
	// Simulate lost state by choosing a fresh state directory for B.
	b.e.Close()
	b.state = filepath.Join(t.TempDir(), "new-state")
	var err error
	b.e, err = NewEngine(b.dir, b.state, WithBootstrap("join"))
	if err != nil {
		t.Fatal(err)
	}
	archives, _ := filepath.Glob(filepath.Join(b.dir, localPrefix+"prejoin-*", "old"))
	if len(archives) != 1 {
		t.Fatal("old data was not archived")
	}
	data, _ := os.ReadFile(archives[0])
	if string(data) != "old copy" {
		t.Fatal("archive corrupted")
	}
	list, err := b.e.Manifest()
	if err != nil || len(list.Files) != 0 {
		t.Fatalf("private archive advertised: %v %v", list, err)
	}
	serve(t, b)
	converge(t, a, b)
	absent(t, a, "old")
	absent(t, b, "old")
	if validName(filepath.Base(filepath.Dir(archives[0]))+"/old") == nil {
		t.Fatal("remote access to private archive allowed")
	}
}
func TestBootstrapDoesNotRepeatOnNormalRestart(t *testing.T) {
	base := t.TempDir()
	folder, state := filepath.Join(base, "files"), filepath.Join(base, "state")
	os.MkdirAll(folder, 0755)
	os.WriteFile(filepath.Join(folder, "one"), []byte("one"), 0644)
	e, err := NewEngine(folder, state, WithBootstrap("join"))
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	// A normal restart must not quarantine newly synchronized files.
	os.WriteFile(filepath.Join(folder, "new"), []byte("new"), 0644)
	e, err = NewEngine(folder, state, WithBootstrap("join"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.files["new"] == nil {
		t.Fatal("bootstrap repeated despite existing identity")
	}
}

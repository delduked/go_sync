package servers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheSkipsUnchangedScansAndRepairsMissedEvents(t *testing.T) {
	a := newFixture(t)
	data := bytes.Repeat([]byte("a"), defaultBlockSize+17)
	write(t, a, "file", data)
	before := a.e.Metrics().ScanRead
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	if a.e.Metrics().ScanRead != before {
		t.Fatal("unchanged file rehashed")
	}
	path := filepath.Join(a.dir, "file")
	st, _ := os.Stat(path)
	data[1] = 'b'
	if err := os.WriteFile(path, data, st.Mode()); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, st.ModTime(), st.ModTime())
	// Simulate a missed watcher event and an edit preserving size and timestamp.
	cached := a.e.cache["file"]
	cached.checked = time.Now().Add(-2 * time.Minute)
	a.e.cache["file"] = cached
	old := a.e.files["file"].Hash
	if err := a.e.Scan(); err != nil {
		t.Fatal(err)
	}
	if a.e.files["file"].Hash == old || a.e.Metrics().ScanRead == before {
		t.Fatal("periodic verification did not repair changed contents")
	}
	before = a.e.Metrics().ScanRead
	a.e.Invalidate("file")
	a.e.Scan()
	if a.e.Metrics().ScanRead <= before {
		t.Fatal("event invalidation ignored")
	}
	before = a.e.Metrics().ScanRead
	a.e.Rescan()
	if a.e.Metrics().ScanRead <= before {
		t.Fatal("explicit rescan used cache")
	}
}
func TestCacheNeverBypassesDestinationCheck(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	write(t, a, "file", []byte("base"))
	pull(t, b, a)
	b.e.Scan()
	expected := copyEntry(b.e.files["file"])
	path := filepath.Join(b.dir, "file")
	st, _ := os.Stat(path)
	os.WriteFile(path, []byte("edit"), 0644)
	os.Chtimes(path, st.ModTime(), st.ModTime())
	incoming := copyEntry(expected)
	incoming.Revision++
	temp, name, err := b.e.stage("file")
	if err != nil {
		t.Fatal(err)
	}
	temp.WriteString("base")
	temp.Close()
	defer b.e.root.Remove(name)
	if err = b.e.apply(incoming, expected, name); err == nil {
		t.Fatal("cached destination check hid local edit")
	}
	equalFile(t, b, "file", []byte("edit"))
}

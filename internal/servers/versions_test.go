package servers

import (
	pb "github.com/TypeTerrors/go_sync/proto"
	"testing"
)

func TestCausalHistoryAcrossPeersAndRestart(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "file", []byte("base"))
	base := copyEntry(a.e.files["file"])
	pull(t, b, a)
	write(t, b, "file", []byte("descendant"))
	descendant := copyEntry(b.e.files["file"])
	if compare(descendant, base) != after {
		t.Fatal("sequential edit classified as concurrent")
	}
	write(t, a, "file", []byte("independent"))
	independent := copyEntry(a.e.files["file"])
	if compare(independent, descendant) != concurrent {
		t.Fatal("independent edits not detected")
	}
	b.e.Close()
	var err error
	b.e, err = NewEngine(b.dir, b.state)
	if err != nil {
		t.Fatal(err)
	}
	if compare(independent, b.e.files["file"]) != concurrent {
		t.Fatal("history lost at restart")
	}
}
func TestVectorOrdering(t *testing.T) {
	a := &pb.FileEntry{Origin: "a", Revision: 1}
	b := &pb.FileEntry{Origin: "b", Revision: 100, Version: map[string]uint64{"a": 1}}
	if compare(a, b) != before || compare(b, a) != after || compare(a, a) != equal {
		t.Fatal("invalid partial order")
	}
	c := &pb.FileEntry{Origin: "a", Revision: 2}
	if compare(c, b) != concurrent {
		t.Fatal("global counter mistaken for causality")
	}
	merged := copyEntry(b)
	merged.Version = mergedVersion(b, c)
	if compare(merged, b) != after || compare(merged, c) != after {
		t.Fatal("merged history does not dominate both parents")
	}
}

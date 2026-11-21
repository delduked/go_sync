package servers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func converge(t *testing.T, peers ...*fixture) {
	t.Helper()
	for round := 0; round < 5; round++ {
		for _, to := range peers {
			for _, from := range peers {
				if to != from {
					pull(t, to, from)
				}
			}
		}
	}
}
func conflictData(t *testing.T, f *fixture) [][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(f.dir, conflictDirectory, "*", "data"))
	if err != nil {
		t.Fatal(err)
	}
	var data [][]byte
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, b)
	}
	return data
}
func TestConcurrentFilesPreservedOnAllPeers(t *testing.T) {
	a, b, c := newFixture(t), newFixture(t), newFixture(t)
	for _, f := range []*fixture{a, b, c} {
		serve(t, f)
	}
	write(t, a, "file", []byte("base"))
	pull(t, b, a)
	pull(t, c, a)
	values := [][]byte{[]byte("A edit"), []byte("B edit"), []byte("C edit")}
	for i, f := range []*fixture{a, b, c} {
		write(t, f, "file", values[i])
	}
	converge(t, a, b, c)
	main, _ := os.ReadFile(filepath.Join(a.dir, "file"))
	for _, f := range []*fixture{a, b, c} {
		equalFile(t, f, "file", main)
		copies := append(conflictData(t, f), main)
		for _, want := range values {
			found := false
			for _, got := range copies {
				if bytes.Equal(got, want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("lost concurrent value %s", want)
			}
		}
		if compare(f.e.files["file"], a.e.files["file"]) != equal {
			t.Fatalf("histories did not converge: a=%v current=%v", a.e.files["file"], f.e.files["file"])
		}
	}
	before := len(conflictData(t, a))
	converge(t, a, b, c)
	if len(conflictData(t, a)) != before {
		t.Fatal("duplicate conflicts")
	}
	// An edit after convergence descends from all three histories.
	write(t, b, "file", []byte("resolved"))
	converge(t, a, b, c)
	for _, f := range []*fixture{a, b, c} {
		equalFile(t, f, "file", []byte("resolved"))
		if len(conflictData(t, f)) != before {
			t.Fatal("sequential resolution created another conflict")
		}
	}
}
func TestIdenticalConcurrentEditsDoNotCreateConflictCopies(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	serve(t, b)
	write(t, a, "file", []byte("same"))
	write(t, b, "file", []byte("same"))
	converge(t, a, b)
	if len(conflictData(t, a)) != 0 || len(conflictData(t, b)) != 0 {
		t.Fatal("identical contents copied as conflict")
	}
	if compare(a.e.files["file"], b.e.files["file"]) != equal {
		t.Fatal("identical contents did not merge causal histories")
	}
}

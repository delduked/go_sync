package servers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func converge(t *testing.T, peers ...*fixture) {
	t.Helper()
	// Type/parent conflicts can temporarily prevent a child transfer. Exercise
	// the same retry behavior as the connection worker, and require a clean end.
	for round := 0; round < 8; round++ {
		for _, to := range peers {
			for _, from := range peers {
				if to != from {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					err := to.e.Reconcile(ctx, from.client)
					cancel()
					if err != nil && round == 7 {
						t.Fatalf("did not converge after retries: %v", err)
					}
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

func TestConcurrentOperationsPreserveContents(t *testing.T) {
	for _, scenario := range []string{"delete-edit", "rename-edit", "same-path-create", "directory-new-child", "directory-file"} {
		t.Run(scenario, func(t *testing.T) {
			a, b := newFixture(t), newFixture(t)
			serve(t, a)
			serve(t, b)
			if scenario == "same-path-create" {
				write(t, a, "item", []byte("created A"))
				write(t, b, "item", []byte("created B"))
			} else if scenario == "directory-new-child" || scenario == "directory-file" {
				write(t, a, "folder/base", []byte("base"))
				pull(t, b, a)
				os.RemoveAll(filepath.Join(a.dir, "folder"))
				if scenario == "directory-file" {
					write(t, a, "folder", []byte("replacement file"))
				} else {
					a.e.Scan()
				}
				write(t, b, "folder/new", []byte("independent child"))
			} else {
				write(t, a, "item", []byte("base"))
				pull(t, b, a)
				if scenario == "delete-edit" {
					os.Remove(filepath.Join(a.dir, "item"))
				} else {
					os.Rename(filepath.Join(a.dir, "item"), filepath.Join(a.dir, "renamed"))
				}
				a.e.Scan()
				write(t, b, "item", []byte("independent edit"))
			}
			converge(t, a, b)
			for _, f := range []*fixture{a, b} {
				var want [][]byte
				switch scenario {
				case "same-path-create":
					want = [][]byte{[]byte("created A"), []byte("created B")}
				case "delete-edit", "rename-edit":
					want = [][]byte{[]byte("independent edit")}
				case "directory-new-child":
					want = [][]byte{[]byte("independent child")}
				case "directory-file":
					want = [][]byte{[]byte("replacement file"), []byte("independent child")}
				}
				all := conflictData(t, f)
				for _, name := range []string{"item", "renamed", "folder", "folder/new"} {
					if data, err := os.ReadFile(filepath.Join(f.dir, name)); err == nil {
						all = append(all, data)
					}
				}
				for _, value := range want {
					found := false
					for _, data := range all {
						if bytes.Equal(data, value) {
							found = true
						}
					}
					if !found {
						t.Fatalf("lost %s", value)
					}
				}
				if scenario == "rename-edit" {
					equalFile(t, f, "renamed", []byte("base"))
				}
			}
		})
	}
}

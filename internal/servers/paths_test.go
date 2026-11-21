package servers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableNames(t *testing.T) {
	for _, name := range []string{"CON", "CONIN$", "CONOUT$.txt", "COM¹.log", "LPT²", "CON .txt", "aux.txt", "nested/COM1.exe", "NUL", "name.", "name ", "a?b", "a*b", "a<b", "a|b", "a\tb", "e\u0301.txt", strings.Repeat("x", 256)} {
		if validName(name) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"src/main.go", "café.txt", "COM10.txt", "CONSOLE", ".env", "folder with spaces/a.ts"} {
		if err := validName(name); err != nil {
			t.Errorf("rejected %q: %v", name, err)
		}
	}
}
func TestCaseCollisionsCannotOverwriteLocalData(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	write(t, a, "Readme.md", []byte("remote"))
	write(t, b, "README.md", []byte("local"))
	if err := b.e.Reconcile(context.Background(), a.client); err == nil {
		t.Fatal("case collision accepted")
	}
	equalFile(t, b, "README.md", []byte("local"))
	absent(t, b, "Readme.md")
	if err := os.WriteFile(filepath.Join(a.dir, "README.md"), []byte("second local name"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.e.Manifest(); err == nil {
		t.Fatal("ambiguous tree advertised")
	}
}

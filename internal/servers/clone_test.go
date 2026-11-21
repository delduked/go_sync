package servers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneIsolationWhenSupported(t *testing.T) {
	dir := t.TempDir()
	src, err := os.Create(filepath.Join(dir, "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	src.WriteString("original")
	dst, err := os.Create(filepath.Join(dir, "staged"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if !cloneFile(dst, src) {
		t.Skip("filesystem does not support reflinks; portable fallback covered by transfer tests")
	}
	dst.WriteAt([]byte("changed!"), 0)
	data, _ := os.ReadFile(src.Name())
	if string(data) != "original" {
		t.Fatal("staging modified source")
	}
}

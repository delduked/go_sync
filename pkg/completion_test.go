package pkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompletionRequiresExactContents(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "download")
	dest := filepath.Join(dir, "dest", "download")
	os.MkdirAll(filepath.Dir(source), 0755)
	os.MkdirAll(filepath.Dir(dest), 0755)
	os.WriteFile(source, []byte("finished bytes"), 0644)
	marker, err := MarkComplete(source)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(marker)
	os.WriteFile(dest+CompletionSuffix, raw, 0644)
	if VerifyComplete(dest) == nil {
		t.Fatal("marker alone reported completion")
	}
	os.WriteFile(dest, []byte("unfinished"), 0644)
	if VerifyComplete(dest) == nil {
		t.Fatal("partial file reported complete")
	}
	data, _ := os.ReadFile(source)
	os.WriteFile(dest, data, 0644)
	if err := VerifyComplete(dest); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	os.WriteFile(dest, data, 0644)
	if VerifyComplete(dest) == nil {
		t.Fatal("stale marker accepted modified file")
	}
	os.WriteFile(source, nil, 0644)
	MarkComplete(source)
	if err := VerifyComplete(source); err != nil {
		t.Fatal("empty completed file rejected", err)
	}
}

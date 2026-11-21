//go:build linux || darwin

package servers

import (
	"golang.org/x/sys/unix"
	"path/filepath"
	"testing"
)

func TestSpecialFilesStayLocalWithoutBlockingScan(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	if err := unix.Mkfifo(filepath.Join(a.dir, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	write(t, a, "normal", []byte("data"))
	pull(t, b, a)
	equalFile(t, b, "normal", []byte("data"))
	absent(t, b, "pipe")
}

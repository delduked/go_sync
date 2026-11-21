//go:build linux

package servers

import (
	"golang.org/x/sys/unix"
	"os"
)

// A reflink gives staging its own inode with copy-on-write data. Unsupported
// filesystems fall back to the portable verified-copy path.
func cloneFile(dst, src *os.File) bool {
	return unix.IoctlFileClone(int(dst.Fd()), int(src.Fd())) == nil
}

package servers

import (
	"fmt"
	"os"
	"strings"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
)

// Unknown timestamps from legacy peers and synthetic conflict metadata do not
// manufacture local edits. New regular-file revisions carry millisecond mtimes.
func sameSnapshot(current, previous *pb.FileEntry) bool {
	return sameContent(current, previous) && (previous.ModifiedMillis == 0 || current.ModifiedMillis == previous.ModifiedMillis)
}
func (e *Engine) setModified(name string, entry *pb.FileEntry) error {
	if entry.ModifiedMillis == 0 || entry.IsDeleted || entry.IsDirectory {
		return nil
	}
	stamp := time.UnixMilli(entry.ModifiedMillis)
	return e.root.Chtimes(name, stamp, stamp)
}

// Reject aliases and special objects, including symlink parents inside the
// root. They remain local and must never be replaced by a remote revision.
func (e *Engine) checkLocalPath(name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := e.root.Lstat(strings.Join(parts[:i+1], "/"))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported local file type at %s", strings.Join(parts[:i+1], "/"))
		}
	}
	return nil
}
func (e *Engine) unsupportedLocked(name string) bool {
	for name != "" {
		if e.unsupported[name] != "" {
			return true
		}
		i := strings.LastIndexByte(name, '/')
		if i < 0 {
			break
		}
		name = name[:i]
	}
	return false
}

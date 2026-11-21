package servers

import (
	"os"
	"path/filepath"

	pb "github.com/TypeTerrors/go_sync/proto"
)

// The index is a hint, never authority: all reused bytes are hashed again and
// the complete result is verified. Missing/stale candidates only cost I/O.
func (e *Engine) indexContent(previous, next *pb.FileEntry) {
	if e.contentPaths == nil {
		e.contentPaths = make(map[string]map[string]bool)
	}
	if previous != nil && !previous.IsDeleted && !previous.IsDirectory {
		paths := e.contentPaths[previous.Hash]
		delete(paths, previous.FileName)
		if len(paths) == 0 {
			delete(e.contentPaths, previous.Hash)
		}
	}
	if next != nil && !next.IsDeleted && !next.IsDirectory {
		if e.contentPaths[next.Hash] == nil {
			e.contentPaths[next.Hash] = make(map[string]bool)
		}
		e.contentPaths[next.Hash][next.FileName] = true
	}
}
func regularBasis(file *os.File, err error) (*os.File, error) {
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = os.ErrInvalid
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
func (e *Engine) openBasis(entry *pb.FileEntry) (*os.File, error) {
	if file, err := regularBasis(e.root.Open(entry.FileName)); err == nil {
		return file, nil
	}
	e.mu.Lock()
	var candidates []string
	for name := range e.contentPaths[entry.Hash] {
		if !e.ignoredLocked(name, false) {
			candidates = append(candidates, name)
		}
	}
	e.mu.Unlock()
	for _, name := range candidates {
		if file, err := regularBasis(e.root.Open(name)); err == nil {
			return file, nil
		}
	}
	// Deletions are applied before creations. Their retained content is a useful
	// basis for a rename even when the old pathname has already disappeared.
	backups, err := filepath.Glob(filepath.Join(e.stateDir, "versions", "*-"+entry.Hash))
	if err != nil {
		return nil, err
	}
	for _, name := range backups {
		if file, err := regularBasis(os.Open(name)); err == nil {
			return file, nil
		}
	}
	return nil, os.ErrNotExist
}

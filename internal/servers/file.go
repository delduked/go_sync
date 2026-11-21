package servers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
	"github.com/fsnotify/fsnotify"
)

const stagePrefix = ".gosync-stage-"

type safeRoot struct{ *os.Root }

func validName(name string) error {
	if name == "" || name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
		return fmt.Errorf("invalid relative path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, stagePrefix) {
			return fmt.Errorf("reserved path %q", name)
		}
	}
	return nil
}
func NewEngine(folder, state string) (*Engine, error) {
	folder, err := filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(folder, 0755); err != nil {
		return nil, err
	}
	folder, err = filepath.EvalSymlinks(folder)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	state, err = filepath.EvalSymlinks(state)
	if err != nil {
		return nil, err
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(folder, state)
	if err != nil {
		return nil, err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("state directory must be outside sync folder")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	db, err := badger.Open(badger.DefaultOptions(state).WithLogger(nil).WithSyncWrites(true))
	if err != nil {
		root.Close()
		return nil, err
	}
	e := &Engine{root: &safeRoot{root}, db: db, stateDir: state, files: make(map[string]*pb.FileEntry)}
	if err = e.load(); err != nil {
		e.Close()
		return nil, err
	}
	if e.id == "" {
		e.id = randomID()
		err = db.Update(func(tx *badger.Txn) error { return tx.Set([]byte("node"), []byte(e.id)) })
		if err != nil {
			e.Close()
			return nil, err
		}
	}
	if err = e.Scan(); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

// fingerprint rejects special files and symlinks. os.Root also confines access
// when a path is swapped for an escaping symlink between validation and opening.
func (e *Engine) fingerprint(name string, allowGrowth bool) (*pb.FileEntry, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	info, err := e.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	r := &pb.FileEntry{FileName: name, Mode: uint32(info.Mode().Perm()), IsDirectory: info.IsDir()}
	if info.IsDir() {
		r.Mode = 0755 // Directory permissions are local; files retain their permission bits.
		return r, nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsupported file type: %s", name)
	}
	f, err := e.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	// Capture a bounded prefix rather than chasing a growing EOF.
	n, err := io.CopyN(h, f, before.Size())
	e.metrics.ScanRead.Add(n)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, before) || !os.SameFile(before, after) || ((before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime())) && !(allowGrowth && after.Size() > before.Size())) || n != before.Size() {
		return nil, fmt.Errorf("file changed during scan: %s", name)
	}
	r.FileSize = n
	r.Hash = hex.EncodeToString(h.Sum(nil))
	return r, nil
}
func (e *Engine) scanLocked() error {
	seen := make(map[string]bool)
	err := fs.WalkDir(e.root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), stagePrefix) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		seen[name] = true
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		current, err := e.fingerprint(name, true)
		if err != nil {
			return err
		}
		if !sameContent(current, e.files[name]) {
			return e.local(current)
		}
		return nil
	})
	if err != nil {
		return err
	} // Never infer deletions from an incomplete scan.
	for name, previous := range e.files {
		if !seen[name] && !previous.IsDeleted {
			if err = e.local(&pb.FileEntry{FileName: name, IsDeleted: true, IsDirectory: previous.IsDirectory}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *Engine) Scan() error { e.mu.Lock(); defer e.mu.Unlock(); return e.scanLocked() }
func (e *Engine) Manifest() (*pb.FileList, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.scanLocked(); err != nil {
		return nil, err
	}
	list := &pb.FileList{}
	for _, f := range e.files {
		list.Files = append(list.Files, copyEntry(f))
	}
	return list, nil
}

// Watch provides prompt notifications; periodic recursive scans repair missed
// events and register directories added since the last pass. Events are hints,
// not operations to replay, so remote commits cannot echo partial contents.
func (e *Engine) Watch(ctx context.Context, interval time.Duration) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("watcher: %v", err)
		return
	}
	defer watcher.Close()

	refresh := func() {
		if err := e.Scan(); err != nil {
			log.Warnf("scan: %v", err)
		}
		watched := map[string]bool{}
		for _, p := range watcher.WatchList() {
			watched[p] = true
		}
		filepath.WalkDir(e.root.Name(), func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && !watched[p] {
				if watcher.Add(p) == nil {
					watched[p] = true
				}
			}
			return nil
		})
	}
	refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// A fixed window bounds delay even during continuous writes. Scans capture
	// bounded prefixes, allowing verified snapshots while appenders keep running.
	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		case _, ok := <-watcher.Events:
			if !ok {
				return
			}
			if timerC == nil {
				timer = time.NewTimer(100 * time.Millisecond)
				timerC = timer.C
			}
		case <-timerC:
			timerC = nil
			refresh()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Warnf("watcher: %v", err)
			refresh()
		}
	}
}

// Retain overwritten regular files locally so deterministic revision ordering
// never silently destroys the only copy of an independently edited version.
func (e *Engine) backup(name string) error {
	f, err := e.root.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return nil
	}
	dir := filepath.Join(e.stateDir, "versions")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	out, err := os.CreateTemp(dir, "version-")
	if err != nil {
		return err
	}
	temp := out.Name()
	defer os.Remove(temp)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), f)
	e.metrics.BackupRead.Add(n)
	e.metrics.BackupWrite.Add(n)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte(name))
	return os.Rename(temp, filepath.Join(dir, fmt.Sprintf("%x-%x", key, h.Sum(nil))))
}

// Recheck only the target at commit time; full-tree repair scans run separately.
func (e *Engine) refreshLocked(name string) error {
	// A receiving destination must stay stable through the final check. Unlike
	// source scans, accepting a growing prefix here could hide a local append.
	current, err := e.fingerprint(name, false)
	previous := e.files[name]
	if errors.Is(err, os.ErrNotExist) {
		if previous != nil && !previous.IsDeleted {
			return e.local(&pb.FileEntry{FileName: name, IsDeleted: true, IsDirectory: previous.IsDirectory})
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !sameContent(current, previous) {
		return e.local(current)
	}
	return nil
}

func (e *Engine) apply(entry, expected *pb.FileEntry, staged string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Detect local edits made while the download was in flight.
	if err := e.refreshLocked(entry.FileName); err != nil {
		return err
	}
	current := e.files[entry.FileName]
	if !sameVersion(current, expected) {
		return fmt.Errorf("destination changed during transfer")
	}
	if !newer(entry, current) {
		return nil
	}
	// A same-origin append retains every previous byte in the new file. Avoid
	// archiving a complete growing file on every incremental snapshot.
	if !e.preservesPrefix(current, entry, staged) {
		if err := e.backup(entry.FileName); err != nil {
			return err
		}
	}
	if entry.IsDeleted {
		err := e.root.Remove(entry.FileName)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		} // Never recursively delete untracked local data.
	} else if entry.IsDirectory {
		if info, err := e.root.Lstat(entry.FileName); err == nil && !info.IsDir() {
			if err = e.root.Remove(entry.FileName); err != nil {
				return err
			}
		}
		if err := e.root.MkdirAll(entry.FileName, 0755); err != nil {
			return err
		}
	} else {
		if info, err := e.root.Lstat(entry.FileName); err == nil && info.IsDir() {
			if err = e.root.Remove(entry.FileName); err != nil {
				return err
			}
		}
		if err := e.root.Rename(staged, entry.FileName); err != nil {
			return err
		}
	}
	return e.save(entry)
}

func (e *Engine) stage(name string) (*os.File, string, error) {
	if err := validName(name); err != nil {
		return nil, "", err
	}
	parent := path.Dir(name)
	if err := e.root.MkdirAll(parent, 0755); err != nil {
		return nil, "", err
	}
	temp := path.Join(parent, stagePrefix+randomID())
	f, err := e.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	return f, temp, err
}

func (e *Engine) preservesPrefix(current, next *pb.FileEntry, staged string) bool {
	if current == nil || current.IsDeleted || current.IsDirectory || next.IsDeleted || next.IsDirectory || staged == "" || current.Origin != next.Origin || current.Mode != next.Mode || next.FileSize < current.FileSize {
		return false
	}
	file, err := e.root.Open(staged)
	if err != nil {
		return false
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.CopyN(h, file, current.FileSize)
	e.metrics.PrefixRead.Add(n)
	if err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == current.Hash
}

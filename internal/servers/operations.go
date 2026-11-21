package servers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
)

// Maintenance commands require existing state and a stopped service. Avoid
// scanning on open: verification must compare against the saved metadata.
func WithStoredState() Option { return func(e *Engine) error { e.skipInitialScan = true; return nil } }
func (e *Engine) StoredManifest() *pb.FileList {
	e.mu.Lock()
	defer e.mu.Unlock()
	list := &pb.FileList{}
	for _, f := range e.files {
		list.Files = append(list.Files, copyEntry(f))
	}
	sort.Slice(list.Files, func(i, j int) bool { return list.Files[i].FileName < list.Files[j].FileName })
	return list
}

type VerifyIssue struct {
	Path    string `json:"path"`
	Problem string `json:"problem"`
}

func (e *Engine) Verify() ([]VerifyIssue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadIgnoreLocked(); err != nil {
		return nil, err
	}
	issues := []VerifyIssue{}
	seen := map[string]bool{}
	err := fs.WalkDir(e.root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if privateName(d.Name()) || e.ignoredLocked(name, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		seen[name] = true
		if !d.IsDir() && !d.Type().IsRegular() {
			issues = append(issues, VerifyIssue{name, "unsupported local object"})
			return nil
		}
		current, err := e.fingerprint(name, false)
		if err != nil {
			issues = append(issues, VerifyIssue{name, err.Error()})
			return nil
		}
		if !sameSnapshot(current, e.files[name]) {
			issues = append(issues, VerifyIssue{name, "differs from saved revision (content, metadata or untracked path)"})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for name, f := range e.files {
		if !f.IsDeleted && !seen[name] && !e.ignoredLocked(name, f.IsDirectory) {
			issues = append(issues, VerifyIssue{name, "missing from disk"})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
	return issues, nil
}

type HistoryVersion struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Bytes    int64     `json:"bytes"`
	Retained time.Time `json:"retained"`
}

func (e *Engine) History(name string) ([]HistoryVersion, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(name))
	prefix := fmt.Sprintf("%x-", key)
	entries, err := os.ReadDir(filepath.Join(e.stateDir, "versions"))
	if os.IsNotExist(err) {
		return []HistoryVersion{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []HistoryVersion{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && len(entry.Name()) == 129 {
			hash := strings.TrimPrefix(entry.Name(), prefix)
			if _, err := hex.DecodeString(hash); err != nil {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			if info.Mode().IsRegular() {
				result = append(result, HistoryVersion{entry.Name(), hash, info.Size(), info.ModTime()})
			}
		}
	}
	return result, nil
}
func (e *Engine) RestoreVersion(name, id string) error {
	versions, err := e.History(name)
	if err != nil {
		return err
	}
	for _, version := range versions {
		if version.ID == id {
			file, err := os.Open(filepath.Join(e.stateDir, "versions", id))
			if err != nil {
				return err
			}
			defer file.Close()
			return e.restore(name, file, version.Hash, 0644)
		}
	}
	return fmt.Errorf("unknown retained version for %s", name)
}
func (e *Engine) RestoreConflict(id string) error {
	if len(id) != 32 {
		return fmt.Errorf("invalid conflict ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return err
	}
	dir := path.Join(conflictDirectory, id)
	data, err := e.root.ReadFile(path.Join(dir, "details.json"))
	if err != nil {
		return err
	}
	var details conflictDetails
	if err = json.Unmarshal(data, &details); err != nil {
		return err
	}
	if details.Deleted || details.Directory {
		return fmt.Errorf("conflict records a deletion/directory operation; inspect details and apply the desired local operation")
	}
	if details.Mode > 0777 {
		return fmt.Errorf("invalid conflict mode")
	}
	source := path.Join(dir, "data")
	if err = e.checkLocalPath(source); err != nil {
		return err
	}
	file, err := e.root.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	return e.restore(details.Path, file, details.Hash, os.FileMode(details.Mode))
}
func (e *Engine) restore(name string, source io.Reader, hash string, mode os.FileMode) error {
	if err := validName(name); err != nil {
		return err
	}
	if decoded, err := hex.DecodeString(hash); err != nil || len(decoded) != 32 {
		return fmt.Errorf("invalid retained hash")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadIgnoreLocked(); err != nil {
		return err
	}
	if e.ignoredLocked(name, false) {
		return fmt.Errorf("restore path is excluded")
	}
	if err := e.checkLocalPath(name); err != nil {
		return err
	}
	if err := e.checkPortableTarget(name); err != nil {
		return err
	}
	if err := e.refreshLocked(name); err != nil {
		return err
	}
	temp, staged, err := e.stage(name)
	if err != nil {
		return err
	}
	defer e.root.Remove(staged)
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(temp, h), source)
	if err == nil && hex.EncodeToString(h.Sum(nil)) != hash {
		err = fmt.Errorf("retained content failed verification")
	}
	if err == nil {
		err = temp.Chmod(mode)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = e.backup(name); err != nil {
		return err
	}
	if err = e.root.Rename(staged, name); err != nil {
		return err
	}
	// Restoring is a new, intentional local edit inheriting the current history.
	return e.scanLocked()
}
func (e *Engine) scanRequests() error {
	entries, err := os.ReadDir(filepath.Join(e.stateDir, "commands"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var requests []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "rescan-") && strings.HasSuffix(entry.Name(), ".request") {
			requests = append(requests, entry.Name())
		}
	}
	if len(requests) == 0 {
		return e.Scan()
	}
	if err = e.Rescan(); err != nil {
		return err
	}
	for _, name := range requests {
		if err = os.Remove(filepath.Join(e.stateDir, "commands", name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

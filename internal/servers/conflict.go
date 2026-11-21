package servers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	pb "github.com/TypeTerrors/go_sync/proto"
)

const conflictDirectory = ".gosync-conflicts"

type conflictDetails struct {
	Path      string `json:"path"`
	Origin    string `json:"origin"`
	Revision  uint64 `json:"revision"`
	Hash      string `json:"hash,omitempty"`
	Deleted   bool   `json:"deleted"`
	Directory bool   `json:"directory"`
	Mode      uint32 `json:"mode"`
}

func conflictID(f *pb.FileEntry) string {
	data, _ := json.Marshal(conflictDetails{f.FileName, f.Origin, f.Revision, f.Hash, f.IsDeleted, f.IsDirectory, f.Mode})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// Preserve a losing local version before replacing it. Its stable name and
// original causal history make this operation idempotent across all peers.
// Artifacts live at the root so deleting/renaming the old parent cannot hide them.
func (e *Engine) preserveConflictLocked(loser *pb.FileEntry) error {
	if loser == nil {
		return nil
	}
	return e.preserveConflictFromLocked(loser, loser.FileName)
}

func (e *Engine) preserveConflictFromLocked(loser *pb.FileEntry, source string) error {
	if loser == nil {
		return nil
	}
	dir := path.Join(conflictDirectory, conflictID(loser))
	if err := e.root.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, name := range []string{conflictDirectory, dir} {
		record, err := e.fingerprint(name, false)
		if err != nil {
			return err
		}
		if !sameContent(record, e.files[name]) {
			if err = e.local(record); err != nil {
				return err
			}
		}
	}
	if !loser.IsDeleted && !loser.IsDirectory {
		name := path.Join(dir, "data")
		if _, err := e.root.Lstat(name); errors.Is(err, os.ErrNotExist) {
			original, err := e.root.Open(source)
			if err != nil {
				return err
			}
			temp, staged, err := e.stage(name)
			if err != nil {
				original.Close()
				return err
			}
			h := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(temp, h), original)
			original.Close()
			e.metrics.BackupRead.Add(n)
			e.metrics.BackupWrite.Add(n)
			if copyErr == nil && (n != loser.FileSize || hex.EncodeToString(h.Sum(nil)) != loser.Hash) {
				copyErr = fmt.Errorf("conflicting file changed while preserving it")
			}
			if copyErr == nil {
				copyErr = temp.Chmod(os.FileMode(loser.Mode))
			}
			if copyErr == nil {
				copyErr = temp.Sync()
			}
			closeErr := temp.Close()
			if copyErr == nil {
				copyErr = closeErr
			}
			if copyErr != nil {
				e.root.Remove(staged)
				return copyErr
			}
			if err = e.setModified(staged, loser); err != nil {
				e.root.Remove(staged)
				return err
			}
			if err = e.root.Rename(staged, name); err != nil {
				e.root.Remove(staged)
				return err
			}
			record := copyEntry(loser)
			record.FileName = name
			if err = e.save(record); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	details := conflictDetails{loser.FileName, loser.Origin, loser.Revision, loser.Hash, loser.IsDeleted, loser.IsDirectory, loser.Mode}
	data, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return err
	}
	name := path.Join(dir, "details.json")
	if _, err = e.root.Lstat(name); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, staged, err := e.stage(name)
	if err != nil {
		return err
	}
	defer e.root.Remove(staged)
	_, err = temp.Write(data)
	if err == nil {
		err = temp.Chmod(0644)
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
	if err = e.root.Rename(staged, name); err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	return e.save(&pb.FileEntry{FileName: name, FileSize: int64(len(data)), Hash: hex.EncodeToString(hash[:]), Mode: 0644, Origin: loser.Origin, Revision: loser.Revision, Version: vector(loser)})
}

// A parent deletion/type replacement cannot erase locally retained children.
// Preserve the incoming operation and emit a causally newer retained directory.
func (e *Engine) retainDirectoryLocked(incoming, current *pb.FileEntry, staged string) (bool, error) {
	if current == nil || current.IsDeleted || !current.IsDirectory || (!incoming.IsDeleted && incoming.IsDirectory) {
		return false, nil
	}
	dir, err := e.root.Open(current.FileName)
	if err != nil {
		return false, err
	}
	children, err := dir.ReadDir(1)
	dir.Close()
	if err != nil && err != io.EOF {
		return false, err
	}
	if len(children) == 0 {
		return false, nil
	}
	source := incoming.FileName
	if !incoming.IsDeleted && !incoming.IsDirectory {
		source = staged
	}
	if err = e.preserveConflictFromLocked(incoming, source); err != nil {
		return true, err
	}
	retained := copyEntry(current)
	retained.Version = mergedVersion(current, incoming)
	if incoming.Revision > e.clock {
		e.clock = incoming.Revision
	}
	return true, e.local(retained)
}

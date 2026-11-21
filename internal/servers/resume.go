package servers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pathLock struct {
	mu   sync.Mutex
	refs int
}

func (e *Engine) lockTransfer(name string) func() {
	e.transferMu.Lock()
	if e.transfers == nil {
		e.transfers = make(map[string]*pathLock)
	}
	lock := e.transfers[name]
	if lock == nil {
		lock = &pathLock{}
		e.transfers[name] = lock
	}
	lock.refs++
	e.transferMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		e.transferMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(e.transfers, name)
		}
		e.transferMu.Unlock()
	}
}

type resumeRecord struct {
	Name    string    `json:"name"`
	Updated time.Time `json:"updated"`
}

func resumeKey(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}
func resumeName(name string) string { return path.Join(path.Dir(name), stagePrefix+resumeKey(name)) }
func (e *Engine) resumeJournal(name string) string {
	return filepath.Join(e.stateDir, "transfers", resumeKey(name)+".json")
}
func (e *Engine) discardResume(name string) {
	e.root.Remove(resumeName(name))
	os.Remove(e.resumeJournal(name))
}
func (e *Engine) resumeStage(name string) (*os.File, string, bool, error) {
	if err := validName(name); err != nil {
		return nil, "", false, err
	}
	staged := resumeName(name)
	if err := e.root.MkdirAll(path.Dir(name), 0755); err != nil {
		return nil, "", false, err
	}
	resume := false
	if info, err := e.root.Lstat(staged); err == nil {
		if !info.Mode().IsRegular() {
			return nil, "", false, fmt.Errorf("invalid partial transfer file")
		}
		// An orphaned file without our matching journal is not trusted as staging.
		var record resumeRecord
		raw, readErr := os.ReadFile(e.resumeJournal(name))
		resume = readErr == nil && json.Unmarshal(raw, &record) == nil && record.Name == name
		if !resume {
			if err = e.root.Remove(staged); err != nil {
				return nil, "", false, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", false, err
	}
	file, err := e.root.OpenFile(staged, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, "", false, err
	}
	dir := filepath.Dir(e.resumeJournal(name))
	if err = os.MkdirAll(dir, 0700); err != nil {
		file.Close()
		return nil, "", false, err
	}
	data, _ := json.Marshal(resumeRecord{Name: name, Updated: time.Now()})
	journal, err := os.CreateTemp(dir, "journal-")
	if err != nil {
		file.Close()
		return nil, "", false, err
	}
	defer os.Remove(journal.Name())
	_, err = journal.Write(data)
	if err == nil {
		err = journal.Sync()
	}
	closeErr := journal.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(journal.Name(), e.resumeJournal(name))
	}
	if err != nil {
		file.Close()
		return nil, "", false, err
	}
	return file, staged, resume, nil
}
func keepPartial(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch status.Code(err) {
	case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable, codes.Aborted:
		return true
	}
	return false
}

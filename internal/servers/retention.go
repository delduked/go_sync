package servers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v3"
)

// Compact deletion floors retain the minimum causal information indefinitely.
// They are omitted from normal manifests, then reactivated if a peer mentions
// the old path. Blindly forgetting deletions would resurrect offline copies.
func (e *Engine) RestoreDeletionFloors(names []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, name := range names {
		if err := validName(name); err != nil {
			return err
		}
		if f := e.graveyard[name]; f != nil {
			if err := e.save(f); err != nil {
				return err
			}
		}
	}
	return nil
}

type PruneResult struct {
	TombstonesCompacted int `json:"tombstones_compacted"`
	BackupsRemoved      int `json:"backups_removed"`
	PartialsRemoved     int `json:"partials_removed"`
}

func (e *Engine) Prune(cutoff time.Time) (PruneResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var result PruneResult
	for name, f := range e.files {
		if !f.IsDeleted || !e.deletedAt[name].Before(cutoff) {
			continue
		}
		data, err := json.Marshal(f)
		if err != nil {
			return result, err
		}
		if err = e.db.Update(func(tx *badger.Txn) error {
			if err := tx.Set([]byte("graveyard/"+name), data); err != nil {
				return err
			}
			if err := tx.Delete([]byte("file/" + name)); err != nil {
				return err
			}
			return tx.Delete([]byte("deleted-at/" + name))
		}); err != nil {
			return result, err
		}
		e.graveyard[name] = copyEntry(f)
		delete(e.files, name)
		delete(e.deletedAt, name)
		e.generation++
		result.TombstonesCompacted++
	}
	backups, err := os.ReadDir(filepath.Join(e.stateDir, "versions"))
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	for _, entry := range backups {
		info, err := entry.Info()
		if err != nil {
			return result, err
		}
		if info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			if err = os.Remove(filepath.Join(e.stateDir, "versions", entry.Name())); err != nil {
				return result, err
			}
			result.BackupsRemoved++
		}
	}
	journals, err := os.ReadDir(filepath.Join(e.stateDir, "transfers"))
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	for _, entry := range journals {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(e.stateDir, "transfers", entry.Name()))
		if err != nil {
			return result, err
		}
		var record resumeRecord
		if json.Unmarshal(data, &record) != nil || validName(record.Name) != nil || entry.Name() != resumeKey(record.Name)+".json" || !record.Updated.Before(cutoff) {
			continue
		}
		e.transferMu.Lock()
		if e.transfers[record.Name] == nil {
			e.discardResume(record.Name)
			result.PartialsRemoved++
		}
		e.transferMu.Unlock()
	}
	return result, nil
}

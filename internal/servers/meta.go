package servers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

// Engine serializes metadata changes and filesystem commits, never network I/O.
// Records, including tombstones, survive restarts; the clock never goes backwards.
type Engine struct {
	session      string
	generation   uint64
	unsupported  map[string]string
	contentPaths map[string]map[string]bool
	ignores      []ignoreRule
	bootstrap    string
	transferMu   sync.Mutex
	transfers    map[string]*pathLock
	// Set before starting workers. Zero selects 64 KiB.
	BlockSize int
	metrics   Metrics

	RepairInterval time.Duration
	cache          map[string]fingerprintCache
	mu             sync.Mutex
	root           *safeRoot
	db             *badger.DB
	stateDir       string
	id             string
	clock          uint64
	files          map[string]*pb.FileEntry
	graveyard      map[string]*pb.FileEntry
	deletedAt      map[string]time.Time
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func copyEntry(e *pb.FileEntry) *pb.FileEntry {
	if e == nil {
		return nil
	}
	return proto.Clone(e).(*pb.FileEntry)
}
func newer(a, b *pb.FileEntry) bool {
	return b == nil || a.Revision > b.Revision || (a.Revision == b.Revision && a.Origin > b.Origin)
}
func sameVersion(a, b *pb.FileEntry) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && proto.Equal(a, b))
}
func sameContent(a, b *pb.FileEntry) bool {
	return a != nil && b != nil && a.IsDeleted == b.IsDeleted && a.Hash == b.Hash && a.FileSize == b.FileSize && a.Mode == b.Mode && a.IsDirectory == b.IsDirectory
}

func (e *Engine) load() error {
	return e.db.View(func(tx *badger.Txn) error {
		if item, err := tx.Get([]byte("node")); err == nil {
			if err := item.Value(func(b []byte) error { e.id = string(b); return nil }); err != nil {
				return err
			}
		} else if err != badger.ErrKeyNotFound {
			return err
		}
		it := tx.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte("file/")); it.ValidForPrefix([]byte("file/")); it.Next() {
			var f pb.FileEntry
			if err := it.Item().Value(func(b []byte) error { return json.Unmarshal(b, &f) }); err != nil {
				return err
			}
			if err := validName(f.FileName); err != nil {
				return fmt.Errorf("invalid stored path: %w", err)
			}
			e.files[f.FileName] = &f
			e.indexContent(nil, &f)
			if f.Revision > e.clock {
				e.clock = f.Revision
			}
		}
		if item, err := tx.Get([]byte("clock")); err == nil {
			var clock uint64
			if err = item.Value(func(b []byte) error { return json.Unmarshal(b, &clock) }); err != nil {
				return err
			}
			if clock > e.clock {
				e.clock = clock
			}
		} else if err != badger.ErrKeyNotFound {
			return err
		}
		for it.Seek([]byte("graveyard/")); it.ValidForPrefix([]byte("graveyard/")); it.Next() {
			var f pb.FileEntry
			if err := it.Item().Value(func(b []byte) error { return json.Unmarshal(b, &f) }); err != nil {
				return err
			}
			if err := validateEntry(&f); err != nil {
				return err
			}
			e.graveyard[f.FileName] = &f
			if f.Revision > e.clock {
				e.clock = f.Revision
			}
		}
		for name, f := range e.files {
			if f.IsDeleted {
				when := time.Now()
				if item, err := tx.Get([]byte("deleted-at/" + name)); err == nil {
					if err = item.Value(func(b []byte) error { return json.Unmarshal(b, &when) }); err != nil {
						return err
					}
				} else if err != badger.ErrKeyNotFound {
					return err
				}
				e.deletedAt[name] = when
			}
		}
		return nil
	})
}
func (e *Engine) save(f *pb.FileEntry) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	clock := e.clock
	if f.Revision > clock {
		clock = f.Revision
	}
	clockData, _ := json.Marshal(clock)
	when := e.deletedAt[f.FileName]
	if f.IsDeleted && (when.IsZero() || !sameVersion(f, e.files[f.FileName])) {
		when = time.Now()
	}
	whenData, _ := json.Marshal(when)
	if err = e.db.Update(func(tx *badger.Txn) error {
		if err := tx.Set([]byte("file/"+f.FileName), data); err != nil {
			return err
		}
		if err := tx.Set([]byte("clock"), clockData); err != nil {
			return err
		}
		if err := tx.Delete([]byte("graveyard/" + f.FileName)); err != nil {
			return err
		}
		if f.IsDeleted {
			return tx.Set([]byte("deleted-at/"+f.FileName), whenData)
		}
		return tx.Delete([]byte("deleted-at/" + f.FileName))
	}); err != nil {
		return err
	}
	e.generation++
	e.indexContent(e.files[f.FileName], f)
	e.files[f.FileName] = copyEntry(f)
	delete(e.graveyard, f.FileName)
	e.clock = clock
	if f.IsDeleted {
		e.deletedAt[f.FileName] = when
	} else {
		delete(e.deletedAt, f.FileName)
	}
	return nil
}
func (e *Engine) local(f *pb.FileEntry) error {
	previous := e.files[f.FileName]
	if previous == nil {
		previous = e.graveyard[f.FileName]
	}
	f.Version = mergedVersion(f, previous)
	e.clock++
	f.Revision = e.clock
	f.Origin = e.id
	f.Version[e.id] = f.Revision
	return e.save(f)
}
func (e *Engine) Close() error { e.root.Close(); return e.db.Close() }
func (e *Engine) ID() string   { return e.id }

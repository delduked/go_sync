package servers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/dgraph-io/badger/v3"
	"google.golang.org/protobuf/proto"
)

// Engine serializes metadata changes and filesystem commits, never network I/O.
// Records, including tombstones, survive restarts; the clock never goes backwards.
type Engine struct {
	// Set before starting workers. Zero selects 64 KiB.
	BlockSize int
	metrics   Metrics

	mu       sync.Mutex
	root     *safeRoot
	db       *badger.DB
	stateDir string
	id       string
	clock    uint64
	files    map[string]*pb.FileEntry
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
			if f.Revision > e.clock {
				e.clock = f.Revision
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
	if err = e.db.Update(func(tx *badger.Txn) error { return tx.Set([]byte("file/"+f.FileName), data) }); err != nil {
		return err
	}
	e.files[f.FileName] = copyEntry(f)
	if f.Revision > e.clock {
		e.clock = f.Revision
	}
	return nil
}
func (e *Engine) local(f *pb.FileEntry) error {
	e.clock++
	f.Revision = e.clock
	f.Origin = e.id
	return e.save(f)
}
func (e *Engine) Close() error { e.root.Close(); return e.db.Close() }
func (e *Engine) ID() string   { return e.id }

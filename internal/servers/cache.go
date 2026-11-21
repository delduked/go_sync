package servers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
)

// The cache is only an optimization. Watch events invalidate it, attributes are
// checked on every scan, and a full hash is forced after RepairInterval. It is
// deliberately not persisted: a restart starts with an independent verification.
type fingerprintCache struct {
	info    os.FileInfo
	entry   *pb.FileEntry
	checked time.Time
	blocks  [][sha256.Size]byte
}

func sameAttributes(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}
func (e *Engine) cacheFresh(c fingerprintCache, info os.FileInfo) bool {
	ttl := e.RepairInterval
	if ttl == 0 {
		ttl = time.Minute
	}
	return c.info != nil && sameAttributes(c.info, info) && time.Since(c.checked) < ttl
}
func (e *Engine) Invalidate(name string) { e.mu.Lock(); delete(e.cache, name); e.mu.Unlock() }
func (e *Engine) Rescan() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	clear(e.cache)
	return e.scanLocked()
}
func (e *Engine) hashFile(file *os.File, size int64) (string, [][sha256.Size]byte, int64, error) {
	h := sha256.New()
	buf := make([]byte, defaultBlockSize)
	var blocks [][sha256.Size]byte
	var total int64
	for total < size {
		n, err := io.ReadFull(file, buf[:min(int64(len(buf)), size-total)])
		total += int64(n)
		if n > 0 {
			h.Write(buf[:n])
			blocks = append(blocks, sha256.Sum256(buf[:n]))
		}
		if err != nil {
			return "", nil, total, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), blocks, total, nil
}

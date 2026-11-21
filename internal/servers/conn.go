package servers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Reconcile fetches durable state instead of enqueuing lossy filesystem events.
// A failed transfer stays pending in the manifest and is retried next pass.
func (e *Engine) Reconcile(ctx context.Context, client pb.FileSyncServiceClient) error {
	e.mu.Lock()
	if err := e.loadIgnoreLocked(); err != nil {
		e.mu.Unlock()
		return err
	}
	var known []string
	for name, f := range e.files {
		if !f.IsDeleted && !e.ignoredLocked(name, f.IsDirectory) {
			known = append(known, name)
		}
	}
	e.mu.Unlock()
	remote, err := fetchManifest(ctx, client, known)
	if err != nil {
		return err
	}
	for _, f := range remote.Files {
		if err = validateEntry(f); err != nil {
			return err
		}
	}
	sort.Slice(remote.Files, func(i, j int) bool {
		a, b := remote.Files[i], remote.Files[j]
		if (a.FileName == ignoreFile) != (b.FileName == ignoreFile) {
			return a.FileName == ignoreFile
		}
		if a.IsDeleted != b.IsDeleted {
			return a.IsDeleted
		} // Remove old paths before creating replacements.
		da, db := strings.Count(a.FileName, "/"), strings.Count(b.FileName, "/")
		if da != db {
			if a.IsDeleted {
				return da > db
			}
			return da < db
		}
		return a.FileName < b.FileName
	})
	var firstErr error
	for _, f := range remote.Files {
		if err = ctx.Err(); err != nil {
			return err
		}
		excluded, excludeErr := e.excluded(f.FileName, f.IsDirectory)
		if excludeErr != nil {
			return excludeErr
		}
		if excluded {
			continue
		}
		if err = e.RestoreDeletionFloors([]string{f.FileName}); err != nil {
			return err
		}
		e.mu.Lock()
		local := copyEntry(e.files[f.FileName])
		e.mu.Unlock()
		if shouldReceive(f, local) {
			if err = e.pull(ctx, client, f, local); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", f.FileName, err)
			}
		}
	}
	return firstErr
}

type Connections struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	peers    map[string]context.CancelFunc
	engine   *Engine
	options  []grpc.DialOption
	interval time.Duration
	closed   bool
}

func NewConnections(ctx context.Context, e *Engine, interval time.Duration, options ...grpc.DialOption) *Connections {
	ctx, cancel := context.WithCancel(ctx)
	return &Connections{ctx: ctx, cancel: cancel, peers: make(map[string]context.CancelFunc), engine: e, interval: interval, options: options}
}
func (c *Connections) AddPeer(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.peers[addr] != nil {
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.peers[addr] = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		conn, err := grpc.NewClient(addr, c.options...)
		if err != nil {
			log.Errorf("peer %s: %v", addr, err)
			return
		}
		defer conn.Close()
		client := pb.NewFileSyncServiceClient(conn)
		var lastRemote, lastLocal string
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				callCtx, done := context.WithTimeout(ctx, 10*time.Minute)
				announcement, err := client.Announce(callCtx, &pb.Empty{})
				localGeneration := c.engine.Generation()
				if status.Code(err) == codes.Unimplemented {
					err = c.engine.Reconcile(callCtx, client)
				} else if err == nil && (announcement.Generation != lastRemote || localGeneration != lastLocal) {
					err = c.engine.Reconcile(callCtx, client)
					if err == nil {
						lastRemote = announcement.Generation
						lastLocal = c.engine.Generation()
					}
				}
				done()
				if err != nil && ctx.Err() == nil {
					log.Debugf("peer %s: %v", addr, err)
				}
				timer.Reset(c.interval)
			}
		}
	}()
}
func (c *Connections) Close() { c.mu.Lock(); c.closed = true; c.cancel(); c.mu.Unlock(); c.wg.Wait() }

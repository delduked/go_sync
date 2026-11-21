package servers

import (
	"encoding/hex"
	"errors"
	"sort"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
)

var errDeferred = errors.New("waiting for preferred relay")

type routePolicy struct {
	allow func(*pb.FileEntry) bool
	done  func(*pb.FileEntry, error)
}

// Every origin gets its own tree. Node identities, not addresses or discovery
// order, determine edges. Data is still verified independently at every relay.
func relayParent(self, origin string, ids []string, fanout int) string {
	if fanout <= 0 || self == origin {
		return ""
	}
	set := map[string]bool{self: true}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	delete(set, origin)
	rest := make([]string, 0, len(set))
	for id := range set {
		rest = append(rest, id)
	}
	sort.Strings(rest)
	ordered := append([]string{origin}, rest...)
	for i, id := range ordered {
		if id == self {
			return ordered[(i-1)/fanout]
		}
	}
	return ""
}
func validID(id string) bool { _, err := hex.DecodeString(id); return len(id) == 32 && err == nil }
func (c *Connections) noteNode(addr, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodes[addr] != id {
		known := id == c.engine.ID()
		for _, node := range c.nodes {
			if node == id {
				known = true
			}
		}
		c.nodes[addr] = id
		if !known {
			c.membershipChanged = time.Now()
		}
	}
}

// SetFanout must be called before AddPeer. Zero selects the full-mesh baseline.
func (c *Connections) SetFanout(fanout int) { c.fanout = fanout }
func (c *Connections) route(addr string) *routePolicy {
	return &routePolicy{allow: func(entry *pb.FileEntry) bool {
		if entry.IsDeleted || entry.IsDirectory {
			return true
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		source := c.nodes[addr]
		if c.fanout == 0 || source == "" || entry.Origin == c.engine.ID() {
			return true
		}
		if time.Since(c.membershipChanged) < 2*c.interval {
			return false
		}
		ids := make([]string, 0, len(c.nodes))
		for _, id := range c.nodes {
			ids = append(ids, id)
		}
		parent := relayParent(c.engine.ID(), entry.Origin, ids, c.fanout)
		if source == parent {
			return true
		}
		since, ok := c.pending[entry.FileName]
		if !ok {
			since = time.Now()
			c.pending[entry.FileName] = since
		}
		// A partition, departed origin or unequal membership cannot leave a branch
		// waiting forever. Any authenticated holder becomes eligible after the grace.
		return time.Since(since) >= max(3*time.Second, 4*c.interval)
	}, done: func(entry *pb.FileEntry, err error) {
		if err == nil {
			c.mu.Lock()
			delete(c.pending, entry.FileName)
			c.mu.Unlock()
		}
	}}
}

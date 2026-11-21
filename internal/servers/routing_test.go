package servers

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "github.com/TypeTerrors/go_sync/proto"
)

func TestRelayTreesBoundFanoutFromEveryOrigin(t *testing.T) {
	ids := make([]string, 10)
	for i := range ids {
		ids[i] = fmt.Sprintf("%032x", i+1)
	}
	for _, origin := range ids {
		children := map[string]int{}
		for _, self := range ids {
			if self == origin {
				continue
			}
			parent := relayParent(self, origin, ids, 2)
			children[parent]++
			visited := map[string]bool{self: true}
			for parent != origin {
				if visited[parent] || parent == "" {
					t.Fatal("routing cycle")
				}
				visited[parent] = true
				parent = relayParent(parent, origin, ids, 2)
			}
		}
		for node, count := range children {
			if count > 2 {
				t.Fatalf("%s has %d children", node, count)
			}
		}
	}
}
func TestMissingRelayFallsBackAndSuccessfulRetryResetsWait(t *testing.T) {
	f := newFixture(t)
	f.e.id = "ffffffffffffffffffffffffffffffff"
	c := NewConnections(context.Background(), f.e, 10*time.Millisecond)
	defer c.Close()
	ids := []string{f.e.ID(), "00000000000000000000000000000001", "00000000000000000000000000000002"}
	c.nodes = map[string]string{"origin": ids[1], "relay": ids[2]}
	c.fanout = 1
	entry := &pb.FileEntry{FileName: "file", Origin: ids[1]}
	route := c.route("origin")
	if route.allow(entry) {
		t.Fatal("skipped preferred relay")
	}
	c.pending[entry.FileName] = time.Now().Add(-4 * time.Second)
	if !route.allow(entry) {
		t.Fatal("failed to fall back")
	}
	route.done(entry, nil)
	if _, ok := c.pending[entry.FileName]; ok {
		t.Fatal("successful transfer kept stale wait")
	}
}

package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusExplainsProgressAndFailures(t *testing.T) {
	f := newFixture(t)
	write(t, f, "source", []byte("data"))
	f.e.startProgress("large.bin", 1000)
	f.e.addProgress("large.bin", 250)
	c := NewConnections(context.Background(), f.e, 1)
	defer c.Close()
	c.recordResult("peer:50051", fmt.Errorf("certificate expired"))
	if err := f.e.WriteStatus(c.Status(), true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(f.state, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s StatusSnapshot
	if err = json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Files != 1 || s.NodeID != f.e.ID() || len(s.Transfers) != 1 || s.Transfers[0].ReceivedBytes != 250 || len(s.Peers) != 1 || s.Peers[0].LastError != "certificate expired" || !s.Peers[0].Pending {
		t.Fatalf("incomplete status: %+v", s)
	}
	f.e.endProgress("large.bin")
	c.recordResult("peer:50051", nil)
	s = f.e.Status(c.Status(), false)
	if len(s.Transfers) != 0 || s.Peers[0].Pending || s.Running {
		t.Fatal("stale status persisted")
	}
}

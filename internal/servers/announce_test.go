package servers

import (
	"context"
	"testing"

	pb "github.com/TypeTerrors/go_sync/proto"
)

func TestAnnouncementsTrackChangesAndRestart(t *testing.T) {
	f := newFixture(t)
	serve(t, f)
	first, err := f.client.Announce(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.client.Announce(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation != second.Generation || first.NodeId != f.e.ID() {
		t.Fatal("idle announcement changed")
	}
	write(t, f, "source.go", []byte("package main"))
	third, err := f.client.Announce(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if third.Generation == first.Generation {
		t.Fatal("change was not announced")
	}
	if err = f.e.Close(); err != nil {
		t.Fatal(err)
	}
	f.e, err = NewEngine(f.dir, f.state)
	if err != nil {
		t.Fatal(err)
	}
	if f.e.Generation() == third.Generation || f.e.ID() != third.NodeId {
		t.Fatal("restart generation/identity incorrect")
	}
}

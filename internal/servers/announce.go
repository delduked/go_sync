package servers

import (
	"context"
	"fmt"

	pb "github.com/TypeTerrors/go_sync/proto"
)

func (e *Engine) generationLocked() string { return fmt.Sprintf("%s:%d", e.session, e.generation) }
func (e *Engine) Generation() string       { e.mu.Lock(); defer e.mu.Unlock(); return e.generationLocked() }
func (s *Service) Announce(context.Context, *pb.Empty) (*pb.Announcement, error) {
	s.Engine.mu.Lock()
	defer s.Engine.mu.Unlock()
	return &pb.Announcement{NodeId: s.Engine.id, Generation: s.Engine.generationLocked()}, nil
}

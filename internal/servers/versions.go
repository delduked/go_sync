package servers

import pb "github.com/TypeTerrors/go_sync/proto"

type order int

const (
	equal order = iota
	before
	after
	concurrent
)

// Effective vectors also understand pre-vector v2 records. Legacy updates may
// appear concurrent; treating them conservatively avoids silent data loss.
func vector(f *pb.FileEntry) map[string]uint64 {
	v := make(map[string]uint64)
	if f == nil {
		return v
	}
	for id, n := range f.Version {
		v[id] = n
	}
	if f.Revision > v[f.Origin] {
		v[f.Origin] = f.Revision
	}
	return v
}
func compare(a, b *pb.FileEntry) order {
	av, bv := vector(a), vector(b)
	less, greater := false, false
	for id, n := range av {
		if n > bv[id] {
			greater = true
		}
		if n < bv[id] {
			less = true
		}
	}
	for id, n := range bv {
		if n > av[id] {
			less = true
		}
	}
	if less && greater {
		return concurrent
	}
	if less {
		return before
	}
	if greater {
		return after
	}
	return equal
}
func mergedVersion(a, b *pb.FileEntry) map[string]uint64 {
	v := vector(a)
	for id, n := range vector(b) {
		if n > v[id] {
			v[id] = n
		}
	}
	return v
}
func shouldReceive(incoming, current *pb.FileEntry) bool {
	if current == nil {
		return true
	}
	switch compare(incoming, current) {
	case after:
		return true
	case concurrent:
		return newer(incoming, current) || sameContent(incoming, current)
	case equal:
		return sameContent(incoming, current) && newer(incoming, current)
	}
	return false
}

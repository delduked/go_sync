package servers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TransferProgress struct {
	Path          string    `json:"path"`
	TargetBytes   int64     `json:"target_bytes"`
	ReceivedBytes int64     `json:"payload_received_this_attempt"`
	Started       time.Time `json:"started"`
}
type PeerStatus struct {
	Address     string    `json:"address"`
	NodeID      string    `json:"node_id,omitempty"`
	LastAttempt time.Time `json:"last_attempt"`
	LastSuccess time.Time `json:"last_success"`
	Pending     bool      `json:"pending"`
	LastError   string    `json:"last_error,omitempty"`
}
type StatusSnapshot struct {
	Updated         time.Time          `json:"updated"`
	Running         bool               `json:"running"`
	NodeID          string             `json:"node_id"`
	FolderID        string             `json:"folder_id"`
	Root            string             `json:"root"`
	Generation      string             `json:"generation"`
	Files           int                `json:"files"`
	Tombstones      int                `json:"tombstones"`
	DeletionFloors  int                `json:"deletion_floors"`
	Conflicts       int                `json:"conflicts"`
	Partials        int                `json:"partial_transfers"`
	ActiveUploads   int                `json:"active_uploads"`
	ActiveDownloads int                `json:"active_downloads"`
	LastScan        time.Time          `json:"last_scan"`
	ScanError       string             `json:"scan_error,omitempty"`
	Unsupported     map[string]string  `json:"unsupported_paths,omitempty"`
	Transfers       []TransferProgress `json:"transfers"`
	Peers           []PeerStatus       `json:"peers"`
	Metrics         MetricsSnapshot    `json:"metrics"`
}
type progressTracker struct {
	sync.Mutex
	paths map[string]TransferProgress
}

func (e *Engine) startProgress(name string, size int64) {
	e.progress.Lock()
	defer e.progress.Unlock()
	if e.progress.paths == nil {
		e.progress.paths = make(map[string]TransferProgress)
	}
	e.progress.paths[name] = TransferProgress{Path: name, TargetBytes: size, Started: time.Now()}
}
func (e *Engine) addProgress(name string, n int64) {
	e.progress.Lock()
	defer e.progress.Unlock()
	p := e.progress.paths[name]
	p.ReceivedBytes += n
	e.progress.paths[name] = p
}
func (e *Engine) endProgress(name string) {
	e.progress.Lock()
	defer e.progress.Unlock()
	delete(e.progress.paths, name)
}
func (c *Connections) recordResult(addr string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.status[addr]
	s.Address = addr
	s.NodeID = c.nodes[addr]
	s.LastAttempt = time.Now()
	s.Pending = err != nil
	s.LastError = ""
	if err == nil {
		s.LastSuccess = s.LastAttempt
	} else if err != errDeferred {
		s.LastError = err.Error()
	}
	c.status[addr] = s
}
func (c *Connections) Status() []PeerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]PeerStatus, 0, len(c.status))
	for _, s := range c.status {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result
}
func (e *Engine) Status(peers []PeerStatus, running bool) StatusSnapshot {
	e.mu.Lock()
	result := StatusSnapshot{Updated: time.Now(), Running: running, NodeID: e.id, FolderID: e.folderID, Root: e.root.Name(), Generation: e.generationLocked(), DeletionFloors: len(e.graveyard), LastScan: e.lastScan, ScanError: e.lastScanError, Unsupported: make(map[string]string), Peers: peers, Metrics: e.Metrics(), Transfers: []TransferProgress{}}
	for name, f := range e.files {
		if f.IsDeleted {
			result.Tombstones++
		} else {
			result.Files++
			if strings.HasPrefix(name, conflictDirectory+"/") && strings.HasSuffix(name, "/details.json") {
				result.Conflicts++
			}
		}
	}
	for name, reason := range e.unsupported {
		result.Unsupported[name] = reason
	}
	e.mu.Unlock()
	e.progress.Lock()
	for _, p := range e.progress.paths {
		result.Transfers = append(result.Transfers, p)
	}
	e.progress.Unlock()
	sort.Slice(result.Transfers, func(i, j int) bool { return result.Transfers[i].Path < result.Transfers[j].Path })
	partials, _ := os.ReadDir(filepath.Join(e.stateDir, "transfers"))
	result.Partials = len(partials)
	result.ActiveUploads = len(e.limits.send)
	result.ActiveDownloads = len(e.limits.receive)
	return result
}
func (e *Engine) WriteStatus(peers []PeerStatus, running bool) error {
	data, err := json.MarshalIndent(e.Status(peers, running), "", "  ")
	if err != nil {
		return err
	}
	out, err := os.CreateTemp(e.stateDir, "status-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	_, err = out.Write(data)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(out.Name(), filepath.Join(e.stateDir, "status.json"))
}

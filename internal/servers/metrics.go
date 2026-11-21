package servers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
)

// Metrics count logical regular-file I/O, not kernel/physical disk traffic.
// They exclude database, metadata and protocol framing/TLS overhead. Counters
// include failed attempts and reset when the process restarts.
type Metrics struct {
	ScanRead        atomic.Int64
	SourceRead      atomic.Int64
	BasisRead       atomic.Int64
	StageWrite      atomic.Int64
	VerifyRead      atomic.Int64
	BackupRead      atomic.Int64
	BackupWrite     atomic.Int64
	PrefixRead      atomic.Int64
	PayloadSent     atomic.Int64
	PayloadReceived atomic.Int64
	Reused          atomic.Int64
	Transfers       atomic.Int64
	Failures        atomic.Int64
	TransferNanos   atomic.Int64
}
type MetricsSnapshot struct {
	ScanRead        int64 `json:"scan_read_bytes"`
	SourceRead      int64 `json:"source_read_bytes"`
	BasisRead       int64 `json:"basis_read_bytes"`
	StageWrite      int64 `json:"stage_write_bytes"`
	VerifyRead      int64 `json:"verify_read_bytes"`
	BackupRead      int64 `json:"backup_read_bytes"`
	BackupWrite     int64 `json:"backup_write_bytes"`
	PrefixRead      int64 `json:"prefix_read_bytes"`
	PayloadSent     int64 `json:"payload_sent_bytes"`
	PayloadReceived int64 `json:"payload_received_bytes"`
	Reused          int64 `json:"reused_bytes"`
	Transfers       int64 `json:"transfer_attempts"`
	Failures        int64 `json:"transfer_failures"`
	TransferNanos   int64 `json:"transfer_nanoseconds"`
}

func (e *Engine) Metrics() MetricsSnapshot {
	m := &e.metrics
	return MetricsSnapshot{m.ScanRead.Load(), m.SourceRead.Load(), m.BasisRead.Load(), m.StageWrite.Load(), m.VerifyRead.Load(), m.BackupRead.Load(), m.BackupWrite.Load(), m.PrefixRead.Load(), m.PayloadSent.Load(), m.PayloadReceived.Load(), m.Reused.Load(), m.Transfers.Load(), m.Failures.Load(), m.TransferNanos.Load()}
}
func (s MetricsSnapshot) ReadBytes() int64 {
	return s.ScanRead + s.SourceRead + s.BasisRead + s.VerifyRead + s.BackupRead + s.PrefixRead
}
func (s MetricsSnapshot) WriteBytes() int64 { return s.StageWrite + s.BackupWrite }

// WriteMetrics exports a private, atomically replaced JSON snapshot in state.
func (e *Engine) WriteMetrics() error {
	data, err := json.MarshalIndent(e.Metrics(), "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(e.stateDir, "metrics-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(e.stateDir, "metrics.json"))
}

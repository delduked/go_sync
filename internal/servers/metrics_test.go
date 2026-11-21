package servers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetricsTrackActualTransfer(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	client := &measuredClient{FileSyncServiceClient: serve(t, a)}
	a.client = client
	data := bytes.Repeat([]byte("x"), 2*defaultBlockSize)
	write(t, a, "file", data)
	pull(t, b, a)
	first := a.e.Metrics()
	received := b.e.Metrics()
	if first.PayloadSent != client.bytes.Load() || first.PayloadSent != int64(len(data)) || received.PayloadReceived != first.PayloadSent {
		t.Fatalf("payload mismatch: %+v %+v", first, received)
	}
	data[13] = 'y'
	write(t, a, "file", data)
	pull(t, b, a)
	next := a.e.Metrics()
	if next.PayloadSent-first.PayloadSent != defaultBlockSize || next.Reused-first.Reused != defaultBlockSize {
		t.Fatalf("delta counters: %+v", next)
	}
	if received.VerifyRead != int64(len(data)) || received.Transfers != 1 || received.Failures != 0 || received.TransferNanos <= 0 {
		t.Fatalf("receiver metrics: %+v", received)
	}
	if err := b.e.WriteMetrics(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(b.state, "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exported MetricsSnapshot
	if err = json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	if exported != b.e.Metrics() {
		t.Fatal("export mismatch")
	}
}

// One iteration is the reproducible cost baseline. Setup is excluded; measured
// work includes detecting the edit, transfer, validation and destination commit.
func BenchmarkTransferWorkloads(b *testing.B) {
	for _, workload := range []string{"append", "overwrite", "truncate"} {
		b.Run(workload, func(b *testing.B) {
			b.StopTimer()
			a, dest := newFixture(b), newFixture(b)
			serve(b, a)
			base := bytes.Repeat([]byte("a"), 2*1024*1024)
			var sent, received, reused, reads, writes, nanos int64
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				write(b, a, "file", base)
				pull(b, dest, a)
				beforeA, beforeB := a.e.Metrics(), dest.e.Metrics()
				var changed []byte
				switch workload {
				case "append":
					changed = append(append([]byte{}, base...), bytes.Repeat([]byte("b"), 1024)...)
				case "overwrite":
					changed = append([]byte{}, base...)
					changed[70000] = 'b'
				case "truncate":
					changed = base[:len(base)/2]
				}
				b.StartTimer()
				start := time.Now()
				write(b, a, "file", changed)
				pull(b, dest, a)
				nanos += time.Since(start).Nanoseconds()
				b.StopTimer()
				equalFile(b, dest, "file", changed)
				afterA, afterB := a.e.Metrics(), dest.e.Metrics()
				sent += afterA.PayloadSent - beforeA.PayloadSent
				received += afterB.PayloadReceived - beforeB.PayloadReceived
				reused += afterA.Reused - beforeA.Reused
				reads += afterA.ReadBytes() + afterB.ReadBytes() - beforeA.ReadBytes() - beforeB.ReadBytes()
				writes += afterA.WriteBytes() + afterB.WriteBytes() - beforeA.WriteBytes() - beforeB.WriteBytes()
			}
			n := float64(b.N)
			b.ReportMetric(float64(sent)/n, "payload-B/op")
			b.ReportMetric(float64(received)/n, "received-B/op")
			b.ReportMetric(float64(reused)/n, "reused-B/op")
			b.ReportMetric(float64(reads)/n, "read-B/op")
			b.ReportMetric(float64(writes)/n, "write-B/op")
			b.ReportMetric(float64(nanos)/n/1e6, "converge-ms/op")
		})
	}
}

package servers

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentSourcesDoNotDuplicatePayload(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	serve(t, a)
	data := bytes.Repeat([]byte("a"), 1<<20)
	write(t, a, "file", data)
	manifest, err := a.e.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 9)
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- b.e.pull(context.Background(), a.client, manifest.Files[0], nil) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	equalFile(t, b, "file", data)
	if sent := a.e.Metrics().PayloadSent; sent != int64(len(data)) {
		t.Fatalf("sent %d bytes for one %d-byte revision", sent, len(data))
	}
}
func TestTransferQuantumYieldsAndResumes(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	if err := WithTransferLimits(1, 0, 128<<10)(a.e); err != nil {
		t.Fatal(err)
	}
	serve(t, a)
	data := bytes.Repeat([]byte("q"), 1<<20)
	write(t, a, "large", data)
	if err := b.e.Reconcile(context.Background(), a.client); err == nil {
		t.Fatal("large transfer did not yield")
	}
	write(t, a, "small", []byte("interactive edit"))
	for i := 0; i < 10; i++ {
		before := a.e.Metrics().PayloadSent
		err := b.e.Reconcile(context.Background(), a.client)
		equalFile(t, b, "small", []byte("interactive edit"))
		if sent := a.e.Metrics().PayloadSent - before; sent > (128<<10)+32 {
			t.Fatalf("quantum exceeded: %d", sent)
		}
		if err == nil {
			equalFile(t, b, "large", data)
			return
		}
	}
	t.Fatal("quantized transfer failed to finish")
}
func TestUploadBandwidthLimitAndCancellation(t *testing.T) {
	a, b := newFixture(t), newFixture(t)
	if err := WithTransferLimits(1, 128<<10, 0)(a.e); err != nil {
		t.Fatal(err)
	}
	serve(t, a)
	data := bytes.Repeat([]byte("x"), 256<<10)
	write(t, a, "file", data)
	started := time.Now()
	pull(t, b, a)
	if elapsed := time.Since(started); elapsed < 1400*time.Millisecond {
		t.Fatalf("rate limit completed too quickly: %s", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.e.limits.waitBandwidth(ctx, 65536); err == nil {
		t.Fatal("ignored bandwidth cancellation")
	}
	slots := make(chan struct{}, 1)
	release, err := acquire(context.Background(), slots)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = acquire(ctx, slots); err == nil {
		t.Fatal("ignored queue cancellation")
	}
}

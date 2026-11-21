package servers

import (
	"context"
	"fmt"
	"time"
)

type transferLimits struct {
	send, receive           chan struct{}
	rateGate                chan struct{}
	next                    time.Time
	bytesPerSecond, quantum int64
}

func newTransferLimits(concurrent int, rate, quantum int64) *transferLimits {
	return &transferLimits{send: make(chan struct{}, concurrent), receive: make(chan struct{}, concurrent), rateGate: make(chan struct{}, 1), bytesPerSecond: rate, quantum: quantum}
}
func WithTransferLimits(concurrent int, uploadBytesPerSecond, quantum int64) Option {
	return func(e *Engine) error {
		if concurrent < 1 || concurrent > 64 || uploadBytesPerSecond < 0 || quantum < 0 || quantum > 0 && quantum < defaultBlockSize {
			return fmt.Errorf("transfers must be 1–64, upload limit nonnegative, quantum zero or at least 65536 bytes")
		}
		e.limits = newTransferLimits(concurrent, uploadBytesPerSecond, quantum)
		return nil
	}
}
func acquire(ctx context.Context, slots chan struct{}) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (l *transferLimits) waitBandwidth(ctx context.Context, n int64) error {
	if l.bytesPerSecond == 0 {
		return ctx.Err()
	}
	release, err := acquire(ctx, l.rateGate)
	if err != nil {
		return err
	}
	defer release()
	if delay := time.Until(l.next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.next = time.Now().Add(time.Duration(float64(n) / float64(l.bytesPerSecond) * float64(time.Second)))
	return nil
}
func (l *transferLimits) payloadQuantum(chunkSize int64) int64 {
	quantum := l.quantum
	if quantum > 0 && l.bytesPerSecond > 0 && l.bytesPerSecond < quantum/2 {
		quantum = max(chunkSize, 2*l.bytesPerSecond)
	}
	return quantum
}

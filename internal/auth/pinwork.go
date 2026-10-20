package auth

import (
	"context"
	"io"
	"runtime"
	"sync/atomic"
)

// pinWork runs every scrypt call. Each run takes 16 MiB (128 * N * r), so at most one per
// CPU runs at a time and a flood of sign-ins queues instead of exhausting memory.
type pinWork struct {
	keys  Keys
	slots chan struct{}
	runs  atomic.Int64
}

func newPINWork(keys Keys) *pinWork {
	return &pinWork{keys: keys, slots: make(chan struct{}, runtime.NumCPU())}
}

func (w *pinWork) hash(ctx context.Context, pin PIN, usernameDigest []byte, random io.Reader) (PINHash, error) {
	if err := w.acquire(ctx); err != nil {
		return PINHash{}, err
	}
	defer w.release()

	return w.keys.HashPIN(pin, usernameDigest, random)
}

func (w *pinWork) verify(ctx context.Context, pin PIN, usernameDigest []byte, stored PINHash) (bool, error) {
	if err := w.acquire(ctx); err != nil {
		return false, err
	}
	defer w.release()

	return w.keys.VerifyPIN(pin, usernameDigest, stored), nil
}

func (w *pinWork) acquire(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrBusy
	}

	select {
	case w.slots <- struct{}{}:
		w.runs.Add(1)

		return nil
	case <-ctx.Done():
		return ErrBusy
	}
}

func (w *pinWork) release() {
	<-w.slots
}

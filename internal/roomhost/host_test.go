package roomhost

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/akkamaze/tutti/internal/room"
)

type loadingStore struct {
	fakeStore
	loads   int
	loadErr error
}

func (s *loadingStore) Load(_ context.Context, _ room.ID) (room.State, Rev, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loads++
	if s.loadErr != nil {
		return room.State{}, 0, s.loadErr
	}

	return room.NewState(owner), 1, nil
}

func (s *loadingStore) loadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loads
}

func newTestHost(t *testing.T, store *loadingStore) *Host {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return NewHost(ctx, HostConfig{Store: store, Queues: &fakeQueues{}, Clock: newFakeClock(t0)})
}

func TestHostStartsARoomOnceAndReusesIt(t *testing.T) {
	store := &loadingStore{}
	h := newTestHost(t, store)

	var wg sync.WaitGroup
	rooms := make([]*Room, 20)

	for i := range rooms {
		wg.Go(func() {
			r, err := h.Room(context.Background(), 7)
			if err != nil {
				t.Errorf("room: %v", err)
			}
			rooms[i] = r
		})
	}
	wg.Wait()

	for _, r := range rooms[1:] {
		if r != rooms[0] {
			t.Fatal("concurrent lookups started more than one actor for the same room")
		}
	}

	if n := store.loadCount(); n != 1 {
		t.Fatalf("loads = %d, want 1", n)
	}
}

func TestHostDoesNotKeepAFailedLoad(t *testing.T) {
	store := &loadingStore{loadErr: errors.New("db down")}
	h := newTestHost(t, store)

	if _, err := h.Room(context.Background(), 7); err == nil {
		t.Fatal("room loaded from a failing store, want an error")
	}

	store.mu.Lock()
	store.loadErr = nil
	store.mu.Unlock()

	if _, err := h.Room(context.Background(), 7); err != nil {
		t.Fatalf("room after the store recovered: %v", err)
	}
}

func TestHostReloadsARoomThatStopped(t *testing.T) {
	store := &loadingStore{}
	h := newTestHost(t, store)

	first, err := h.Room(context.Background(), 7)
	if err != nil {
		t.Fatalf("room: %v", err)
	}

	store.mu.Lock()
	store.fail = ErrFenced
	store.mu.Unlock()

	_ = first.Do(context.Background(), room.Admit{Who: ann})
	<-first.done

	store.mu.Lock()
	store.fail = nil
	store.mu.Unlock()

	second, err := h.Room(context.Background(), 7)
	if err != nil {
		t.Fatalf("room after it stopped: %v", err)
	}

	if second == first || store.loadCount() != 2 {
		t.Fatalf("got the stopped room back (loads %d), want a fresh load", store.loadCount())
	}
}

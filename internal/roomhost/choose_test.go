package roomhost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/akkamaze/tutti/internal/account"
	"github.com/akkamaze/tutti/internal/room"
)

const bob account.ID = 3

type fakeQueues struct {
	mu     sync.Mutex
	heads  map[account.ID]room.Track
	fail   error
	lookup int
}

func (q *fakeQueues) Head(_ context.Context, dj account.ID) (room.Track, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.lookup++

	if q.fail != nil {
		return room.Track{}, false, q.fail
	}

	track, ok := q.heads[dj]

	return track, ok, nil
}

func (q *fakeQueues) heal() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.fail = nil
}

var annSong = room.Track{Item: 21, Video: "bbbbbbbbbbb", Title: "Ann's", Duration: 180 * time.Second}

func startChoosingRoom(t *testing.T, queues Queues, clock *fakeClock) (*Room, channelSink) {
	t.Helper()

	s := room.NewState(owner)
	for _, id := range []account.ID{ann, bob} {
		s.Roles[id] = room.RoleMember
		s.Presence[id] = room.Here{Conns: 1}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	r := Start(ctx, Config{ID: 7, State: s, Rev: 1, Store: &fakeStore{}, Clock: clock, Queues: queues})
	updates := make(channelSink, 16)

	if _, _, err := r.Attach(ctx, updates); err != nil {
		t.Fatalf("attach: %v", err)
	}

	return r, updates
}

func awaitPhase[P room.Phase](t *testing.T, updates channelSink) P {
	t.Helper()

	for {
		u := expectUpdate(t, updates)
		if p, ok := u.State.Phase.(P); ok {
			return p
		}
	}
}

func TestSteppingUpPlaysTheFirstSongInTheQueue(t *testing.T) {
	queues := &fakeQueues{heads: map[account.ID]room.Track{ann: annSong}}
	r, updates := startChoosingRoom(t, queues, newFakeClock(t0))

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("step up: %v", err)
	}

	p := awaitPhase[room.Playing](t, updates)
	if p.Play.DJ != ann || p.Play.Track != annSong {
		t.Fatalf("play = %+v, want ann's first song", p.Play)
	}
}

func TestEveryoneWithAnEmptyQueueLeavesTheRoomIdle(t *testing.T) {
	queues := &fakeQueues{heads: map[account.ID]room.Track{}}
	r, updates := startChoosingRoom(t, queues, newFakeClock(t0))

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("step up: %v", err)
	}

	awaitPhase[room.Idle](t, updates)
}

func TestAFailedQueueLookupIsRetried(t *testing.T) {
	clock := newFakeClock(t0)
	queues := &fakeQueues{heads: map[account.ID]room.Track{ann: annSong}, fail: errors.New("db down")}
	r, updates := startChoosingRoom(t, queues, clock)

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("step up: %v", err)
	}

	awaitPhase[room.Choosing](t, updates)
	expectQuiet(t, updates)

	queues.heal()
	clock.awaitWaiter(t)
	clock.Advance(time.Second)

	awaitPhase[room.Playing](t, updates)
}

func TestAnEmptyQueueIsPassedForTheNextDJsSong(t *testing.T) {
	bobSong := room.Track{Item: 31, Video: "ccccccccccc", Title: "Bob's", Duration: time.Minute}
	queues := &fakeQueues{heads: map[account.ID]room.Track{bob: bobSong}}

	s := room.NewState(owner)
	for _, id := range []account.ID{ann, bob} {
		s.Roles[id] = room.RoleMember
		s.Presence[id] = room.Here{Conns: 1}
		s.Booth = s.Booth.StepUp(id)
	}
	s.Phase = room.Choosing{DJ: ann}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &fakeStore{}
	Start(ctx, Config{ID: 7, State: s, Rev: 1, Store: store, Queues: queues, Clock: newFakeClock(t0)})

	deadline := time.Now().Add(2 * time.Second)
	for {
		commits := store.committed()
		if n := len(commits); n > 0 {
			if p, ok := commits[n-1].next.Phase.(room.Playing); ok && p.Play.DJ == bob {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("commits = %+v, want bob playing after ann's empty queue was passed", commits)
		}

		time.Sleep(time.Millisecond)
	}
}

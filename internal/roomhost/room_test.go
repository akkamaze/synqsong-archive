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

const (
	owner account.ID = 1
	ann   account.ID = 2
)

var t0 = time.UnixMilli(1_790_000_000_000)

type commit struct {
	rev   Rev
	next  room.State
	facts []room.Fact
}

type fakeStore struct {
	mu      sync.Mutex
	commits []commit
	fail    error
}

func (f *fakeStore) Load(_ context.Context, _ room.ID) (room.State, Rev, error) {
	return room.NewState(owner), 1, nil
}

func (f *fakeStore) Commit(_ context.Context, _ room.ID, prev Rev, next room.State, facts []room.Fact) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fail != nil {
		return f.fail
	}

	f.commits = append(f.commits, commit{rev: prev, next: next, facts: facts})

	return nil
}

func (f *fakeStore) committed() []commit {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]commit(nil), f.commits...)
}

type recordingSink struct {
	mu      sync.Mutex
	updates []Update
}

func (r *recordingSink) Send(u Update) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.updates = append(r.updates, u)

	return true
}

func (r *recordingSink) received() []Update {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Update(nil), r.updates...)
}

func startRoom(t *testing.T, store Store) *Room {
	t.Helper()

	state := room.NewState(owner)
	state.Roles[ann] = room.RoleMember

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return Start(ctx, Config{ID: 7, State: state, Rev: 1, Store: store, Queues: &fakeQueues{}, Clock: newFakeClock(t0)})
}

func TestDoCommitsThenBroadcasts(t *testing.T) {
	store := &fakeStore{}
	r := startRoom(t, store)
	sink := &recordingSink{}

	snapshot, detach, err := r.Attach(context.Background(), sink)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer detach()

	if snapshot.Rev != 1 {
		t.Fatalf("attach snapshot rev = %d, want 1", snapshot.Rev)
	}

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("step up: %v", err)
	}

	commits := store.committed()
	if len(commits) == 0 || commits[0].rev != 1 {
		t.Fatalf("commits = %+v, want the step up committed first, fenced on rev 1", commits)
	}

	updates := sink.received()
	if len(updates) == 0 || updates[0].Rev != 2 || !updates[0].State.Booth.Has(ann) {
		t.Fatalf("updates = %+v, want the first broadcast at rev 2 with ann in the booth", updates)
	}
}

func TestRefusalIsReturnedAndNothingIsWritten(t *testing.T) {
	store := &fakeStore{}
	r := startRoom(t, store)

	err := r.Do(context.Background(), room.Request{By: 999, Cmd: room.StepUp{}})
	if !errors.Is(err, room.ErrNotMember) {
		t.Fatalf("stranger step up: %v, want room.ErrNotMember", err)
	}

	if n := len(store.committed()); n != 0 {
		t.Fatalf("refused command wrote %d commits, want none", n)
	}
}

func TestAnInputThatChangesNothingIsNotWritten(t *testing.T) {
	store := &fakeStore{}
	r := startRoom(t, store)

	if err := r.Do(context.Background(), room.Tick{}); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if n := len(store.committed()); n != 0 {
		t.Fatalf("no-op tick wrote %d commits, want none", n)
	}
}

func TestAFailedCommitKeepsTheOldState(t *testing.T) {
	store := &fakeStore{fail: errors.New("disk full")}
	r := startRoom(t, store)
	sink := &recordingSink{}

	_, detach, err := r.Attach(context.Background(), sink)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer detach()

	err = r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("step up with a failing store: %v, want ErrUnavailable", err)
	}

	if n := len(sink.received()); n != 0 {
		t.Fatalf("failed commit broadcast %d updates, want none", n)
	}

	store.mu.Lock()
	store.fail = nil
	store.mu.Unlock()

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if got := store.committed(); len(got) == 0 || got[0].rev != 1 {
		t.Fatalf("retry commits = %+v, want the retry still fenced on rev 1", got)
	}
}

func TestAFencedRoomStops(t *testing.T) {
	store := &fakeStore{fail: ErrFenced}
	r := startRoom(t, store)

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); !errors.Is(err, ErrFenced) {
		t.Fatalf("first command after losing the room: %v, want ErrFenced", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := r.Do(ctx, room.Tick{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("command to a fenced room: %v, want ErrClosed", err)
	}
}

func TestDetachStopsUpdates(t *testing.T) {
	r := startRoom(t, &fakeStore{})
	sink := &recordingSink{}

	_, detach, err := r.Attach(context.Background(), sink)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	detach()

	if err := r.Do(context.Background(), room.Request{By: ann, Cmd: room.StepUp{}}); err != nil {
		t.Fatalf("step up: %v", err)
	}

	if n := len(sink.received()); n != 0 {
		t.Fatalf("detached sink got %d updates, want none", n)
	}
}

func TestConcurrentInputsAreAppliedOneAtATime(t *testing.T) {
	store := &fakeStore{}
	state := room.NewState(owner)

	const people = 30
	for i := range people {
		state.Roles[account.ID(100+i)] = room.RoleMember
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := Start(ctx, Config{ID: 7, State: state, Rev: 1, Store: store, Queues: &fakeQueues{}, Clock: newFakeClock(t0)})

	var wg sync.WaitGroup
	for i := range people {
		wg.Go(func() {
			if err := r.Do(ctx, room.Request{By: account.ID(100 + i), Cmd: room.Say{Body: "hi"}}); err != nil {
				t.Errorf("say by %d: %v", 100+i, err)
			}
		})
	}
	wg.Wait()

	commits := store.committed()
	if len(commits) != people {
		t.Fatalf("commits = %d, want %d", len(commits), people)
	}

	for i, c := range commits {
		if c.rev != Rev(1+i) {
			t.Fatalf("commit %d built on rev %d, want %d: every change must build on the one before", i, c.rev, 1+i)
		}
	}
}

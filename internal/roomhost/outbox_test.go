package roomhost

import (
	"context"
	"testing"
	"time"

	"synqsong/internal/account"
	"synqsong/internal/room"
)

func TestOutboxDropsItselfWhenFull(t *testing.T) {
	o := NewOutbox(2)

	if !o.Send(Update{Rev: 1}) || !o.Send(Update{Rev: 2}) {
		t.Fatal("outbox refused updates while it had room")
	}

	if o.Send(Update{Rev: 3}) {
		t.Fatal("full outbox accepted an update, want it to give up")
	}

	select {
	case <-o.Dropped():
	default:
		t.Fatal("full outbox did not signal Dropped")
	}

	if got := (<-o.Updates()).Rev; got != 1 {
		t.Fatalf("first queued update rev = %d, want 1", got)
	}

	if o.Send(Update{Rev: 4}) {
		t.Fatal("dropped outbox accepted an update once it had room again, want it to stay dropped")
	}
}

func TestASlowConnectionIsCutWithoutStallingTheRoom(t *testing.T) {
	state := room.NewState(owner)
	for i := range 10 {
		state.Roles[account.ID(100+i)] = room.RoleMember
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := Start(ctx, Config{ID: 7, State: state, Rev: 1, Store: &fakeStore{}, Queues: &fakeQueues{}, Clock: newFakeClock(t0)})

	slow := NewOutbox(2)
	if _, _, err := r.Attach(ctx, slow); err != nil {
		t.Fatalf("attach slow: %v", err)
	}

	fast := &recordingSink{}
	if _, _, err := r.Attach(ctx, fast); err != nil {
		t.Fatalf("attach fast: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		for i := range 10 {
			if err := r.Do(ctx, room.Request{By: account.ID(100 + i), Cmd: room.Say{Body: "hi"}}); err != nil {
				t.Errorf("say: %v", err)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the room stalled behind a connection nobody was reading")
	}

	select {
	case <-slow.Dropped():
	default:
		t.Fatal("the slow connection was never dropped")
	}

	if n := len(fast.received()); n != 10 {
		t.Fatalf("the fast connection got %d updates, want all 10", n)
	}

	if n := len(slow.Updates()); n != 2 {
		t.Fatalf("the slow connection holds %d updates, want only the 2 that fit before it was cut", n)
	}
}

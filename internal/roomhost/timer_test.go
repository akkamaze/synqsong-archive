package roomhost

import (
	"context"
	"testing"
	"time"

	"synqsong/internal/room"
)

// channelSink hands each update to the test as it arrives.
type channelSink chan Update

func (c channelSink) Send(u Update) bool {
	c <- u

	return true
}

func expectUpdate(t *testing.T, updates channelSink) Update {
	t.Helper()

	select {
	case u := <-updates:
		return u

	case <-time.After(2 * time.Second):
		t.Fatal("no update arrived")

		return Update{}
	}
}

func expectQuiet(t *testing.T, updates channelSink) {
	t.Helper()

	select {
	case u := <-updates:
		t.Fatalf("unexpected update at rev %d", u.Rev)

	case <-time.After(50 * time.Millisecond):
	}
}

func playingRoom(length time.Duration) room.State {
	s := room.NewState(owner)
	s.Roles[ann] = room.RoleMember
	s.Presence[ann] = room.Here{Conns: 1}
	s.Booth = s.Booth.StepUp(ann)
	s.Phase = room.Playing{Play: room.Play{
		Token: room.PlayToken(t0.UnixMilli()),
		Track: room.Track{Item: 1, Video: "aaaaaaaaaaa", Duration: length},
		DJ:    ann,
	}}

	return s
}

func startWithClock(t *testing.T, state room.State, clock *fakeClock) (*Room, channelSink) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	r := Start(ctx, Config{ID: 7, State: state, Rev: 1, Store: &fakeStore{}, Queues: &fakeQueues{}, Clock: clock})
	updates := make(channelSink, 8)

	if _, _, err := r.Attach(ctx, updates); err != nil {
		t.Fatalf("attach: %v", err)
	}

	return r, updates
}

func TestTheFallbackEndsASongWithNobodyReporting(t *testing.T) {
	clock := newFakeClock(t0)
	_, updates := startWithClock(t, playingRoom(200*time.Second), clock)

	clock.Advance(202*time.Second - time.Millisecond)
	expectQuiet(t, updates)

	clock.Advance(time.Millisecond)

	u := expectUpdate(t, updates)
	if len(u.Facts) != 1 {
		t.Fatalf("facts at the fallback = %#v, want the song finished", u.Facts)
	}

	if _, ok := u.Facts[0].(room.Finished); !ok {
		t.Fatalf("fact = %#v, want room.Finished", u.Facts[0])
	}
}

func TestAnInputThatKeepsTheDeadlineKeepsTheTimer(t *testing.T) {
	clock := newFakeClock(t0)
	r, _ := startWithClock(t, playingRoom(200*time.Second), clock)

	// The round trip of a second attach makes sure the room is done with the first.
	if _, _, err := r.Attach(context.Background(), make(channelSink, 8)); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if n := clock.timersArmed(); n != 1 {
		t.Fatalf("timers armed = %d, want 1: an input that keeps the deadline must not replace the timer", n)
	}
}

func TestAnOverdueDeadlineWaitsOneSecond(t *testing.T) {
	clock := newFakeClock(t0.Add(time.Hour))
	_, updates := startWithClock(t, playingRoom(time.Second), clock)

	clock.Advance(time.Second - time.Millisecond)
	expectQuiet(t, updates)

	clock.Advance(time.Millisecond)
	expectUpdate(t, updates)
}

func TestARestoredRoomWithNobodyConnectedStillMovesOn(t *testing.T) {
	clock := newFakeClock(t0)
	store := &fakeStore{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Start(ctx, Config{ID: 7, State: playingRoom(200 * time.Second), Rev: 1, Store: store, Queues: &fakeQueues{}, Clock: clock})

	clock.awaitWaiter(t)
	clock.Advance(202 * time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for len(store.committed()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the fallback never fired in a room nobody attached to")
		}

		time.Sleep(time.Millisecond)
	}
}

func TestAFallbackThatFailsToSaveIsRetried(t *testing.T) {
	clock := newFakeClock(t0)
	store := &fakeStore{fail: ErrUnavailable}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Start(ctx, Config{ID: 7, State: playingRoom(200 * time.Second), Rev: 1, Store: store, Queues: &fakeQueues{}, Clock: clock})

	// The failed save keeps the state, so the deadline stays the same; the room must still wake again.
	clock.awaitWaiter(t)
	armed := clock.timersArmed()
	clock.Advance(202 * time.Second)

	waitFor(t, "the room to arm a retry", func() bool { return clock.timersArmed() > armed })

	store.mu.Lock()
	store.fail = nil
	store.mu.Unlock()

	clock.Advance(time.Second)

	waitFor(t, "the retried fallback to be saved", func() bool { return len(store.committed()) > 0 })
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited 2s for %s", what)
		}

		time.Sleep(time.Millisecond)
	}
}

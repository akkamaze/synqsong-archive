package roomhost

import (
	"context"
	"testing"
	"time"

	"github.com/akkamaze/tutti/internal/room"
)

// channelSink hands each update to the test as it arrives.
type channelSink chan Update

func (c channelSink) Send(u Update) { c <- u }

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

func startWithClock(t *testing.T, state room.State, clock *fakeClock) channelSink {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	r := Start(ctx, Config{ID: 7, State: state, Rev: 1, Store: &fakeStore{}, Clock: clock})
	updates := make(channelSink, 8)

	if _, _, err := r.Attach(ctx, updates); err != nil {
		t.Fatalf("attach: %v", err)
	}

	return updates
}

func TestTheFallbackEndsASongWithNobodyReporting(t *testing.T) {
	clock := newFakeClock(t0)
	updates := startWithClock(t, playingRoom(200*time.Second), clock)

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

func TestAnOverdueDeadlineWaitsOneSecond(t *testing.T) {
	clock := newFakeClock(t0.Add(time.Hour))
	updates := startWithClock(t, playingRoom(time.Second), clock)

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

	Start(ctx, Config{ID: 7, State: playingRoom(200 * time.Second), Rev: 1, Store: store, Clock: clock})

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

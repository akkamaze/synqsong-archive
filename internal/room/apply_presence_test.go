package room

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/akkamaze/tutti/internal/account"
)

func presenceOf(t *testing.T, s State, who account.ID) Presence {
	t.Helper()

	return s.Presence[who]
}

func TestConnectionsAreCounted(t *testing.T) {
	s := roomWith()
	s, _ = must(t, s, Connected{Who: cat})

	if got := presenceOf(t, s, cat); got != (Here{Conns: 2}) {
		t.Fatalf("second tab: presence %#v, want Here{2}", got)
	}

	s, _ = must(t, s, Disconnected{Who: cat})
	if got := presenceOf(t, s, cat); got != (Here{Conns: 1}) {
		t.Fatalf("one tab closed: presence %#v, want Here{1}", got)
	}

	s, _ = must(t, s, Disconnected{Who: cat})
	if got := presenceOf(t, s, cat); got != (Away{Since: t0}) {
		t.Fatalf("last tab went quiet: presence %#v, want Away since now", got)
	}
}

func TestStrangersCannotConnect(t *testing.T) {
	if _, _, err := Apply(roomWith(), Connected{Who: 12345}, t0); !errors.Is(err, ErrNotMember) {
		t.Fatalf("connect by a non-member: %v, want ErrNotMember", err)
	}
}

func TestAListenerWhoSaysGoodbyeLeaves(t *testing.T) {
	s, _ := must(t, roomWith(), Disconnected{Who: cat, Goodbye: true})

	if _, ok := s.Presence[cat]; ok {
		t.Fatal("listener who said goodbye is still in the room")
	}
}

func TestAnOnDeckDJWhoLeavesSleepsInTheirSlot(t *testing.T) {
	s, _ := annPlaying()

	s, _ = must(t, s, Disconnected{Who: bob, Goodbye: true})

	assertOrder(t, s.Booth, ann, bob)

	if got := presenceOf(t, s, bob); got != (Asleep{Since: t0}) {
		t.Fatalf("on-deck DJ who left: presence %#v, want asleep in their slot", got)
	}
}

func TestAnOnDeckDJWhoSignsOutLosesTheirSlot(t *testing.T) {
	s, _ := annPlaying()

	s, _ = must(t, s, Disconnected{Who: bob, Goodbye: true, SignOut: true})

	assertOrder(t, s.Booth, ann)

	if _, ok := s.Presence[bob]; ok {
		t.Fatal("signed-out DJ is still in the room")
	}
}

func TestThePlayingDJWhoLeavesFinishesTheSongThenDrops(t *testing.T) {
	s, play := annPlaying()

	s, _ = must(t, s, Disconnected{Who: ann, Goodbye: true, SignOut: true})

	if playing(t, s).Play != play {
		t.Fatal("the playing DJ's song stopped when they left")
	}

	s, _, err := Apply(s, Request{By: cat, Cmd: Ended{Token: play.Token, Video: play.Track.Video}}, t0.Add(200*time.Second))
	if err != nil {
		t.Fatalf("ended: %v", err)
	}

	assertOrder(t, s.Booth, bob)

	if _, ok := s.Presence[ann]; ok {
		t.Fatal("asleep DJ stayed in the room after their slot was gone")
	}
}

func TestAnAsleepDJsTurnBecomesARest(t *testing.T) {
	s, play := annPlaying()
	s, _ = must(t, s, Disconnected{Who: bob, Goodbye: true})

	s, _ = must(t, s, Request{By: ann, Cmd: Skip{Token: play.Token}})

	rest, ok := s.Phase.(Resting)
	if !ok || rest.DJ != bob || rest.Reason != RestOffline {
		t.Fatalf("phase = %#v, want bob resting offline when his turn came", s.Phase)
	}
}

func TestAnAsleepDJWhoReturnsDuringTheirRestPlays(t *testing.T) {
	s, play := annPlaying()
	s, _ = must(t, s, Disconnected{Who: bob, Goodbye: true})
	s, _ = must(t, s, Request{By: ann, Cmd: Skip{Token: play.Token}})

	s, _ = must(t, s, Connected{Who: bob})

	choosing(t, s, bob)

	if got := presenceOf(t, s, bob); got != (Here{Conns: 1}) {
		t.Fatalf("returning DJ presence %#v, want Here{1}", got)
	}
}

func TestADJWhoFallsAsleepWhileChoosingRestsInstead(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Choosing{DJ: ann}

	s, _ = must(t, s, Disconnected{Who: ann, Goodbye: true})

	rest, ok := s.Phase.(Resting)
	if !ok || rest.DJ != ann || rest.Reason != RestOffline {
		t.Fatalf("phase = %#v, want ann resting offline", s.Phase)
	}
}

func TestListenersReportUnplayableWhenTheDJIsAsleep(t *testing.T) {
	s, play := annPlaying()
	s, _ = must(t, s, Disconnected{Who: ann, Goodbye: true})

	s, facts := must(t, s, Request{By: cat, Cmd: Unplayable{Token: play.Token, Video: play.Track.Video}})

	assertOrder(t, s.Booth, bob)
	choosing(t, s, bob)

	if want := []Fact{Finished{Play: play, Reason: EndUnplayable}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
}

func TestOnlyConnectedMembersCountTowardVotes(t *testing.T) {
	s, play := annPlaying()
	s, _ = must(t, s, Disconnected{Who: cat})
	s, _ = must(t, s, Disconnected{Who: owner, Goodbye: true})

	// Eligible now: bob only (cat is away, the owner left, ann is playing). 50% of 1 is 1.
	_, facts := must(t, s, Request{By: bob, Cmd: VoteSkip{Token: play.Token}})

	if want := []Fact{Finished{Play: play, Reason: EndVotedOut}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want the single eligible vote to skip", facts)
	}
}

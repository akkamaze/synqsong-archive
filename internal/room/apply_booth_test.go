package room

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"synqsong/internal/account"
)

const (
	owner account.ID = 100
	guest account.ID = 999
)

var song1 = Track{Item: 11, Video: "aaaaaaaaaaa", Title: "One", Duration: 200 * time.Second}

func roomWith(djs ...account.ID) State {
	s := NewState(owner)
	for _, id := range []account.ID{ann, bob, cat} {
		s.Roles[id] = RoleMember
	}
	s.Roles[guest] = RoleGuest

	s.Presence = map[account.ID]Presence{}
	for _, id := range []account.ID{owner, ann, bob, cat} {
		s.Presence[id] = Here{Conns: 1}
	}

	for _, dj := range djs {
		s.Booth = s.Booth.StepUp(dj)
	}

	return s
}

func must(t *testing.T, s State, in Input) (State, []Fact) {
	t.Helper()

	next, facts, err := Apply(s, in, t0)
	if err != nil {
		t.Fatalf("Apply(%T) refused: %v", in, err)
	}

	return next, facts
}

func playing(t *testing.T, s State) Playing {
	t.Helper()

	p, ok := s.Phase.(Playing)
	if !ok {
		t.Fatalf("phase = %#v, want Playing", s.Phase)
	}

	return p
}

func choosing(t *testing.T, s State, dj account.ID) {
	t.Helper()

	c, ok := s.Phase.(Choosing)
	if !ok || c.DJ != dj {
		t.Fatalf("phase = %#v, want Choosing for DJ %d", s.Phase, dj)
	}
}

func TestSteppingUpInAnIdleRoomLooksForTheirSong(t *testing.T) {
	s, _ := must(t, roomWith(), Request{By: ann, Cmd: StepUp{}})

	choosing(t, s, ann)

	s, _ = must(t, s, TrackChosen{DJ: ann, Track: song1, Found: true})

	p := playing(t, s)
	if p.Play.DJ != ann || p.Play.Track != song1 || p.Play.Token != PlayToken(t0.UnixMilli()) {
		t.Fatalf("play = %+v, want ann playing song1 from now", p.Play)
	}

	if s.LastToken != p.Play.Token {
		t.Fatalf("last token = %d, want the new play's %d", s.LastToken, p.Play.Token)
	}
}

func TestATrackForSomeoneElseIsIgnored(t *testing.T) {
	s, _ := must(t, roomWith(ann, bob), Request{By: ann, Cmd: StepUp{}})
	s.Phase = Choosing{DJ: ann}

	next, facts := must(t, s, TrackChosen{DJ: bob, Track: song1, Found: true})

	choosing(t, next, ann)

	if len(facts) != 0 {
		t.Fatalf("facts = %v, want none for a stale track", facts)
	}
}

func TestEmptyQueuesAreRotatedPastThenTheRoomIdles(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Choosing{DJ: ann}

	s, _ = must(t, s, TrackChosen{DJ: ann, Found: false})
	choosing(t, s, bob)

	s, _ = must(t, s, TrackChosen{DJ: bob, Found: false})

	if _, ok := s.Phase.(Idle); !ok {
		t.Fatalf("phase = %#v, want Idle once every DJ had nothing", s.Phase)
	}

	assertOrder(t, s.Booth, ann, bob)
}

func TestGuestsAndStrangersCannotStepUp(t *testing.T) {
	for _, by := range []account.ID{guest, 12345} {
		_, _, err := Apply(roomWith(), Request{By: by, Cmd: StepUp{}}, t0)
		if err == nil {
			t.Errorf("step up by %d was accepted, want refused", by)
		}
	}
}

func TestPlayingDJStepsDownAfterTheirSong(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Playing{Play: Play{Token: 5, Track: song1, DJ: ann}}

	s, facts := must(t, s, Request{By: ann, Cmd: StepDown{}})

	if playing(t, s).Play.DJ != ann || !s.Booth.HeadLeaving() || len(facts) != 0 {
		t.Fatalf("after step down mid-song: phase %#v, leaving %v, facts %v; want song to go on", s.Phase, s.Booth.HeadLeaving(), facts)
	}
}

func TestHeadLookingForASongStepsDownAtOnce(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Choosing{DJ: ann}

	s, _ = must(t, s, Request{By: ann, Cmd: StepDown{}})

	assertOrder(t, s.Booth, bob)
	choosing(t, s, bob)
}

func TestOwnerKicksThePlayingDJ(t *testing.T) {
	s := roomWith(ann, bob)
	play := Play{Token: 5, Track: song1, DJ: ann}
	s.Phase = Playing{Play: play}

	s, facts := must(t, s, Request{By: owner, Cmd: KickFromBooth{DJ: ann}})

	assertOrder(t, s.Booth, bob)
	choosing(t, s, bob)

	want := []Fact{Finished{Play: play, Reason: EndFinished}}
	if !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v (a removed DJ's song counts as finished)", facts, want)
	}
}

func TestOnlyTheOwnerKicks(t *testing.T) {
	s := roomWith(ann, bob)
	s.Roles[cat] = RoleDJ

	_, _, err := Apply(s, Request{By: cat, Cmd: KickFromBooth{DJ: ann}}, t0)
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("kick by a DJ: %v, want ErrNotAllowed", err)
	}
}

func TestKickingSomeoneNotInTheBoothChangesNothing(t *testing.T) {
	s := roomWith(ann)

	next, facts := must(t, s, Request{By: owner, Cmd: KickFromBooth{DJ: bob}})

	if !reflect.DeepEqual(next, s) || len(facts) != 0 {
		t.Fatal("kicking someone not in the booth changed the room")
	}
}

func TestASongThatAlreadyFailedThisTurnEndsTheTurn(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Choosing{DJ: ann, Failed: []QueueItemID{song1.Item}}

	s, _ = must(t, s, TrackChosen{DJ: ann, Track: song1, Found: true})

	rest, ok := s.Phase.(Resting)
	if !ok || rest.DJ != ann || rest.Reason != RestUnplayable {
		t.Fatalf("phase = %#v, want ann resting because every song failed", s.Phase)
	}
}

func TestApplyLeavesTheInputStateAlone(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Playing{Play: Play{Token: 5, Track: song1, DJ: ann}}
	before := roomWith(ann, bob)
	before.Phase = s.Phase

	_, _, _ = Apply(s, Request{By: owner, Cmd: KickFromBooth{DJ: ann}}, t0)
	_, _, _ = Apply(s, Request{By: cat, Cmd: StepUp{}}, t0)

	if !reflect.DeepEqual(s, before) {
		t.Fatal("Apply changed the state it was given")
	}
}

package room

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"synqsong/internal/account"
)

// annPlaying: ann plays song1, bob waits, cat listens. The play started at t0.
func annPlaying() (State, Play) {
	s := roomWith(ann, bob)
	play := Play{Token: PlayToken(t0.UnixMilli()), Track: song1, DJ: ann}
	s.Phase = Playing{Play: play}
	s.LastToken = play.Token

	return s, play
}

func applyAt(t *testing.T, s State, in Input, at time.Time) (State, []Fact, error) {
	t.Helper()

	return Apply(s, in, at)
}

func TestEndedRotatesTheDJAndRecordsTheSong(t *testing.T) {
	s, play := annPlaying()

	next, facts, err := applyAt(t, s, Request{By: cat, Cmd: Ended{Token: play.Token, Video: play.Track.Video}}, t0.Add(200*time.Second))
	if err != nil {
		t.Fatalf("ended: %v", err)
	}

	assertOrder(t, next.Booth, bob, ann)
	choosing(t, next, bob)

	if want := []Fact{Finished{Play: play, Reason: EndFinished}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
}

func TestSecondEndedForTheSamePlayIsStale(t *testing.T) {
	s, play := annPlaying()
	end := Request{By: cat, Cmd: Ended{Token: play.Token, Video: play.Track.Video}}

	s, _, _ = applyAt(t, s, end, t0.Add(200*time.Second))
	s, _ = must(t, s, TrackChosen{DJ: bob, Track: song1, Found: true})

	_, _, err := applyAt(t, s, end, t0.Add(201*time.Second))
	if !errors.Is(err, ErrStalePlay) {
		t.Fatalf("duplicate ended: %v, want ErrStalePlay so it cannot end bob's new play", err)
	}
}

func TestEndedTooEarlyIsRefused(t *testing.T) {
	s, play := annPlaying()

	_, _, err := applyAt(t, s, Request{By: cat, Cmd: Ended{Token: play.Token, Video: play.Track.Video}}, t0.Add(10*time.Second))
	if !errors.Is(err, ErrEndedTooEarly) {
		t.Fatalf("ended 10s into a 200s song: %v, want ErrEndedTooEarly", err)
	}
}

func TestGuestsCannotDrivePlayback(t *testing.T) {
	s, play := annPlaying()

	for _, cmd := range []Command{
		Ended{Token: play.Token, Video: play.Track.Video},
		Loaded{Token: play.Token, Video: play.Track.Video, Duration: time.Minute},
		VoteSkip{Token: play.Token},
	} {
		if _, _, err := applyAt(t, s, Request{By: guest, Cmd: cmd}, t0.Add(300*time.Second)); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("guest %T: %v, want ErrNotAllowed", cmd, err)
		}
	}
}

func TestLoadedTeachesTheRoomTheLength(t *testing.T) {
	s := roomWith(ann)
	unknown := Track{Item: 12, Video: "bbbbbbbbbbb"}
	play := Play{Token: 9, Track: unknown, DJ: ann}
	s.Phase = Playing{Play: play}

	next, facts := must(t, s, Request{By: cat, Cmd: Loaded{Token: 9, Video: unknown.Video, Duration: 180 * time.Second}})

	if got := playing(t, next).Play.Track.Duration; got != 180*time.Second {
		t.Fatalf("duration = %v, want 180s from the report", got)
	}

	if want := []Fact{LengthLearned{Item: 12, Duration: 180 * time.Second}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v so the next play knows it", facts, want)
	}

	_, again := must(t, next, Request{By: bob, Cmd: Loaded{Token: 9, Video: unknown.Video, Duration: 180 * time.Second}})
	if len(again) != 0 {
		t.Fatalf("same length reported twice gave facts %#v, want none", again)
	}
}

func TestLoadedWithoutALengthIsRefused(t *testing.T) {
	s, play := annPlaying()

	if _, _, err := applyAt(t, s, Request{By: cat, Cmd: Loaded{Token: play.Token, Video: play.Track.Video}}, t0); err == nil {
		t.Fatal("loaded with zero length was accepted")
	}
}

func TestUnplayableKeepsTheDJsTurn(t *testing.T) {
	s, play := annPlaying()

	next, facts := must(t, s, Request{By: ann, Cmd: Unplayable{Token: play.Token, Video: play.Track.Video}})

	assertOrder(t, next.Booth, ann, bob)

	c, ok := next.Phase.(Choosing)
	if !ok || c.DJ != ann || !reflect.DeepEqual(c.Failed, []QueueItemID{song1.Item}) {
		t.Fatalf("phase = %#v, want ann choosing again with song1 marked failed", next.Phase)
	}

	if want := []Fact{Finished{Play: play, Reason: EndUnplayable}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
}

func TestOnlyThePlayingDJReportsUnplayable(t *testing.T) {
	s, play := annPlaying()

	if _, _, err := applyAt(t, s, Request{By: cat, Cmd: Unplayable{Token: play.Token, Video: play.Track.Video}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("listener reported unplayable: %v, want ErrNotAllowed while the DJ is here", err)
	}
}

func TestSkip(t *testing.T) {
	s, play := annPlaying()
	s.Roles[cat] = RoleDJ
	skip := Skip{Token: play.Token}

	for _, by := range []account.ID{ann, cat, owner} {
		next, facts := must(t, s, Request{By: by, Cmd: skip})

		choosing(t, next, bob)

		if want := []Fact{Finished{Play: play, Reason: EndSkipped}}; !reflect.DeepEqual(facts, want) {
			t.Errorf("skip by %d: facts %#v, want %#v", by, facts, want)
		}
	}

	if _, _, err := applyAt(t, s, Request{By: bob, Cmd: skip}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("member skipped someone else's song: %v, want ErrNotAllowed by default", err)
	}

	s.Settings.Skip = SkipByEveryone
	if _, _, err := applyAt(t, s, Request{By: bob, Cmd: skip}, t0); err != nil {
		t.Fatalf("member skip with \"members can skip\" on: %v, want accepted", err)
	}

	if _, _, err := applyAt(t, s, Request{By: owner, Cmd: Skip{Token: play.Token - 1}}, t0); !errors.Is(err, ErrStalePlay) {
		t.Fatalf("skip naming an old play: %v, want ErrStalePlay so two skips cannot skip two songs", err)
	}
}

func TestVoteSkipNeedsHalfOfTheListeners(t *testing.T) {
	s, play := annPlaying()
	vote := VoteSkip{Token: play.Token}

	// Eligible: owner, bob, cat (ann is playing, guests never count). 50% of 3 needs 2 votes.
	s, facts := must(t, s, Request{By: cat, Cmd: vote})
	if len(facts) != 0 || !reflect.DeepEqual(playing(t, s).Votes, []account.ID{cat}) {
		t.Fatalf("first vote: phase %#v, facts %#v; want one vote counted and the song still on", s.Phase, facts)
	}

	s, facts = must(t, s, Request{By: cat, Cmd: vote})
	if len(facts) != 0 || len(playing(t, s).Votes) != 1 {
		t.Fatal("the same person voted twice")
	}

	s, facts = must(t, s, Request{By: bob, Cmd: vote})
	choosing(t, s, bob)

	if want := []Fact{Finished{Play: play, Reason: EndVotedOut}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
}

func TestThePlayingDJCannotVoteOnTheirOwnSong(t *testing.T) {
	s, play := annPlaying()

	if _, _, err := applyAt(t, s, Request{By: ann, Cmd: VoteSkip{Token: play.Token}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("DJ voted on their own song: %v, want ErrNotAllowed", err)
	}
}

func TestVotesResetForTheNextPlay(t *testing.T) {
	s, play := annPlaying()
	s, _ = must(t, s, Request{By: cat, Cmd: VoteSkip{Token: play.Token}})
	s, _ = must(t, s, Request{By: ann, Cmd: Skip{Token: play.Token}})
	s, _ = must(t, s, TrackChosen{DJ: bob, Track: song1, Found: true})

	if votes := playing(t, s).Votes; len(votes) != 0 {
		t.Fatalf("votes carried into the next play: %v", votes)
	}
}

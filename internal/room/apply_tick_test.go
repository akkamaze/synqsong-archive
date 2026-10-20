package room

import (
	"reflect"
	"testing"
	"time"
)

func tickAt(t *testing.T, s State, at time.Time) (State, []Fact) {
	t.Helper()

	next, facts, err := Apply(s, Tick{}, at)
	if err != nil {
		t.Fatalf("tick refused: %v", err)
	}

	return next, facts
}

func TestTickEndsARestAndPassesTheTurn(t *testing.T) {
	s := roomWith(ann, bob)
	s, _ = must(t, s, Disconnected{Who: ann, Goodbye: true})
	s.Phase = startRest(ann, RestOffline, t0)

	early, _ := tickAt(t, s, t0.Add(3*time.Second-time.Millisecond))
	if !reflect.DeepEqual(early, s) {
		t.Fatal("tick before the rest ended changed the room")
	}

	s, _ = tickAt(t, s, t0.Add(3*time.Second))

	assertOrder(t, s.Booth, bob)
	choosing(t, s, bob)

	if _, ok := s.Presence[ann]; ok {
		t.Fatal("asleep DJ stayed in the room after their rest")
	}
}

func TestTickFallbackEndsASongNobodyReported(t *testing.T) {
	s, play := annPlaying()

	early, _ := tickAt(t, s, t0.Add(202*time.Second-time.Millisecond))
	if !reflect.DeepEqual(early, s) {
		t.Fatal("tick before length + 2s changed the room")
	}

	s, facts := tickAt(t, s, t0.Add(202*time.Second))

	choosing(t, s, bob)

	if want := []Fact{Finished{Play: play, Reason: EndFinished}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
}

func TestTickNeverEndsASongOfUnknownLength(t *testing.T) {
	s := roomWith(ann)
	s.Phase = Playing{Play: Play{Token: PlayToken(t0.UnixMilli()), Track: Track{Item: 3, Video: "ccccccccccc"}, DJ: ann}}

	next, facts := tickAt(t, s, t0.Add(24*time.Hour))
	if !reflect.DeepEqual(next, s) || len(facts) != 0 {
		t.Fatal("tick ended a song whose length is unknown, want it to wait for a report")
	}
}

func TestTickCountsOutAwayMembersAfterFifteenMinutes(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Playing{Play: Play{Token: PlayToken(t0.UnixMilli()), Track: Track{Item: 5, Video: "eeeeeeeeeee", Duration: time.Hour}, DJ: ann}}
	s, _ = must(t, s, Disconnected{Who: cat})
	s, _ = must(t, s, Disconnected{Who: bob})

	early, _ := tickAt(t, s, t0.Add(15*time.Minute-time.Millisecond))
	if !reflect.DeepEqual(early, s) {
		t.Fatal("tick before the grace ran out changed the room")
	}

	s, _ = tickAt(t, s, t0.Add(15*time.Minute))

	if _, ok := s.Presence[cat]; ok {
		t.Fatal("away listener is still in the room after the grace")
	}

	if got := s.Presence[bob]; got != (Asleep{Since: t0.Add(15 * time.Minute)}) {
		t.Fatalf("away DJ presence = %#v, want asleep in their slot", got)
	}
}

func TestADJCountedOutAsTheirSongEndsDropsFromTheBooth(t *testing.T) {
	s, _ := annPlaying()
	s.Phase = Playing{Play: Play{Token: PlayToken(t0.UnixMilli()), Track: Track{Item: 4, Video: "ddddddddddd", Duration: 15 * time.Minute}, DJ: ann}}
	s, _ = must(t, s, Disconnected{Who: ann})

	s, _ = tickAt(t, s, t0.Add(15*time.Minute+2*time.Second))

	assertOrder(t, s.Booth, bob)
}

func TestTickTwiceIsTheSameAsOnce(t *testing.T) {
	s, _ := annPlaying()
	at := t0.Add(202 * time.Second)

	once, _ := tickAt(t, s, at)
	twice, facts := tickAt(t, once, at)

	if !reflect.DeepEqual(once, twice) || len(facts) != 0 {
		t.Fatal("a second tick at the same time changed the room again")
	}
}

func TestNextDeadline(t *testing.T) {
	if _, ok := NextDeadline(roomWith()); ok {
		t.Fatal("an idle room with everyone here has a deadline")
	}

	playingRoom, _ := annPlaying()
	if at, ok := NextDeadline(playingRoom); !ok || !at.Equal(t0.Add(202*time.Second)) {
		t.Fatalf("playing deadline = %v, %v; want the fallback at length + 2s", at, ok)
	}

	resting := roomWith(ann)
	resting.Phase = startRest(ann, RestOffline, t0)
	if at, ok := NextDeadline(resting); !ok || !at.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("resting deadline = %v, %v; want the rest's end", at, ok)
	}

	away, _ := must(t, playingRoom, Disconnected{Who: cat})
	if at, ok := NextDeadline(away); !ok || !at.Equal(t0.Add(202*time.Second)) {
		t.Fatalf("deadline with an away member = %v, %v; want the earlier fallback", at, ok)
	}

	away.Phase = Idle{}
	if at, ok := NextDeadline(away); !ok || !at.Equal(t0.Add(15*time.Minute)) {
		t.Fatalf("idle deadline with an away member = %v, %v; want their grace end", at, ok)
	}
}

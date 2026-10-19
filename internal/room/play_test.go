package room

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.UnixMilli(1_790_000_000_000)

func playOf(length time.Duration) Play {
	return Play{
		Token: PlayToken(t0.UnixMilli()),
		Track: Track{Video: "dQw4w9WgXcQ", Duration: length},
		DJ:    7,
	}
}

func TestNextTokenNeverRepeats(t *testing.T) {
	first := nextToken(0, t0)
	if first != PlayToken(t0.UnixMilli()) {
		t.Fatalf("first token = %d, want the start time %d", first, t0.UnixMilli())
	}

	sameMillisecond := nextToken(first, t0)
	if sameMillisecond != first+1 {
		t.Fatalf("token in the same millisecond = %d, want %d", sameMillisecond, first+1)
	}

	clockWentBack := nextToken(sameMillisecond, t0.Add(-time.Minute))
	if clockWentBack != sameMillisecond+1 {
		t.Fatalf("token after the clock went back = %d, want %d", clockWentBack, sameMillisecond+1)
	}
}

func TestCheckReportNamesThisPlay(t *testing.T) {
	play := playOf(200 * time.Second)

	if err := play.CheckReport(play.Token, play.Track.Video); err != nil {
		t.Fatalf("report for this play: %v, want accepted", err)
	}

	if err := play.CheckReport(play.Token-1, play.Track.Video); !errors.Is(err, ErrStalePlay) {
		t.Fatalf("report with the previous token: %v, want ErrStalePlay", err)
	}

	if err := play.CheckReport(play.Token, "xxxxxxxxxxx"); !errors.Is(err, ErrMismatchedVideo) {
		t.Fatalf("report from the previous video's iframe: %v, want ErrMismatchedVideo", err)
	}
}

func TestCheckEndedRefusesEarlyEnds(t *testing.T) {
	play := playOf(200 * time.Second)

	if err := play.CheckEnded(t0.Add(195*time.Second - time.Millisecond)); !errors.Is(err, ErrEndedTooEarly) {
		t.Fatalf("ended 5.001s before the end: %v, want ErrEndedTooEarly", err)
	}

	if err := play.CheckEnded(t0.Add(195 * time.Second)); err != nil {
		t.Fatalf("ended 5s before the end: %v, want accepted", err)
	}
}

func TestCheckEndedTrustsReportsWhenLengthUnknownOrShort(t *testing.T) {
	if err := playOf(0).CheckEnded(t0.Add(time.Second)); err != nil {
		t.Fatalf("unknown length, ended after 1s: %v, want accepted (no fallback exists)", err)
	}

	if err := playOf(3 * time.Second).CheckEnded(t0); err != nil {
		t.Fatalf("genuine 3s clip: %v, want accepted", err)
	}
}

func TestFallbackAt(t *testing.T) {
	at, ok := playOf(200 * time.Second).FallbackAt()
	if !ok || !at.Equal(t0.Add(202*time.Second)) {
		t.Fatalf("fallback = %v, %v; want length + 2s grace = %v", at, ok, t0.Add(202*time.Second))
	}

	if _, ok := playOf(0).FallbackAt(); ok {
		t.Fatal("fallback armed without a known length, want none as in td-fm")
	}
}

func TestUnplayableGraceIsThreeSeconds(t *testing.T) {
	if UnplayableGrace != 3*time.Second {
		t.Fatalf("unplayable grace = %v, want 3s for every failure (owner's change from td-fm's 10s/3s)", UnplayableGrace)
	}
}

func TestRestLastsThreeSeconds(t *testing.T) {
	rest := startRest(7, RestOffline, t0)

	if !rest.Until.Equal(t0.Add(3 * time.Second)) {
		t.Fatalf("rest until = %v, want 3s after it starts (owner's change from td-fm's 10s)", rest.Until)
	}
}

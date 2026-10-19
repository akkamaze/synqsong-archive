package room

import (
	"testing"
	"time"

	"synqsong/internal/account"
)

func TestRestoredRoomCountsBoothDJsAwayFromTheLastSave(t *testing.T) {
	s := roomWith(ann, bob)
	s.Presence = nil
	savedAt := t0.Add(-time.Minute)

	s = Restored(s, savedAt)

	for _, dj := range []account.ID{ann, bob} {
		if got := s.Presence[dj]; got != (Away{Since: savedAt}) {
			t.Errorf("DJ %d presence after restore = %#v, want away since the last save", dj, got)
		}
	}

	if _, ok := s.Presence[cat]; ok {
		t.Fatal("a listener outside the booth is present after a restore, want nobody until they connect")
	}
}

func TestARestoredDJWhoNeverReturnsIsCountedOut(t *testing.T) {
	s := roomWith(ann, bob)
	s.Phase = Playing{Play: Play{Token: PlayToken(t0.UnixMilli()), Track: Track{Item: 9, Video: "fffffffffff", Duration: time.Hour}, DJ: ann}}
	s = Restored(s, t0)

	s, _ = tickAt(t, s, t0.Add(15*time.Minute))

	if got := s.Presence[bob]; got != (Asleep{Since: t0.Add(15 * time.Minute)}) {
		t.Fatalf("restored on-deck DJ after the grace = %#v, want asleep", got)
	}
}

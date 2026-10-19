package room

import (
	"errors"
	"time"

	"github.com/akkamaze/tutti/internal/account"
)

// Timings carried over from td-fm's room engine (server/src/room.js).
const (
	endGrace      = 2 * time.Second
	earlyEndSlack = 5 * time.Second
)

// UnplayableGrace is how long browsers wait on a broken video before reporting it.
// td-fm waited 10s for the first failure in a turn; the owner chose 3s for every one.
const UnplayableGrace = 3 * time.Second

// PlayToken is the play's start time in Unix milliseconds. It also tells apart
// two plays of the same video, so every report about a play must name it.
type PlayToken int64

// nextToken never repeats a token, even for two starts in the same millisecond.
func nextToken(last PlayToken, now time.Time) PlayToken {
	return max(PlayToken(now.UnixMilli()), last+1)
}

func (t PlayToken) startedAt() time.Time {
	return time.UnixMilli(int64(t))
}

// Play is one round of one track on stage.
type Play struct {
	Token PlayToken
	Track Track
	DJ    account.ID
}

var (
	ErrStalePlay       = errors.New("that report is about a song that is no longer playing")
	ErrMismatchedVideo = errors.New("that report names a different video than the one playing")
	ErrEndedTooEarly   = errors.New("that song cannot have ended yet")
)

// CheckReport accepts a browser's report only if it names this exact play.
func (p Play) CheckReport(token PlayToken, video VideoID) error {
	if token != p.Token {
		return ErrStalePlay
	}

	if video != p.Track.Video {
		return ErrMismatchedVideo
	}

	return nil
}

// CheckEnded refuses an "ended" report that comes well before the known end.
// A late listener's player can finish the previous song after the room moved on.
func (p Play) CheckEnded(now time.Time) error {
	length, known := p.Track.KnownDuration()
	if !known {
		return nil
	}

	if now.Sub(p.Token.startedAt()) < length-earlyEndSlack {
		return ErrEndedTooEarly
	}

	return nil
}

// FallbackAt is when the room moves on by itself if nobody reports the end.
// Without a known length there is no fallback, as in td-fm.
func (p Play) FallbackAt() (time.Time, bool) {
	length, known := p.Track.KnownDuration()
	if !known {
		return time.Time{}, false
	}

	return p.Token.startedAt().Add(length + endGrace), true
}

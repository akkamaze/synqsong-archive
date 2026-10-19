package room

import (
	"time"

	"synqsong/internal/account"
)

// restLength is how long a DJ who cannot play stays on stage saying goodbye.
const restLength = 3 * time.Second

// Phase is what the stage is doing. Exactly one of Idle, Choosing, Playing or Resting.
//
//sumtype:decl
type Phase interface{ isPhase() }

// Idle means nobody in the booth has a song to play.
type Idle struct{}

// Choosing waits for the head DJ's next track from their queue.
type Choosing struct {
	DJ account.ID
	// Failed lists this DJ's songs that would not play in this turn.
	Failed []QueueItemID
	// Passed counts DJs rotated past for having no songs during this search.
	Passed int
}

type Playing struct {
	Play   Play
	Failed []QueueItemID
	// Votes are the listeners who voted to skip this play, once each.
	Votes []account.ID
}

// Resting shows a DJ on stage who cannot take their turn, before they leave the booth.
type Resting struct {
	DJ     account.ID
	Reason RestReason
	Until  time.Time
}

func (Idle) isPhase()     {}
func (Choosing) isPhase() {}
func (Playing) isPhase()  {}
func (Resting) isPhase()  {}

type RestReason int8

const (
	// RestOffline: the DJ's turn came while they were away.
	RestOffline RestReason = iota + 1
	// RestUnplayable: every song in this turn failed to play.
	RestUnplayable
)

func startRest(dj account.ID, reason RestReason, now time.Time) Resting {
	return Resting{DJ: dj, Reason: reason, Until: now.Add(restLength)}
}

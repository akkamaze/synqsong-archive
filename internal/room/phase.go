package room

import (
	"time"

	"github.com/akkamaze/tutti/internal/account"
)

// restLength is how long a DJ who cannot play stays on stage saying goodbye.
// td-fm used 10s; the owner chose 3s.
const restLength = 3 * time.Second

// Phase is what the stage is doing. Exactly one of Idle, Playing or Resting.
//
//sumtype:decl
type Phase interface{ isPhase() }

// Idle means nobody in the booth has a song to play.
type Idle struct{}

type Playing struct {
	Play Play
}

// Resting shows a DJ on stage who cannot take their turn, before they leave the booth.
type Resting struct {
	DJ     account.ID
	Reason RestReason
	Until  time.Time
}

func (Idle) isPhase()    {}
func (Playing) isPhase() {}
func (Resting) isPhase() {}

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

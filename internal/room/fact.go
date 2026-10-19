package room

import (
	"time"

	"synqsong/internal/account"
)

// Fact is something Apply decided that the state alone does not record.
//
//sumtype:decl
type Fact interface{ isFact() }

// Finished ends a play: it goes into history and its song to the bottom of its owner's queue.
type Finished struct {
	Play   Play
	Reason EndReason
}

// LengthLearned remembers a song's length so its next play can arm the fallback at once.
type LengthLearned struct {
	Item     QueueItemID
	Duration time.Duration
}

// Said is a chat line to keep forever.
type Said struct {
	By   account.ID
	Body ChatBody
	At   time.Time
}

// Removed tells the runtime to close the member's connections and their membership.
type Removed struct {
	Who account.ID
}

func (Finished) isFact()      {}
func (LengthLearned) isFact() {}
func (Said) isFact()          {}
func (Removed) isFact()       {}

type EndReason int8

const (
	EndFinished EndReason = iota + 1
	EndSkipped
	EndVotedOut
	EndUnplayable
)

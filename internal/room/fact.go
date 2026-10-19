package room

import "time"

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

func (Finished) isFact()      {}
func (LengthLearned) isFact() {}

type EndReason int8

const (
	EndFinished EndReason = iota + 1
	EndSkipped
	EndVotedOut
	EndUnplayable
)

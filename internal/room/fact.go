package room

// Fact is something Apply decided that the state alone does not record.
//
//sumtype:decl
type Fact interface{ isFact() }

// Finished ends a play: it goes into history and its song to the bottom of its owner's queue.
type Finished struct {
	Play   Play
	Reason EndReason
}

func (Finished) isFact() {}

type EndReason int8

const (
	EndFinished EndReason = iota + 1
	EndSkipped
	EndVotedOut
	EndUnplayable
)

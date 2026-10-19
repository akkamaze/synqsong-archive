package room

import "time"

// QueueItemID identifies one entry in an account's song queue.
type QueueItemID int64

// Track is the song a DJ's queue hands to the room.
type Track struct {
	Item      QueueItemID
	Video     VideoID
	Title     string
	Channel   string
	Thumbnail string
	Duration  time.Duration
}

// KnownDuration is false when YouTube did not report a length yet.
func (t Track) KnownDuration() (time.Duration, bool) {
	return t.Duration, t.Duration > 0
}

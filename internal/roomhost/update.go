package roomhost

import "github.com/akkamaze/synqsong/internal/room"

// Update is a saved change: the room's state at Rev and what happened to get there.
// State is shared with other sinks and must not be modified.
type Update struct {
	Rev   Rev
	State room.State
	Facts []room.Fact
}

// Sink receives a room's updates in order. Send is called from the room's goroutine
// and must not block; returning false means the sink fell behind and the room drops it.
type Sink interface {
	Send(Update) bool
}

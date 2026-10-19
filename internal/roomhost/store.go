// Package roomhost runs each room as one goroutine that applies inputs in order,
// saves each change before anyone sees it, and sends updates to attached sinks.
package roomhost

import (
	"context"
	"errors"

	"github.com/akkamaze/tutti/internal/room"
)

// Rev counts a room's saved changes. A commit names the rev it builds on.
type Rev int64

// Store saves one change of a room in one transaction. It returns ErrFenced
// when the saved rev is no longer prev, which means another process owns the room.
type Store interface {
	Commit(ctx context.Context, id room.ID, prev Rev, next room.State, facts []room.Fact) error
}

var (
	ErrFenced      = errors.New("another process took over this room")
	ErrUnavailable = errors.New("the room could not save that change, try again")
	ErrClosed      = errors.New("this room is closed")
)

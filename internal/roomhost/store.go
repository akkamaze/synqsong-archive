// Package roomhost runs each room as one goroutine that applies inputs in order,
// saves each change before anyone sees it, and sends updates to attached sinks.
package roomhost

import (
	"context"
	"errors"

	"github.com/akkamaze/synqsong/internal/account"
	"github.com/akkamaze/synqsong/internal/room"
)

// Rev counts a room's saved changes. A commit names the rev it builds on.
type Rev int64

// Store keeps rooms. Load reads a room as last saved. Commit saves one change in
// one transaction, and returns ErrFenced when the saved rev is no longer prev,
// which means another process owns the room.
type Store interface {
	Load(ctx context.Context, id room.ID) (room.State, Rev, error)
	Commit(ctx context.Context, id room.ID, prev Rev, next room.State, facts []room.Fact) error
}

// Queues reads the first song in a DJ's queue. The queue belongs to the account, not the room.
type Queues interface {
	Head(ctx context.Context, dj account.ID) (room.Track, bool, error)
}

var (
	ErrFenced      = errors.New("another process took over this room")
	ErrUnavailable = errors.New("the room could not save that change, try again")
	ErrClosed      = errors.New("this room is closed")
)

// Package queue keeps each account's song queue, used in every room the account DJs in.
package queue

import (
	"context"
	"errors"

	"synqsong/internal/account"
	"synqsong/internal/room"
)

// Store owns queue rows. Placement: a new song goes to the top, or right after
// the song this DJ is playing now so their set is not cut off.
type Store interface {
	Queue(ctx context.Context, owner account.ID) ([]room.Track, error)
	Queued(ctx context.Context, owner account.ID, video room.VideoID) (room.Track, bool, error)
	Add(ctx context.Context, owner account.ID, track room.Track) (room.Track, error)
	Remove(ctx context.Context, owner account.ID, items []room.QueueItemID) (int, error)
	Reorder(ctx context.Context, owner account.ID, order []room.QueueItemID) error
}

var (
	ErrAlreadyQueued = errors.New("that song is already in your queue")
	ErrStaleOrder    = errors.New("your queue changed meanwhile, reload it and try again")
)

// AlreadyQueuedError carries the queued song, so the screen can point at it.
type AlreadyQueuedError struct {
	Track room.Track
}

func (e *AlreadyQueuedError) Error() string { return ErrAlreadyQueued.Error() }

func (e *AlreadyQueuedError) Is(target error) bool { return target == ErrAlreadyQueued }

package room

import "errors"

var (
	ErrNotMember  = errors.New("join the room before doing that")
	ErrNotAllowed = errors.New("your role in this room does not allow that")
	ErrBanned     = errors.New("the owner removed you from this room")
	ErrTooFast    = errors.New("wait a moment before sending another message")
)

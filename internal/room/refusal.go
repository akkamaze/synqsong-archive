package room

import "errors"

var (
	ErrNotMember  = errors.New("join the room before doing that")
	ErrNotAllowed = errors.New("your role in this room does not allow that")
)

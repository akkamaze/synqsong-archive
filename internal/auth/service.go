package auth

import (
	"crypto/rand"
	"errors"
	"io"
	"time"
)

// sessionIdle ends a session left unused this long.
const sessionIdle = 90 * 24 * time.Hour

var (
	// ErrWrongLogin covers an unknown username and a wrong PIN alike, so neither can be probed.
	ErrWrongLogin    = errors.New("that username or PIN is not right")
	ErrNoSession     = errors.New("sign in again")
	ErrNotAdmin      = errors.New("only a site admin can do that")
	ErrPINMustDiffer = errors.New("PIN: choose a PIN different from the one an admin gave you")
	ErrMustChangePIN = errors.New("choose your own PIN before doing anything else")
	ErrBusy          = errors.New("the server is busy, try again in a moment")
)

// Session is what a browser holds after signing in, with a fresh device token for its next sign-in.
type Session struct {
	Token    Token
	Device   DeviceToken
	Identity Identity
}

type Service struct {
	keys   Keys
	pins   *pinWork
	store  Store
	limits *Limiter
	now    func() time.Time
	random io.Reader
}

func NewService(keys Keys, store Store, now func() time.Time) *Service {
	return &Service{
		keys:   keys,
		pins:   newPINWork(keys),
		store:  store,
		limits: NewLimiter(limiterMaxKeys),
		now:    now,
		random: rand.Reader,
	}
}

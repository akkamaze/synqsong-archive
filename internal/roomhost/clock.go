package roomhost

import "time"

// Clock is the room's source of time, replaced by a fake one in tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock is wall-clock time.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

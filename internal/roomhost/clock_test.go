package roomhost

import (
	"sync"
	"time"
)

type waiter struct {
	at time.Time
	ch chan time.Time
}

// fakeClock only moves when a test calls Advance.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), ch: ch})

	return ch
}

// awaitWaiter blocks until someone is waiting on the clock, so Advance cannot race the room arming its timer.
func (c *fakeClock) awaitWaiter(t interface{ Fatal(...any) }) {
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.waiters)
		c.mu.Unlock()

		if n > 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("the room never armed its timer")
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)

	pending := c.waiters[:0]
	for _, w := range c.waiters {
		if w.at.After(c.now) {
			pending = append(pending, w)

			continue
		}

		w.ch <- c.now
	}

	c.waiters = pending
}

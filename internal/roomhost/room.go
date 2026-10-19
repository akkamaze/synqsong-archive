package roomhost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"synqsong/internal/room"
)

// minWake keeps the timer at least this far ahead, as td-fm's armFallback did,
// so a deadline already past when it is learned fires a beat later, not at once.
const minWake = time.Second

type Config struct {
	ID     room.ID
	State  room.State
	Rev    Rev
	Store  Store
	Queues Queues
	Clock  Clock
}

// Room is a handle to one running room. All its methods are safe to call from any goroutine.
type Room struct {
	inbox chan func(*actor)
	done  chan struct{}
}

// actor owns a room's state. Only its goroutine touches these fields.
type actor struct {
	ctx    context.Context
	cfg    Config
	state  room.State
	rev    Rev
	sinks  map[Sink]struct{}
	fenced bool
	// wake fires when the room next needs a Tick; nil when nothing is due.
	wake <-chan time.Time
	// wakeFor is the deadline wake was armed for, so an input that keeps it keeps the timer.
	wakeFor time.Time
	// lookupFailed: the queue could not be read for the DJ in Choosing; retry on wake.
	lookupFailed bool
}

// Start runs the room until ctx ends or another process takes it over.
// Store, Queues and Clock are required.
func Start(ctx context.Context, cfg Config) *Room {
	if cfg.Store == nil || cfg.Queues == nil || cfg.Clock == nil {
		panic("roomhost.Start: Config needs Store, Queues and Clock")
	}

	r := &Room{inbox: make(chan func(*actor)), done: make(chan struct{})}
	a := &actor{ctx: ctx, cfg: cfg, state: cfg.State, rev: cfg.Rev, sinks: map[Sink]struct{}{}}

	go a.run(r)

	return r
}

func (a *actor) run(r *Room) {
	defer close(r.done)

	a.choose()
	a.rearm()

	for !a.fenced {
		select {
		case job := <-r.inbox:
			job(a)

		case <-a.wake:
			a.wake = nil
			_ = a.apply(room.Tick{})

		case <-a.ctx.Done():
			return
		}

		a.choose()
		a.rearm()
	}
}

// choose answers Choosing with the DJ's first song until the stage settles,
// as td-fm's playHead read the songs table. A failed read is retried on the next wake.
func (a *actor) choose() {
	a.lookupFailed = false

	for !a.fenced {
		c, ok := a.state.Phase.(room.Choosing)
		if !ok {
			return
		}

		track, found, err := a.cfg.Queues.Head(a.ctx, c.DJ)
		if err != nil {
			a.lookupFailed = true

			return
		}

		before := a.rev
		if err := a.apply(room.TrackChosen{DJ: c.DJ, Track: track, Found: found}); err != nil || a.rev == before {
			return
		}
	}
}

func (a *actor) rearm() {
	if a.lookupFailed {
		a.wake, a.wakeFor = a.cfg.Clock.After(minWake), time.Time{}

		return
	}

	deadline, ok := room.NextDeadline(a.state)
	if !ok {
		a.wake, a.wakeFor = nil, time.Time{}

		return
	}

	if a.wake != nil && deadline.Equal(a.wakeFor) {
		return
	}

	a.wake, a.wakeFor = a.cfg.Clock.After(max(deadline.Sub(a.cfg.Clock.Now()), minWake)), deadline
}

// Do applies one input and returns once it is saved and sent, or why it was refused.
func (r *Room) Do(ctx context.Context, in room.Input) error {
	reply := make(chan error, 1)

	if err := r.call(ctx, func(a *actor) { reply <- a.apply(in) }); err != nil {
		return err
	}

	result, err := wait(ctx, reply)
	if err != nil {
		return err
	}

	return result
}

// Attach returns the room as it is now and sends every later update to sink until detached.
func (r *Room) Attach(ctx context.Context, sink Sink) (Update, func(), error) {
	reply := make(chan Update, 1)

	err := r.call(ctx, func(a *actor) {
		a.sinks[sink] = struct{}{}
		reply <- Update{Rev: a.rev, State: a.state}
	})
	if err != nil {
		return Update{}, func() {}, err
	}

	snapshot, err := wait(ctx, reply)
	if err != nil {
		return Update{}, func() {}, err
	}

	detach := func() {
		_ = r.call(context.Background(), func(a *actor) { delete(a.sinks, sink) })
	}

	return snapshot, detach, nil
}

func (r *Room) stopped() bool {
	select {
	case <-r.done:
		return true

	default:
		return false
	}
}

func (r *Room) call(ctx context.Context, job func(*actor)) error {
	select {
	case r.inbox <- job:
		return nil

	case <-r.done:
		return ErrClosed

	case <-ctx.Done():
		return ctx.Err()
	}
}

func wait[T any](ctx context.Context, reply <-chan T) (T, error) {
	select {
	case v := <-reply:
		return v, nil

	case <-ctx.Done():
		var zero T

		return zero, ctx.Err()
	}
}

// apply saves a change before anyone sees it. A failed save keeps the old state.
func (a *actor) apply(in room.Input) error {
	next, facts, err := room.Apply(a.state, in, a.cfg.Clock.Now())
	if err != nil {
		return err
	}

	if len(facts) == 0 && reflect.DeepEqual(next, a.state) {
		return nil
	}

	if err := a.cfg.Store.Commit(a.ctx, a.cfg.ID, a.rev, a.state, next, facts); err != nil {
		if errors.Is(err, ErrFenced) {
			a.fenced = true

			return ErrFenced
		}

		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	a.state = next
	a.rev++

	update := Update{Rev: a.rev, State: next, Facts: facts}
	for sink := range a.sinks {
		if !sink.Send(update) {
			delete(a.sinks, sink)
		}
	}

	return nil
}

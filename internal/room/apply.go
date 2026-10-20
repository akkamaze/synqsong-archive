package room

import (
	"slices"
	"time"

	"synqsong/internal/account"
)

// Apply applies one input to a room. It never changes s; on refusal it returns s unchanged.
func Apply(s State, in Input, now time.Time) (State, []Fact, error) {
	switch in := in.(type) {
	case Request:
		return applyRequest(s, in, now)

	case TrackChosen:
		return chooseTrack(s, in, now), nil, nil

	case Connected:
		return connected(s, in, now)

	case Disconnected:
		next, facts := disconnected(s, in, now)

		return next, facts, nil

	case Admit:
		return admit(s, in)

	case Tick:
		next, facts := tick(s, now)

		return next, facts, nil
	}

	return s, nil, nil
}

func applyRequest(s State, req Request, now time.Time) (State, []Fact, error) {
	role, ok := s.Roles[req.By]
	if !ok {
		return s, nil, ErrNotMember
	}

	switch cmd := req.Cmd.(type) {
	case StepUp:
		return guard(s, role, ActionJoinBooth, func() (State, []Fact) { return stepUp(s, req, now) })

	case StepDown:
		next, facts := stepDown(s, req, now)

		return next, facts, nil

	case KickFromBooth:
		return guard(s, role, ActionKickFromBooth, func() (State, []Fact) { return removeFromBooth(s, cmd.DJ, now) })

	case Loaded:
		return loaded(s, role, cmd)

	case Ended:
		return ended(s, role, cmd, now)

	case Unplayable:
		return unplayable(s, req.By, role, cmd, now)

	case Skip:
		return skip(s, req.By, role, cmd, now)

	case VoteSkip:
		return voteSkip(s, req.By, role, cmd, now)

	case Say:
		return say(s, req.By, role, cmd, now)

	case RemoveFromRoom:
		return removeFromRoom(s, req.By, role, cmd, now)

	case AssignRole:
		return assignRole(s, role, cmd)

	case ChangeSettings:
		return changeSettings(s, role, cmd)
	}

	return s, nil, nil
}

func guard(s State, role Role, action Action, apply func() (State, []Fact)) (State, []Fact, error) {
	if !Can(role, s.Settings, action) {
		return s, nil, ErrNotAllowed
	}

	next, facts := apply()

	return next, facts, nil
}

// beginTurn gives the stage to the head DJ, or idles when the booth is empty.
func beginTurn(s State, now time.Time) State {
	head, ok := s.Booth.Head()
	if !ok {
		s.Phase = Idle{}

		return s
	}

	return turnFor(s, head, 0, now)
}

// turnFor starts a DJ's turn. An asleep DJ rests on stage instead.
func turnFor(s State, dj account.ID, passed int, now time.Time) State {
	if isAsleep(s, dj) {
		s.Phase = startRest(dj, RestOffline, now)

		return s
	}

	s.Phase = Choosing{DJ: dj, Passed: passed}

	return s
}

func chooseTrack(s State, in TrackChosen, now time.Time) State {
	c, ok := s.Phase.(Choosing)
	if !ok || c.DJ != in.DJ {
		return s
	}

	if !in.Found {
		return passEmptyQueue(s, c, now)
	}

	if slices.Contains(c.Failed, in.Track.Item) {
		s.Phase = startRest(c.DJ, RestUnplayable, now)

		return s
	}

	s.LastToken = nextToken(s.LastToken, now)
	s.Phase = Playing{
		Play:   Play{Token: s.LastToken, Track: in.Track, DJ: c.DJ},
		Failed: c.Failed,
	}

	return s
}

// passEmptyQueue rotates past a DJ with no songs, and idles once every DJ was tried.
func passEmptyQueue(s State, c Choosing, now time.Time) State {
	s.Booth = s.Booth.RotatePast()
	passed := c.Passed + 1

	if passed >= len(s.Booth.DJs()) {
		s.Phase = Idle{}

		return s
	}

	head, _ := s.Booth.Head()

	return turnFor(s, head, passed, now)
}

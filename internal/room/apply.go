package room

import (
	"slices"
	"time"
)

// Apply applies one input to a room. It never changes s; on refusal it returns s unchanged.
func Apply(s State, in Input, now time.Time) (State, []Fact, error) {
	switch in := in.(type) {
	case Request:
		return applyRequest(s, in, now)

	case TrackChosen:
		return chooseTrack(s, in, now), nil, nil
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
		return guard(s, role, ActionJoinBooth, func() (State, []Fact) { return stepUp(s, req) })

	case StepDown:
		return stepDown(s, req), nil, nil

	case KickFromBooth:
		return guard(s, role, ActionKickFromBooth, func() (State, []Fact) { return removeFromBooth(s, cmd.DJ) })
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

// beginTurn looks for the head DJ's song, or idles when the booth is empty.
func beginTurn(s State) State {
	head, ok := s.Booth.Head()
	if !ok {
		s.Phase = Idle{}

		return s
	}

	s.Phase = Choosing{DJ: head}

	return s
}

func chooseTrack(s State, in TrackChosen, now time.Time) State {
	c, ok := s.Phase.(Choosing)
	if !ok || c.DJ != in.DJ {
		return s
	}

	if !in.Found {
		return passEmptyQueue(s, c)
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
func passEmptyQueue(s State, c Choosing) State {
	s.Booth = s.Booth.RotatePast()
	passed := c.Passed + 1

	if passed >= len(s.Booth.DJs()) {
		s.Phase = Idle{}

		return s
	}

	head, _ := s.Booth.Head()
	s.Phase = Choosing{DJ: head, Passed: passed}

	return s
}

package room

import (
	"maps"
	"time"

	"synqsong/internal/account"
)

func connected(s State, in Connected, now time.Time) (State, []Fact, error) {
	if _, ok := s.Roles[in.Who]; !ok {
		return s, nil, ErrNotMember
	}

	wasAsleep := isAsleep(s, in.Who)

	conns := 1
	if here, ok := s.Presence[in.Who].(Here); ok {
		conns = here.Conns + 1
	}

	s = setPresence(s, in.Who, Here{Conns: conns})

	if rest, ok := s.Phase.(Resting); ok && wasAsleep && rest.DJ == in.Who && rest.Reason == RestOffline {
		return beginTurn(s, now), nil, nil
	}

	if _, idle := s.Phase.(Idle); idle && s.Booth.Has(in.Who) {
		return beginTurn(s, now), nil, nil
	}

	return s, nil, nil
}

func disconnected(s State, in Disconnected, now time.Time) (State, []Fact) {
	here, ok := s.Presence[in.Who].(Here)
	if !ok {
		return s, nil
	}

	if here.Conns > 1 {
		return setPresence(s, in.Who, Here{Conns: here.Conns - 1}), nil
	}

	if !in.Goodbye {
		return setPresence(s, in.Who, Away{Since: now}), nil
	}

	return markGone(s, in.Who, in.SignOut, now)
}

// markGone counts someone out. A DJ keeps their slot asleep;
// signing out gives up an on-deck slot at once; a listener just leaves.
func markGone(s State, who account.ID, signOut bool, now time.Time) (State, []Fact) {
	if !s.Booth.Has(who) {
		return removePresence(s, who), nil
	}

	if dj, playing := playingDJ(s); signOut && (!playing || dj != who) {
		s = removePresence(s, who)

		return removeFromBooth(s, who, now)
	}

	s = setPresence(s, who, Asleep{Since: now})

	switch phase := s.Phase.(type) {
	case Resting:
		if phase.DJ == who {
			phase.Reason = RestOffline
			s.Phase = phase
		}

	case Choosing:
		if phase.DJ == who {
			s.Phase = startRest(who, RestOffline, now)
		}

	case Idle, Playing:
	}

	return s, nil
}

// sweepIfGone drops an asleep member whose DJ slot was the only thing keeping them here.
func sweepIfGone(s State, who account.ID) State {
	if isAsleep(s, who) && !s.Booth.Has(who) {
		return removePresence(s, who)
	}

	return s
}

func isAsleep(s State, who account.ID) bool {
	_, asleep := s.Presence[who].(Asleep)

	return asleep
}

func isHere(s State, who account.ID) bool {
	_, here := s.Presence[who].(Here)

	return here
}

func setPresence(s State, who account.ID, p Presence) State {
	s.Presence = maps.Clone(s.Presence)
	if s.Presence == nil {
		s.Presence = map[account.ID]Presence{}
	}

	s.Presence[who] = p

	return s
}

func removePresence(s State, who account.ID) State {
	s.Presence = maps.Clone(s.Presence)
	delete(s.Presence, who)

	return s
}

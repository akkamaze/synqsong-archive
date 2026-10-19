package room

import (
	"maps"
	"slices"
	"time"

	"synqsong/internal/account"
)

// tick counts out expired away members first, so a DJ who is gone as their song
// ends drops from the booth instead of going round again.
func tick(s State, now time.Time) (State, []Fact) {
	var facts []Fact

	for _, who := range awayExpired(s, now) {
		var gone []Fact
		s, gone = markGone(s, who, false, now)
		facts = append(facts, gone...)
	}

	switch phase := s.Phase.(type) {
	case Resting:
		if !now.Before(phase.Until) {
			var ended []Fact
			s, ended = removeFromBooth(s, phase.DJ, now)
			facts = append(facts, ended...)
		}

	case Playing:
		if at, ok := phase.Play.FallbackAt(); ok && !now.Before(at) {
			var ended []Fact
			s, ended = endTurn(s, phase, EndFinished, now)
			facts = append(facts, ended...)
		}

	case Idle, Choosing:
	}

	return s, facts
}

func awayExpired(s State, now time.Time) []account.ID {
	var expired []account.ID

	for _, who := range slices.Sorted(maps.Keys(s.Presence)) {
		if away, ok := s.Presence[who].(Away); ok && !now.Before(away.Since.Add(awayGrace)) {
			expired = append(expired, who)
		}
	}

	return expired
}

// NextDeadline is when the room next needs a Tick, if ever.
func NextDeadline(s State) (time.Time, bool) {
	var deadlines []time.Time

	switch phase := s.Phase.(type) {
	case Resting:
		deadlines = append(deadlines, phase.Until)

	case Playing:
		if at, ok := phase.Play.FallbackAt(); ok {
			deadlines = append(deadlines, at)
		}

	case Idle, Choosing:
	}

	for _, p := range s.Presence {
		if away, ok := p.(Away); ok {
			deadlines = append(deadlines, away.Since.Add(awayGrace))
		}
	}

	if len(deadlines) == 0 {
		return time.Time{}, false
	}

	return slices.MinFunc(deadlines, func(a, b time.Time) int { return a.Compare(b) }), true
}

package room

import (
	"time"

	"github.com/akkamaze/synqsong/internal/account"
)

func stepUp(s State, req Request, now time.Time) (State, []Fact) {
	s.Booth = s.Booth.StepUp(req.By)

	if _, idle := s.Phase.(Idle); idle {
		return beginTurn(s, now), nil
	}

	return s, nil
}

func stepDown(s State, req Request, now time.Time) (State, []Fact) {
	if !s.Booth.Has(req.By) {
		return s, nil
	}

	if dj, ok := playingDJ(s); ok && dj == req.By {
		s.Booth = s.Booth.StepDown(req.By, true)

		return s, nil
	}

	return removeFromBooth(s, req.By, now)
}

// removeFromBooth takes a DJ out at once. A song they were playing counts as
// finished, and a turn they were holding passes to the next DJ, as in td-fm.
func removeFromBooth(s State, dj account.ID, now time.Time) (State, []Fact) {
	if !s.Booth.Has(dj) {
		return s, nil
	}

	wasHead := s.Booth.isHead(dj)
	s.Booth = s.Booth.Remove(dj)
	s = sweepIfGone(s, dj)

	if !wasHead {
		return s, nil
	}

	var facts []Fact
	if p, ok := s.Phase.(Playing); ok {
		facts = append(facts, Finished{Play: p.Play, Reason: EndFinished})
	}

	if _, idle := s.Phase.(Idle); idle {
		return s, facts
	}

	return beginTurn(s, now), facts
}

func playingDJ(s State) (account.ID, bool) {
	p, ok := s.Phase.(Playing)
	if !ok {
		return 0, false
	}

	return p.Play.DJ, true
}

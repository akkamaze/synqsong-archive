package room

import "github.com/akkamaze/tutti/internal/account"

func stepUp(s State, req Request) (State, []Fact) {
	s.Booth = s.Booth.StepUp(req.By)

	if _, idle := s.Phase.(Idle); idle {
		return beginTurn(s), nil
	}

	return s, nil
}

func stepDown(s State, req Request) State {
	if !s.Booth.Has(req.By) {
		return s
	}

	if dj, ok := playingDJ(s); ok && dj == req.By {
		s.Booth = s.Booth.StepDown(req.By, true)

		return s
	}

	next, _ := removeFromBooth(s, req.By)

	return next
}

// removeFromBooth takes a DJ out at once. A song they were playing counts as
// finished, and a turn they were holding passes to the next DJ, as in td-fm.
func removeFromBooth(s State, dj account.ID) (State, []Fact) {
	if !s.Booth.Has(dj) {
		return s, nil
	}

	wasHead := s.Booth.isHead(dj)
	s.Booth = s.Booth.Remove(dj)

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

	return beginTurn(s), facts
}

func playingDJ(s State) (account.ID, bool) {
	p, ok := s.Phase.(Playing)
	if !ok {
		return 0, false
	}

	return p.Play.DJ, true
}

package room

import (
	"slices"
	"time"

	"synqsong/internal/account"
)

func currentPlay(s State) (Playing, error) {
	p, ok := s.Phase.(Playing)
	if !ok {
		return Playing{}, ErrStalePlay
	}

	return p, nil
}

// reportedPlay returns the play a member's browser is reporting on. Guests cannot drive playback.
func reportedPlay(s State, role Role, token PlayToken, video VideoID) (Playing, error) {
	if !role.atLeast(RoleMember) {
		return Playing{}, ErrNotAllowed
	}

	p, err := currentPlay(s)
	if err != nil {
		return Playing{}, err
	}

	return p, p.Play.CheckReport(token, video)
}

func loaded(s State, role Role, cmd Loaded) (State, []Fact, error) {
	p, err := reportedPlay(s, role, cmd.Token, cmd.Video)
	if err != nil {
		return s, nil, err
	}

	if cmd.Duration <= 0 {
		return s, nil, invalid("song length", "report a length above zero")
	}

	if p.Play.Track.Duration == cmd.Duration {
		return s, nil, nil
	}

	p.Play.Track.Duration = cmd.Duration
	s.Phase = p

	return s, []Fact{LengthLearned{Item: p.Play.Track.Item, Duration: cmd.Duration}}, nil
}

func ended(s State, role Role, cmd Ended, now time.Time) (State, []Fact, error) {
	p, err := reportedPlay(s, role, cmd.Token, cmd.Video)
	if err != nil {
		return s, nil, err
	}

	if err := p.Play.CheckEnded(now); err != nil {
		return s, nil, err
	}

	next, facts := endTurn(s, p, EndFinished, now)

	return next, facts, nil
}

// unplayable keeps the DJ's turn: they try their next song, as in td-fm's retryCurrentDj.
// With the DJ asleep nobody else would report, so listeners may, and the turn passes on.
func unplayable(s State, by account.ID, role Role, cmd Unplayable, now time.Time) (State, []Fact, error) {
	p, err := reportedPlay(s, role, cmd.Token, cmd.Video)
	if err != nil {
		return s, nil, err
	}

	if isAsleep(s, p.Play.DJ) {
		next, facts := endTurn(s, p, EndUnplayable, now)

		return next, facts, nil
	}

	if by != p.Play.DJ {
		return s, nil, ErrNotAllowed
	}

	s.Phase = Choosing{DJ: p.Play.DJ, Failed: append(slices.Clone(p.Failed), p.Play.Track.Item)}

	return s, []Fact{Finished{Play: p.Play, Reason: EndUnplayable}}, nil
}

func skip(s State, by account.ID, role Role, cmd Skip, now time.Time) (State, []Fact, error) {
	p, err := currentPlay(s)
	if err != nil {
		return s, nil, err
	}

	if cmd.Token != p.Play.Token {
		return s, nil, ErrStalePlay
	}

	ownSong := by == p.Play.DJ && Can(role, s.Settings, ActionSkipOwnSong)
	if !ownSong && !Can(role, s.Settings, ActionSkipSong) {
		return s, nil, ErrNotAllowed
	}

	next, facts := endTurn(s, p, EndSkipped, now)

	return next, facts, nil
}

func voteSkip(s State, by account.ID, role Role, cmd VoteSkip, now time.Time) (State, []Fact, error) {
	if !Can(role, s.Settings, ActionVoteSkip) {
		return s, nil, ErrNotAllowed
	}

	p, err := currentPlay(s)
	if err != nil {
		return s, nil, err
	}

	if cmd.Token != p.Play.Token {
		return s, nil, ErrStalePlay
	}

	if by == p.Play.DJ {
		return s, nil, ErrNotAllowed
	}

	if slices.Contains(p.Votes, by) {
		return s, nil, nil
	}

	p.Votes = append(slices.Clone(p.Votes), by)

	if len(p.Votes) >= s.Settings.SkipVotes.VotesNeeded(eligibleVoters(s, p.Play.DJ)) {
		next, facts := endTurn(s, p, EndVotedOut, now)

		return next, facts, nil
	}

	s.Phase = p

	return s, nil, nil
}

// eligibleVoters counts connected members other than the playing DJ. Guests never vote.
func eligibleVoters(s State, dj account.ID) int {
	n := 0
	for id, role := range s.Roles {
		if id != dj && role.atLeast(RoleMember) && isHere(s, id) {
			n++
		}
	}

	return n
}

// endTurn finishes the play and hands the stage to the next DJ, as in td-fm's
// endAndRotate. An asleep DJ's set ends here instead of going round again.
func endTurn(s State, p Playing, reason EndReason, now time.Time) (State, []Fact) {
	s.Booth = s.Booth.FinishTurn(isAsleep(s, p.Play.DJ))
	s = sweepIfGone(s, p.Play.DJ)

	return beginTurn(s, now), []Fact{Finished{Play: p.Play, Reason: reason}}
}

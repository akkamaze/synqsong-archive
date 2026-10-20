package room

import (
	"maps"
	"time"

	"synqsong/internal/account"
)

// The least time between one member's messages, kept per member rather than per connection.
const chatThrottle = 500 * time.Millisecond

func say(s State, by account.ID, role Role, cmd Say, now time.Time) (State, []Fact, error) {
	if !Can(role, s.Settings, ActionChat) {
		return s, nil, ErrNotAllowed
	}

	if last, ok := s.LastSaid[by]; ok && now.Sub(last) < chatThrottle {
		return s, nil, ErrTooFast
	}

	s.LastSaid = maps.Clone(s.LastSaid)
	if s.LastSaid == nil {
		s.LastSaid = map[account.ID]time.Time{}
	}
	s.LastSaid[by] = now

	return s, []Fact{Said{By: by, Body: cmd.Body, At: now}}, nil
}

package room

import (
	"time"

	"github.com/akkamaze/synqsong/internal/account"
)

// Restored prepares a room loaded after a restart. Nobody is connected yet, so
// every DJ in the booth is away since the room was last saved, as td-fm counted
// restored DJs' grace from when the server was last alive. Without this, a DJ
// who left during the downtime would still get their turn.
func Restored(s State, savedAt time.Time) State {
	s.Presence = map[account.ID]Presence{}

	for _, dj := range s.Booth.DJs() {
		s.Presence[dj] = Away{Since: savedAt}
	}

	return s
}

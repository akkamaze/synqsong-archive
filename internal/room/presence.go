package room

import "time"

// awayGrace is how long a member who went quiet keeps their place.
const awayGrace = 15 * time.Minute

// Presence is whether a member is in the room right now. Exactly one of Here, Away or Asleep.
//
//sumtype:decl
type Presence interface{ isPresence() }

// Here has at least one open connection.
type Here struct {
	Conns int
}

// Away went quiet without saying goodbye and keeps their place for a grace period.
type Away struct {
	Since time.Time
}

// Asleep is gone but still holds a DJ slot.
type Asleep struct {
	Since time.Time
}

func (Here) isPresence()   {}
func (Away) isPresence()   {}
func (Asleep) isPresence() {}

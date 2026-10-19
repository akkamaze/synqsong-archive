package room

import (
	"slices"

	"github.com/akkamaze/synqsong/internal/account"
)

// Booth is the DJ rotation. The head is the DJ playing now or up next.
// Every change returns a new Booth and leaves the receiver untouched.
type Booth struct {
	order []account.ID
	// headLeaving: the head stepped down mid-song and leaves once it ends.
	headLeaving bool
}

// BoothOf rebuilds a saved booth: DJs in turn order, and whether the head leaves after their song.
func BoothOf(djs []account.ID, headLeaving bool) Booth {
	return Booth{order: slices.Clone(djs), headLeaving: headLeaving && len(djs) > 0}
}

func (b Booth) DJs() []account.ID {
	return slices.Clone(b.order)
}

func (b Booth) Head() (account.ID, bool) {
	if len(b.order) == 0 {
		return 0, false
	}

	return b.order[0], true
}

func (b Booth) HeadLeaving() bool {
	return b.headLeaving
}

func (b Booth) Has(dj account.ID) bool {
	return slices.Contains(b.order, dj)
}

func (b Booth) isHead(dj account.ID) bool {
	head, ok := b.Head()

	return ok && head == dj
}

// StepUp joins the end of the rotation. A head who stepped down mid-song
// and steps up again stays on instead.
func (b Booth) StepUp(dj account.ID) Booth {
	if b.isHead(dj) {
		return Booth{order: slices.Clone(b.order)}
	}

	if b.Has(dj) {
		return b.clone()
	}

	return Booth{order: append(slices.Clone(b.order), dj), headLeaving: b.headLeaving}
}

// StepDown lets a DJ playing right now finish the song; anyone else leaves at once.
func (b Booth) StepDown(dj account.ID, playingNow bool) Booth {
	if b.isHead(dj) && playingNow {
		return Booth{order: slices.Clone(b.order), headLeaving: true}
	}

	return b.Remove(dj)
}

// Remove takes a DJ out of the rotation at once.
func (b Booth) Remove(dj account.ID) Booth {
	i := slices.Index(b.order, dj)
	if i < 0 {
		return b.clone()
	}

	next := Booth{order: slices.Delete(slices.Clone(b.order), i, i+1), headLeaving: b.headLeaving}
	if i == 0 {
		next.headLeaving = false
	}

	return next
}

// FinishTurn ends the head's turn: back of the line, or out if they stepped
// down or dropHead says they are gone.
func (b Booth) FinishTurn(dropHead bool) Booth {
	if len(b.order) == 0 {
		return Booth{}
	}

	rest := slices.Clone(b.order[1:])
	if dropHead || b.headLeaving {
		return Booth{order: rest}
	}

	return Booth{order: append(rest, b.order[0])}
}

// RotatePast moves a head with nothing to play to the back of the line. It never drops them.
func (b Booth) RotatePast() Booth {
	if len(b.order) == 0 {
		return Booth{}
	}

	return Booth{order: append(slices.Clone(b.order[1:]), b.order[0])}
}

func (b Booth) clone() Booth {
	return Booth{order: slices.Clone(b.order), headLeaving: b.headLeaving}
}

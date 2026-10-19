package room

import (
	"time"

	"github.com/akkamaze/tutti/internal/account"
)

// Input is everything Apply reacts to.
//
//sumtype:decl
type Input interface{ isInput() }

// Request is a command from someone in the room.
type Request struct {
	By  account.ID
	Cmd Command
}

// TrackChosen answers Choosing with the first song in that DJ's queue, if any.
type TrackChosen struct {
	DJ    account.ID
	Track Track
	Found bool
}

func (Request) isInput()     {}
func (TrackChosen) isInput() {}

// Command is what a person asks the room to do.
//
//sumtype:decl
type Command interface{ isCommand() }

type StepUp struct{}

type StepDown struct{}

type KickFromBooth struct {
	DJ account.ID
}

// Loaded reports the length a browser's player found for the current play.
type Loaded struct {
	Token    PlayToken
	Video    VideoID
	Duration time.Duration
}

// Ended reports that a browser's player reached the end of the current play.
type Ended struct {
	Token PlayToken
	Video VideoID
}

// Unplayable reports that the current play's video will not play.
type Unplayable struct {
	Token PlayToken
	Video VideoID
}

// Skip and VoteSkip name the play they mean, so a second click cannot skip the next song.
type Skip struct {
	Token PlayToken
}

type VoteSkip struct {
	Token PlayToken
}

func (StepUp) isCommand()        {}
func (StepDown) isCommand()      {}
func (KickFromBooth) isCommand() {}
func (Loaded) isCommand()        {}
func (Ended) isCommand()         {}
func (Unplayable) isCommand()    {}
func (Skip) isCommand()          {}
func (VoteSkip) isCommand()      {}

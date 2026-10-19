package room

import "github.com/akkamaze/tutti/internal/account"

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

func (StepUp) isCommand()        {}
func (StepDown) isCommand()      {}
func (KickFromBooth) isCommand() {}

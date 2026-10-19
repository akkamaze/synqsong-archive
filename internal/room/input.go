package room

import (
	"time"

	"synqsong/internal/account"
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

// Connected: one more connection from this member opened.
type Connected struct {
	Who account.ID
}

// Disconnected: one of this member's connections closed. Goodbye means the
// page said it was leaving; SignOut means they pressed Leave.
type Disconnected struct {
	Who     account.ID
	Goodbye bool
	SignOut bool
}

// Admit makes someone a member after they entered the room's passcode.
type Admit struct {
	Who account.ID
}

func (Request) isInput()      {}
func (TrackChosen) isInput()  {}
func (Connected) isInput()    {}
func (Disconnected) isInput() {}

// Tick lets time pass: rests end, fallbacks fire and away members are counted out.
type Tick struct{}

func (Admit) isInput() {}
func (Tick) isInput()  {}

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

type Say struct {
	Body ChatBody
}

// RemoveFromRoom takes a member out of the room and keeps them out.
type RemoveFromRoom struct {
	Who account.ID
}

type AssignRole struct {
	Who  account.ID
	Role Role
}

type ChangeSettings struct {
	Settings Settings
}

func (StepUp) isCommand()         {}
func (Say) isCommand()            {}
func (RemoveFromRoom) isCommand() {}
func (AssignRole) isCommand()     {}
func (ChangeSettings) isCommand() {}
func (StepDown) isCommand()       {}
func (KickFromBooth) isCommand()  {}
func (Loaded) isCommand()         {}
func (Ended) isCommand()          {}
func (Unplayable) isCommand()     {}
func (Skip) isCommand()           {}
func (VoteSkip) isCommand()       {}

package room

// Action is something a member asks to do in a room.
type Action int8

const (
	ActionChat Action = iota + 1
	ActionJoinBooth
	ActionSkipOwnSong
	ActionVoteSkip
	ActionSkipSong
	ActionKickFromBooth
	ActionRemoveFromRoom
	ActionChangePasscode
	ActionAssignRole
	ActionChangeSettings
	ActionDeleteRoom
)

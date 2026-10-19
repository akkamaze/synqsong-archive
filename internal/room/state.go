package room

import "github.com/akkamaze/tutti/internal/account"

// State is the whole truth about one running room.
type State struct {
	Settings  Settings
	Roles     map[account.ID]Role
	Presence  map[account.ID]Presence
	Booth     Booth
	Phase     Phase
	LastToken PlayToken
}

func NewState(owner account.ID) State {
	return State{
		Settings: DefaultSettings(),
		Roles:    map[account.ID]Role{owner: RoleOwner},
		Presence: map[account.ID]Presence{},
		Phase:    Idle{},
	}
}

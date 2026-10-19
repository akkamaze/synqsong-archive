package room

import (
	"maps"
	"time"

	"github.com/akkamaze/synqsong/internal/account"
)

func admit(s State, in Admit) (State, []Fact, error) {
	if _, banned := s.Banned[in.Who]; banned {
		return s, nil, ErrBanned
	}

	if _, member := s.Roles[in.Who]; member {
		return s, nil, nil
	}

	s.Roles = maps.Clone(s.Roles)
	s.Roles[in.Who] = RoleMember

	return s, nil, nil
}

func removeFromRoom(s State, by account.ID, role Role, cmd RemoveFromRoom, now time.Time) (State, []Fact, error) {
	if !Can(role, s.Settings, ActionRemoveFromRoom) {
		return s, nil, ErrNotAllowed
	}

	if cmd.Who == by {
		return s, nil, invalid("member", "the owner cannot remove themselves")
	}

	if _, member := s.Roles[cmd.Who]; !member {
		return s, nil, nil
	}

	s, facts := removeFromBooth(s, cmd.Who, now)
	s = removePresence(s, cmd.Who)

	s.Roles = maps.Clone(s.Roles)
	delete(s.Roles, cmd.Who)

	s.LastSaid = maps.Clone(s.LastSaid)
	delete(s.LastSaid, cmd.Who)

	s.Banned = maps.Clone(s.Banned)
	if s.Banned == nil {
		s.Banned = map[account.ID]struct{}{}
	}
	s.Banned[cmd.Who] = struct{}{}

	return s, append(facts, Removed(cmd)), nil
}

// assignRole lets the owner make a member a DJ or back. Ownership transfer comes later.
func assignRole(s State, role Role, cmd AssignRole) (State, []Fact, error) {
	if !Can(role, s.Settings, ActionAssignRole) {
		return s, nil, ErrNotAllowed
	}

	current, member := s.Roles[cmd.Who]
	if !member {
		return s, nil, ErrNotMember
	}

	if current == RoleOwner || current == RoleGuest {
		return s, nil, invalid("role", "only members and DJs can change role")
	}

	if cmd.Role != RoleMember && cmd.Role != RoleDJ {
		return s, nil, invalid("role", "choose member or DJ")
	}

	s.Roles = maps.Clone(s.Roles)
	s.Roles[cmd.Who] = cmd.Role

	return s, nil, nil
}

func changeSettings(s State, role Role, cmd ChangeSettings) (State, []Fact, error) {
	if !Can(role, s.Settings, ActionChangeSettings) {
		return s, nil, ErrNotAllowed
	}

	if err := cmd.Settings.validate(); err != nil {
		return s, nil, err
	}

	s.Settings = cmd.Settings

	return s, nil, nil
}

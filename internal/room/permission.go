package room

// Can reports whether a member with this role may take the action under these settings.
func Can(role Role, settings Settings, action Action) bool {
	if !role.atLeast(RoleMember) {
		return false
	}

	switch action {
	case ActionChat, ActionJoinBooth, ActionSkipOwnSong:
		return true

	case ActionSkipSong:
		return role.atLeast(RoleDJ) || settings.Skip == SkipByEveryone

	case ActionKickFromBooth:
		return role.atLeast(RoleDJ)

	case ActionRemoveFromRoom, ActionChangePasscode, ActionAssignRole, ActionChangeSettings, ActionDeleteRoom:
		return role == RoleOwner
	}

	return false
}

package room

import "testing"

type allowed struct {
	member, dj, owner bool
}

// The permission table the owner agreed on 2026-10-19 (revised: only the owner kicks, vote skip, guests).
var permissionTable = map[SkipPolicy]map[Action]allowed{
	SkipByDJs: {
		ActionChat:           {member: true, dj: true, owner: true},
		ActionJoinBooth:      {member: true, dj: true, owner: true},
		ActionSkipOwnSong:    {member: true, dj: true, owner: true},
		ActionVoteSkip:       {member: true, dj: true, owner: true},
		ActionSkipSong:       {member: false, dj: true, owner: true},
		ActionKickFromBooth:  {owner: true},
		ActionRemoveFromRoom: {owner: true},
		ActionChangePasscode: {owner: true},
		ActionAssignRole:     {owner: true},
		ActionChangeSettings: {owner: true},
		ActionDeleteRoom:     {owner: true},
	},
	SkipByEveryone: {
		ActionChat:           {member: true, dj: true, owner: true},
		ActionJoinBooth:      {member: true, dj: true, owner: true},
		ActionSkipOwnSong:    {member: true, dj: true, owner: true},
		ActionVoteSkip:       {member: true, dj: true, owner: true},
		ActionSkipSong:       {member: true, dj: true, owner: true},
		ActionKickFromBooth:  {owner: true},
		ActionRemoveFromRoom: {owner: true},
		ActionChangePasscode: {owner: true},
		ActionAssignRole:     {owner: true},
		ActionChangeSettings: {owner: true},
		ActionDeleteRoom:     {owner: true},
	},
}

func TestCanMatchesAgreedTable(t *testing.T) {
	for policy, actions := range permissionTable {
		settings := Settings{Skip: policy, SkipVotes: DefaultSkipVotes}

		for action := ActionChat; action <= ActionDeleteRoom; action++ {
			want, listed := actions[action]
			if !listed {
				t.Fatalf("action %d is missing from the table for policy %d", action, policy)
			}

			check := func(role Role, want bool) {
				if got := Can(role, settings, action); got != want {
					t.Errorf("Can(role %d, policy %d, action %d) = %v, want %v", role, policy, action, got, want)
				}
			}

			check(RoleMember, want.member)
			check(RoleDJ, want.dj)
			check(RoleOwner, want.owner)
		}
	}
}

func TestDefaultSettingsLetOnlyDJsSkip(t *testing.T) {
	if Can(RoleMember, DefaultSettings(), ActionSkipSong) {
		t.Fatal("a member can skip others' songs under default settings, want only DJs and the owner")
	}
}

func TestGuestsOnlyListen(t *testing.T) {
	everyone := Settings{Skip: SkipByEveryone, SkipVotes: DefaultSkipVotes}

	for action := ActionChat; action <= ActionDeleteRoom; action++ {
		if Can(RoleGuest, everyone, action) {
			t.Errorf("Can(guest, action %d) = true, want guests to only listen", action)
		}
	}
}

func TestInvalidRoleCanDoNothing(t *testing.T) {
	for _, role := range []Role{0, RoleOwner + 1, -1} {
		for action := ActionChat; action <= ActionDeleteRoom; action++ {
			if Can(role, Settings{Skip: SkipByEveryone}, action) {
				t.Errorf("Can(invalid role %d, action %d) = true, want false", role, action)
			}
		}
	}
}

func TestUnknownActionIsDenied(t *testing.T) {
	if Can(RoleOwner, DefaultSettings(), ActionDeleteRoom+1) {
		t.Fatal("an unknown action was allowed, want denied")
	}
}

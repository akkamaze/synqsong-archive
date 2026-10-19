package room

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"synqsong/internal/account"
)

const newcomer account.ID = 200

func TestAdmitMakesANewMember(t *testing.T) {
	s, _ := must(t, roomWith(), Admit{Who: newcomer})

	if got := s.Roles[newcomer]; got != RoleMember {
		t.Fatalf("admitted role = %d, want RoleMember", got)
	}

	again, _ := must(t, s, Admit{Who: newcomer})
	if !reflect.DeepEqual(again, s) {
		t.Fatal("admitting a member twice changed the room")
	}
}

func TestAdmitKeepsAnExistingRole(t *testing.T) {
	s, _ := must(t, roomWith(), Admit{Who: owner})

	if got := s.Roles[owner]; got != RoleOwner {
		t.Fatalf("owner re-admitted as role %d, want RoleOwner kept", got)
	}
}

func TestOwnerRemovesSomeoneForGood(t *testing.T) {
	s, play := annPlaying()

	s, facts := must(t, s, Request{By: owner, Cmd: RemoveFromRoom{Who: ann}})

	if _, ok := s.Roles[ann]; ok {
		t.Fatal("removed member still has a role")
	}

	if _, ok := s.Presence[ann]; ok {
		t.Fatal("removed member is still present")
	}

	assertOrder(t, s.Booth, bob)
	choosing(t, s, bob)

	want := []Fact{Finished{Play: play, Reason: EndFinished}, Removed{Who: ann}}
	if !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}

	if _, _, err := Apply(s, Admit{Who: ann}, t0); !errors.Is(err, ErrBanned) {
		t.Fatalf("removed member re-admitted: %v, want ErrBanned", err)
	}
}

func TestOnlyTheOwnerRemovesAndNeverThemselves(t *testing.T) {
	s := roomWith()
	s.Roles[bob] = RoleDJ

	if _, _, err := Apply(s, Request{By: bob, Cmd: RemoveFromRoom{Who: cat}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("DJ removed someone: %v, want ErrNotAllowed", err)
	}

	if _, _, err := Apply(s, Request{By: owner, Cmd: RemoveFromRoom{Who: owner}}, t0); err == nil {
		t.Fatal("owner removed themselves, want refused")
	}
}

func TestOwnerAssignsDJAndMemberOnly(t *testing.T) {
	s, _ := must(t, roomWith(), Request{By: owner, Cmd: AssignRole{Who: cat, Role: RoleDJ}})

	if got := s.Roles[cat]; got != RoleDJ {
		t.Fatalf("cat role = %d, want RoleDJ", got)
	}

	refused := []AssignRole{
		{Who: cat, Role: RoleOwner},
		{Who: cat, Role: RoleGuest},
		{Who: owner, Role: RoleMember},
		{Who: 12345, Role: RoleDJ},
	}

	for _, cmd := range refused {
		if _, _, err := Apply(s, Request{By: owner, Cmd: cmd}, t0); err == nil {
			t.Errorf("AssignRole%+v accepted, want refused", cmd)
		}
	}

	if _, _, err := Apply(s, Request{By: cat, Cmd: AssignRole{Who: bob, Role: RoleDJ}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("DJ assigned a role: %v, want ErrNotAllowed", err)
	}
}

func TestOwnerChangesSettings(t *testing.T) {
	want := Settings{Skip: SkipByEveryone, SkipVotes: 30}

	s, _ := must(t, roomWith(), Request{By: owner, Cmd: ChangeSettings{Settings: want}})

	if s.Settings != want {
		t.Fatalf("settings = %+v, want %+v", s.Settings, want)
	}

	for _, bad := range []Settings{{Skip: 0, SkipVotes: 50}, {Skip: SkipByDJs, SkipVotes: 0}} {
		if _, _, err := Apply(s, Request{By: owner, Cmd: ChangeSettings{Settings: bad}}, t0); err == nil {
			t.Errorf("settings %+v accepted, want refused", bad)
		}
	}

	if _, _, err := Apply(s, Request{By: bob, Cmd: ChangeSettings{Settings: want}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("member changed settings: %v, want ErrNotAllowed", err)
	}
}

func TestChatIsRecordedAndThrottled(t *testing.T) {
	s := roomWith()

	s, facts, err := Apply(s, Request{By: cat, Cmd: Say{Body: "hi"}}, t0)
	if err != nil {
		t.Fatalf("say: %v", err)
	}

	if want := []Fact{Said{By: cat, Body: "hi", At: t0}}; !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}

	if _, _, err := Apply(s, Request{By: cat, Cmd: Say{Body: "again"}}, t0.Add(499*time.Millisecond)); !errors.Is(err, ErrTooFast) {
		t.Fatalf("second line after 499ms: %v, want ErrTooFast", err)
	}

	if _, _, err := Apply(s, Request{By: cat, Cmd: Say{Body: "again"}}, t0.Add(500*time.Millisecond)); err != nil {
		t.Fatalf("second line after 500ms: %v, want accepted", err)
	}

	if _, _, err := Apply(s, Request{By: bob, Cmd: Say{Body: "me too"}}, t0.Add(time.Millisecond)); err != nil {
		t.Fatalf("another member's line right after: %v, want accepted (throttle is per member)", err)
	}
}

func TestGuestsCannotChat(t *testing.T) {
	if _, _, err := Apply(roomWith(), Request{By: guest, Cmd: Say{Body: "hi"}}, t0); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("guest chat: %v, want ErrNotAllowed", err)
	}
}

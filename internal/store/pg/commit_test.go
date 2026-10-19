package pg

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/akkamaze/synqsong/internal/account"
	"github.com/akkamaze/synqsong/internal/room"
	"github.com/akkamaze/synqsong/internal/roomhost"
)

var t0 = time.UnixMilli(1_790_000_000_000).UTC()

func load(t *testing.T, s *Store, id room.ID) (room.State, roomhost.Rev) {
	t.Helper()

	state, rev, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("load room %d: %v", id, err)
	}

	return state, rev
}

func commit(t *testing.T, s *Store, id room.ID, rev roomhost.Rev, before, next room.State, facts ...room.Fact) {
	t.Helper()

	if err := s.Commit(context.Background(), id, rev, before, next, facts); err != nil {
		t.Fatalf("commit at rev %d: %v", rev, err)
	}
}

// editable copies a loaded state so changing it never reaches the original, as room.Apply
// never does, and drops what Load derives instead of reading so states can be compared.
func editable(s room.State) room.State {
	s.Roles = maps.Clone(s.Roles)
	s.Banned = maps.Clone(s.Banned)
	s.Presence = nil
	s.LastSaid = nil

	return s
}

func TestEveryPhaseRoundTrips(t *testing.T) {
	s := needDB(t)

	phases := func(f fixture, item room.QueueItemID) []room.Phase {
		return []room.Phase{
			room.Idle{},
			room.Choosing{DJ: f.ann, Failed: []room.QueueItemID{item}, Passed: 1},
			room.Playing{
				Play: room.Play{
					Token: room.PlayToken(t0.UnixMilli()),
					Track: room.Track{Item: item, Video: "aaaaaaaaaaa", Title: "One", Channel: "Ch", Thumbnail: "https://i.ytimg.com/x.jpg", Duration: 200 * time.Second},
					DJ:    f.ann,
				},
				Failed: []room.QueueItemID{item},
				Votes:  []account.ID{f.bob},
			},
			room.Playing{Play: room.Play{Token: 7, Track: room.Track{Item: item, Video: "bbbbbbbbbbb", Title: "Unknown length"}, DJ: f.ann}},
			room.Resting{DJ: f.ann, Reason: room.RestUnplayable, Until: t0.Add(3 * time.Second)},
		}
	}

	f := newRoom(t)
	item := addQueueItem(t, f.ann, "aaaaaaaaaaa", 1)

	for _, phase := range phases(f, item) {
		before, rev := load(t, s, f.id)

		next := editable(before)
		next.Roles = map[account.ID]room.Role{f.owner: room.RoleOwner, f.ann: room.RoleDJ, f.bob: room.RoleMember}
		next.Booth = room.BoothOf([]account.ID{f.ann, f.bob}, true)
		next.Settings = room.Settings{Skip: room.SkipByEveryone, SkipVotes: 30}
		next.LastToken = room.PlayToken(t0.UnixMilli())
		next.Phase = phase

		commit(t, s, f.id, rev, before, next)

		got, gotRev := load(t, s, f.id)
		if gotRev != rev+1 {
			t.Fatalf("rev after commit = %d, want %d", gotRev, rev+1)
		}

		if !reflect.DeepEqual(editable(got), editable(next)) {
			t.Fatalf("phase %T did not round-trip:\n got  %+v\n want %+v", phase, editable(got), editable(next))
		}
	}
}

func TestLoadMarksBoothDJsAwaySinceTheSave(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles = map[account.ID]room.Role{f.owner: room.RoleOwner, f.ann: room.RoleMember}
	next.Booth = room.BoothOf([]account.ID{f.ann}, false)
	commit(t, s, f.id, rev, before, next)

	got, _ := load(t, s, f.id)

	if _, away := got.Presence[f.ann].(room.Away); !away {
		t.Fatalf("restored DJ presence = %#v, want Away", got.Presence[f.ann])
	}
}

func TestCommitIsFencedOnTheRev(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	before, rev := load(t, s, f.id)

	next := editable(before)
	next.Settings.SkipVotes = 40
	commit(t, s, f.id, rev, before, next)

	err := s.Commit(context.Background(), f.id, rev, before, next, nil)
	if !errors.Is(err, roomhost.ErrFenced) {
		t.Fatalf("second commit on the old rev: %v, want ErrFenced", err)
	}
}

func TestAFinishedPlayIsRecordedOnceAndRotatedOnce(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	first := addQueueItem(t, f.ann, "aaaaaaaaaaa", 1)
	second := addQueueItem(t, f.ann, "bbbbbbbbbbb", 2)

	play := room.Play{Token: 42, Track: room.Track{Item: first, Video: "aaaaaaaaaaa", Title: "song aaaaaaaaaaa"}, DJ: f.ann}
	finished := room.Finished{Play: play, Reason: room.EndSkipped}

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleMember
	commit(t, s, f.id, rev, before, next, finished)

	again, rev := load(t, s, f.id)
	commit(t, s, f.id, rev, again, editable(again), finished)

	ctx := context.Background()

	var plays int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM play_history WHERE room_id = $1 AND play_token = 42`, int64(f.id)).Scan(&plays); err != nil {
		t.Fatalf("count plays: %v", err)
	}

	if plays != 1 {
		t.Fatalf("history rows for one play saved twice = %d, want 1", plays)
	}

	head, found, err := s.Head(ctx, f.ann)
	if err != nil || !found || head.Item != second {
		t.Fatalf("queue head after the play = %+v, %v, %v; want the second song, the first moved down once", head, found, err)
	}
}

func TestLengthsAndChatAreSaved(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	item := addQueueItem(t, f.ann, "ccccccccccc", 1)

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleMember
	commit(t, s, f.id, rev, before, next,
		room.LengthLearned{Item: item, Duration: 3 * time.Minute},
		room.Said{By: f.ann, Body: "เพลงนี้เพราะ", At: t0})

	head, _, err := s.Head(context.Background(), f.ann)
	if err != nil || head.Duration != 3*time.Minute {
		t.Fatalf("queue item length = %v (%v), want 3m remembered", head.Duration, err)
	}

	var body string
	if err := testPool.QueryRow(context.Background(), `SELECT body FROM chat_messages WHERE room_id = $1`, int64(f.id)).Scan(&body); err != nil || body != "เพลงนี้เพราะ" {
		t.Fatalf("chat body = %q (%v), want the line saved", body, err)
	}
}

func TestARemovedMemberLosesTheirSeatAndIsBanned(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	before, rev := load(t, s, f.id)
	seated := editable(before)
	seated.Roles = map[account.ID]room.Role{f.owner: room.RoleOwner, f.ann: room.RoleMember, f.bob: room.RoleMember}
	seated.Booth = room.BoothOf([]account.ID{f.ann, f.bob}, false)
	commit(t, s, f.id, rev, before, seated)

	before, rev = load(t, s, f.id)
	removed := editable(before)
	delete(removed.Roles, f.ann)
	removed.Booth = room.BoothOf([]account.ID{f.bob}, false)
	removed.Banned = map[account.ID]struct{}{f.ann: {}}
	commit(t, s, f.id, rev, before, removed, room.Removed{Who: f.ann})

	got, _ := load(t, s, f.id)

	if _, member := got.Roles[f.ann]; member {
		t.Fatal("removed member is still a member after reload")
	}

	if _, banned := got.Banned[f.ann]; !banned {
		t.Fatal("removed member is not banned after reload")
	}

	if djs := got.Booth.DJs(); !reflect.DeepEqual(djs, []account.ID{f.bob}) {
		t.Fatalf("booth after reload = %v, want only bob", djs)
	}
}

func TestAFailingStepRollsBackTheWholeCommit(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Settings.SkipVotes = 90
	next.Roles[f.ann] = room.RoleMember

	// The domain caps chat at 300 characters; the table's CHECK refuses this line.
	tooLong := room.Said{By: f.ann, Body: room.ChatBody(strings.Repeat("x", 301)), At: t0}

	if err := s.Commit(context.Background(), f.id, rev, before, next, []room.Fact{tooLong}); err == nil {
		t.Fatal("commit with a line the table refuses succeeded")
	}

	got, gotRev := load(t, s, f.id)

	if gotRev != rev || got.Settings.SkipVotes != before.Settings.SkipVotes {
		t.Fatalf("after a failed commit: rev %d, skip votes %d; want rev %d and votes %d unchanged", gotRev, got.Settings.SkipVotes, rev, before.Settings.SkipVotes)
	}

	if _, member := got.Roles[f.ann]; member {
		t.Fatal("a member added in the failed commit was kept")
	}
}

func TestLoadingAMissingRoomSaysSo(t *testing.T) {
	s := needDB(t)

	if _, _, err := s.Load(context.Background(), 999_999_999); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("load of a missing room: %v, want ErrRoomNotFound", err)
	}
}

func TestARoleChangeIsSaved(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleDJ
	commit(t, s, f.id, rev, before, next)

	got, _ := load(t, s, f.id)
	if got.Roles[f.ann] != room.RoleDJ {
		t.Fatalf("ann's role after reload = %d, want RoleDJ", got.Roles[f.ann])
	}
}

func TestASongTheDJMovedOffTheTopStaysWhereTheyPutIt(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	playing := addQueueItem(t, f.ann, "aaaaaaaaaaa", 2)
	first := addQueueItem(t, f.ann, "bbbbbbbbbbb", 1)
	addQueueItem(t, f.ann, "ccccccccccc", 3)

	play := room.Play{Token: 77, Track: room.Track{Item: playing, Video: "aaaaaaaaaaa", Title: "song aaaaaaaaaaa"}, DJ: f.ann}

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleMember
	commit(t, s, f.id, rev, before, next, room.Finished{Play: play, Reason: room.EndFinished})

	var position int64
	if err := testPool.QueryRow(context.Background(), `SELECT position FROM queue_items WHERE id = $1`, int64(playing)).Scan(&position); err != nil {
		t.Fatalf("read position: %v", err)
	}

	if position != 2 {
		t.Fatalf("played song moved to position %d, want it left at 2 where the DJ put it", position)
	}

	if head, _, _ := s.Head(context.Background(), f.ann); head.Item != first {
		t.Fatalf("queue head = %d, want the song the DJ moved to the top", head.Item)
	}
}

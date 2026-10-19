package pg

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"synqsong/internal/account"
	"synqsong/internal/queue"
	"synqsong/internal/room"
)

func trackOf(video string) room.Track {
	return room.Track{Video: room.VideoID(video), Title: "song " + video, Duration: 3 * time.Minute}
}

func videosOf(t *testing.T, s *Store, owner account.ID) []room.VideoID {
	t.Helper()

	tracks, err := s.Queue(context.Background(), owner)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}

	var ids []room.VideoID
	for _, tr := range tracks {
		ids = append(ids, tr.Video)
	}

	return ids
}

func add(t *testing.T, s *Store, owner account.ID, video string) room.Track {
	t.Helper()

	tr, err := s.Add(context.Background(), owner, trackOf(video))
	if err != nil {
		t.Fatalf("add %s: %v", video, err)
	}

	return tr
}

func TestANewSongGoesToTheTop(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	add(t, s, f.ann, "aaaaaaaaaaa")
	add(t, s, f.ann, "bbbbbbbbbbb")

	if got := videosOf(t, s, f.ann); !slices.Equal(got, []room.VideoID{"bbbbbbbbbbb", "aaaaaaaaaaa"}) {
		t.Fatalf("queue = %v, want the newest first", got)
	}
}

func TestANewSongGoesAfterTheOneThisDJIsPlaying(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	ctx := context.Background()

	playing := add(t, s, f.ann, "aaaaaaaaaaa")
	add(t, s, f.ann, "bbbbbbbbbbb")

	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleMember
	next.Booth = room.BoothOf([]account.ID{f.ann}, false)
	next.Phase = room.Playing{Play: room.Play{Token: 5, Track: playing, DJ: f.ann}}
	commit(t, s, f.id, rev, before, next)

	if _, err := s.Add(ctx, f.ann, trackOf("ccccccccccc")); err != nil {
		t.Fatalf("add while playing: %v", err)
	}

	if got := videosOf(t, s, f.ann); !slices.Equal(got, []room.VideoID{"bbbbbbbbbbb", "aaaaaaaaaaa", "ccccccccccc"}) {
		t.Fatalf("queue = %v, want the new song right after the playing one", got)
	}
}

func TestTheSameVideoCannotBeQueuedTwiceEvenAtOnce(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		added   int
		refused int
	)

	for range 8 {
		wg.Go(func() {
			_, err := s.Add(context.Background(), f.ann, trackOf("aaaaaaaaaaa"))

			mu.Lock()
			defer mu.Unlock()

			switch {
			case err == nil:
				added++
			case errors.Is(err, queue.ErrAlreadyQueued):
				refused++
			default:
				t.Errorf("add: %v", err)
			}
		})
	}
	wg.Wait()

	if added != 1 || refused != 7 {
		t.Fatalf("8 adds of one video at once: %d added, %d refused; want 1 and 7", added, refused)
	}
}

func TestLosingAnAddRaceStillPointsAtTheQueuedSong(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	first := add(t, s, f.ann, "aaaaaaaaaaa")

	_, err := s.Add(context.Background(), f.ann, trackOf("aaaaaaaaaaa"))

	var already *queue.AlreadyQueuedError
	if !errors.As(err, &already) || already.Track != first {
		t.Fatalf("second add = %v; want AlreadyQueuedError with %+v", err, first)
	}
}

func TestAnEmptyQueueIsAnEmptyList(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	if q, err := s.Queue(context.Background(), f.ann); err != nil || q == nil {
		t.Fatalf("empty queue = %#v, %v; want an empty list, which a screen shows as []", q, err)
	}
}

func TestTheRoomRotatingAQueueWhileItsOwnerAddsSongsLosesNothing(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	ctx := context.Background()

	for i := range 5 {
		add(t, s, f.ann, fmt.Sprintf("seed%07d", i))
	}

	// While Ann plays, each add shifts the songs after hers, racing the room's move to the bottom.
	head, _, _ := s.Head(ctx, f.ann)
	before, rev := load(t, s, f.id)
	next := editable(before)
	next.Roles[f.ann] = room.RoleMember
	next.Booth = room.BoothOf([]account.ID{f.ann}, false)
	next.Phase = room.Playing{Play: room.Play{Token: 1, Track: head, DJ: f.ann}}
	commit(t, s, f.id, rev, before, next)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	note := func(err error) {
		mu.Lock()
		defer mu.Unlock()

		errs = append(errs, err)
	}

	wg.Go(func() {
		for i := range 30 {
			head, _, err := s.Head(ctx, f.ann)
			if err != nil {
				note(err)

				continue
			}

			before, rev, err := s.Load(ctx, f.id)
			if err != nil {
				note(err)

				continue
			}

			play := room.Play{Token: room.PlayToken(100 + i), Track: head, DJ: f.ann}
			if err := s.Commit(ctx, f.id, rev, before, editable(before), []room.Fact{room.Finished{Play: play, Reason: room.EndFinished}}); err != nil {
				note(err)
			}
		}
	})

	wg.Go(func() {
		for i := range 30 {
			if _, err := s.Add(ctx, f.ann, trackOf(fmt.Sprintf("add%08d", i))); err != nil {
				note(err)
			}
		}
	})

	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("%d of 60 queue changes failed, first: %v", len(errs), errs[0])
	}

	if n := len(videosOf(t, s, f.ann)); n != 35 {
		t.Fatalf("queue holds %d songs, want 35", n)
	}
}

func TestQueuedFindsOnlyTheOwnersCopy(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	added := add(t, s, f.ann, "aaaaaaaaaaa")

	got, ok, err := s.Queued(context.Background(), f.ann, "aaaaaaaaaaa")
	if err != nil || !ok || got != added {
		t.Fatalf("Queued for the owner = %+v, %v, %v; want %+v", got, ok, err, added)
	}

	if _, ok, err := s.Queued(context.Background(), f.bob, "aaaaaaaaaaa"); err != nil || ok {
		t.Fatalf("Queued for someone else = %v, %v; want not queued", ok, err)
	}
}

func TestReorderTakesTheWholeQueueOrNothing(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)
	ctx := context.Background()

	a := add(t, s, f.ann, "aaaaaaaaaaa")
	b := add(t, s, f.ann, "bbbbbbbbbbb")
	c := add(t, s, f.ann, "ccccccccccc")

	if err := s.Reorder(ctx, f.ann, []room.QueueItemID{a.Item, b.Item, c.Item}); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	if got := videosOf(t, s, f.ann); !slices.Equal(got, []room.VideoID{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc"}) {
		t.Fatalf("queue after reorder = %v", got)
	}

	stale := [][]room.QueueItemID{
		{a.Item, b.Item},
		{a.Item, b.Item, c.Item, 999_999},
		{a.Item, a.Item, b.Item},
	}

	for _, order := range stale {
		if err := s.Reorder(ctx, f.ann, order); !errors.Is(err, queue.ErrStaleOrder) {
			t.Errorf("reorder %v: %v, want ErrStaleOrder", order, err)
		}
	}
}

func TestRemoveTouchesOnlyTheOwnersSongs(t *testing.T) {
	s := needDB(t)
	f := newRoom(t)

	mine := add(t, s, f.ann, "aaaaaaaaaaa")
	theirs := add(t, s, f.bob, "aaaaaaaaaaa")

	n, err := s.Remove(context.Background(), f.ann, []room.QueueItemID{mine.Item, theirs.Item})
	if err != nil || n != 1 {
		t.Fatalf("remove = %d, %v; want only ann's song removed", n, err)
	}

	if got := videosOf(t, s, f.bob); len(got) != 1 {
		t.Fatalf("bob's queue = %v, want untouched", got)
	}
}

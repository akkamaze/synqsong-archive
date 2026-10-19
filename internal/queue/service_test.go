package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"synqsong/internal/account"
	"synqsong/internal/catalog"
	"synqsong/internal/room"
)

type fakeFinder map[room.VideoID]catalog.Video

func (f fakeFinder) LookUp(_ context.Context, id room.VideoID) (catalog.Video, error) {
	v, ok := f[id]
	f[lookedUp] = catalog.Video{Title: string(id)}
	if !ok {
		return catalog.Video{}, catalog.ErrVideoNotFound
	}

	return v, nil
}

// lookedUp marks in a fakeFinder that YouTube was asked, which costs quota.
const lookedUp room.VideoID = "looked-up"

type recordingStore struct {
	added []room.Track
}

func (r *recordingStore) Queued(_ context.Context, _ account.ID, video room.VideoID) (room.Track, bool, error) {
	for _, t := range r.added {
		if t.Video == video {
			return t, true, nil
		}
	}

	return room.Track{}, false, nil
}

func (r *recordingStore) Queue(context.Context, account.ID) ([]room.Track, error) {
	return r.added, nil
}

func (r *recordingStore) Add(_ context.Context, _ account.ID, t room.Track) (room.Track, error) {
	r.added = append(r.added, t)

	return t, nil
}

func (r *recordingStore) Remove(context.Context, account.ID, []room.QueueItemID) (int, error) {
	return 0, nil
}

func (r *recordingStore) Reorder(context.Context, account.ID, []room.QueueItemID) error { return nil }

func TestAddLinkChecksTheVideoBeforeQueueing(t *testing.T) {
	finder := fakeFinder{
		"aaaaaaaaaaa": {ID: "aaaaaaaaaaa", Title: "Fine", Embeddable: true, Duration: 3 * time.Minute},
		"bbbbbbbbbbb": {ID: "bbbbbbbbbbb", Title: "Long", Embeddable: true, Duration: 11 * time.Minute},
		"ccccccccccc": {ID: "ccccccccccc", Title: "Blocked", Duration: time.Minute},
	}
	store := &recordingStore{}
	s := NewService(store, finder)

	got, err := s.AddLink(context.Background(), 1, "https://youtu.be/aaaaaaaaaaa?si=x")
	if err != nil || got.Title != "Fine" || got.Duration != 3*time.Minute {
		t.Fatalf("add a fine link = %+v, %v", got, err)
	}

	refused := map[string]error{
		"https://youtu.be/bbbbbbbbbbb": catalog.ErrTooLong,
		"ccccccccccc":                  catalog.ErrNotEmbeddable,
		"https://youtu.be/ddddddddddd": catalog.ErrVideoNotFound,
	}

	for link, want := range refused {
		if _, err := s.AddLink(context.Background(), 1, link); !errors.Is(err, want) {
			t.Errorf("add %s: %v, want %v", link, err, want)
		}
	}

	if _, err := s.AddLink(context.Background(), 1, "not a link"); err == nil {
		t.Error("a text with no video was added")
	}

	if len(store.added) != 1 {
		t.Fatalf("store got %d songs, want only the one that passed the checks", len(store.added))
	}
}

func TestASongAlreadyQueuedIsRefusedWithoutAskingYouTube(t *testing.T) {
	queued := room.Track{Item: 7, Video: "aaaaaaaaaaa", Title: "Fine"}
	finder := fakeFinder{}
	s := NewService(&recordingStore{added: []room.Track{queued}}, finder)

	_, err := s.AddLink(context.Background(), 1, "https://youtu.be/aaaaaaaaaaa")

	var already *AlreadyQueuedError
	if !errors.As(err, &already) || !errors.Is(err, ErrAlreadyQueued) || already.Track != queued {
		t.Fatalf("add a queued song: %v; want AlreadyQueuedError with the queued song", err)
	}

	if _, asked := finder[lookedUp]; asked {
		t.Fatal("YouTube was asked about a song already in the queue, spending quota")
	}
}

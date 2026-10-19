package queue

import (
	"context"

	"synqsong/internal/account"
	"synqsong/internal/catalog"
	"synqsong/internal/room"
)

// Finder looks a video up on YouTube.
type Finder interface {
	LookUp(ctx context.Context, id room.VideoID) (catalog.Video, error)
}

type Service struct {
	store  Store
	finder Finder
}

func NewService(store Store, finder Finder) *Service {
	return &Service{store: store, finder: finder}
}

// AddLink adds the video in a pasted link or id after checking that it may be queued.
// A song already queued is refused before YouTube is asked, so it costs no quota.
func (s *Service) AddLink(ctx context.Context, owner account.ID, link string) (room.Track, error) {
	id, err := catalog.FindVideoID(link)
	if err != nil {
		return room.Track{}, err
	}

	if queued, ok, err := s.store.Queued(ctx, owner, id); err != nil {
		return room.Track{}, err
	} else if ok {
		return room.Track{}, &AlreadyQueuedError{Track: queued}
	}

	video, err := s.finder.LookUp(ctx, id)
	if err != nil {
		return room.Track{}, err
	}

	if err := video.Queueable(); err != nil {
		return room.Track{}, err
	}

	return s.store.Add(ctx, owner, video.Track())
}

func (s *Service) Queue(ctx context.Context, owner account.ID) ([]room.Track, error) {
	return s.store.Queue(ctx, owner)
}

func (s *Service) Remove(ctx context.Context, owner account.ID, items []room.QueueItemID) (int, error) {
	return s.store.Remove(ctx, owner, items)
}

// Reorder takes the whole queue in its new order; a list that no longer matches is refused.
func (s *Service) Reorder(ctx context.Context, owner account.ID, order []room.QueueItemID) error {
	return s.store.Reorder(ctx, owner, order)
}

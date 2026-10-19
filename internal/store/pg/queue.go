package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/akkamaze/synqsong/internal/account"
	"github.com/akkamaze/synqsong/internal/room"
)

// Head returns the first song in a DJ's queue, if they have one.
func (s *Store) Head(ctx context.Context, dj account.ID) (room.Track, bool, error) {
	var (
		id         int64
		video      string
		track      room.Track
		durationMs *int64
	)

	err := s.pool.QueryRow(ctx, `
		SELECT id, video_id, title, channel, thumbnail, duration_ms
		FROM queue_items WHERE account_id = $1
		ORDER BY position LIMIT 1`, int64(dj)).
		Scan(&id, &video, &track.Title, &track.Channel, &track.Thumbnail, &durationMs)
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Track{}, false, nil
	}
	if err != nil {
		return room.Track{}, false, fmt.Errorf("read the first song of DJ %d: %w", dj, err)
	}

	track.Item = room.QueueItemID(id)
	track.Video = room.VideoID(video)

	if durationMs != nil {
		track.Duration = time.Duration(*durationMs) * time.Millisecond
	}

	return track, true, nil
}

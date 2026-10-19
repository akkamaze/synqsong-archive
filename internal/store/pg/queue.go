package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"synqsong/internal/account"
	"synqsong/internal/queue"
	"synqsong/internal/room"
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

// lockQueue serialises every change to one account's queue, including the room moving a
// finished song to the bottom, for the rest of the transaction. One-key advisory locks are
// account ids; any other lock must use the two-key form, whose key space Postgres keeps apart.
func lockQueue(ctx context.Context, tx pgx.Tx, owner account.ID) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(owner)); err != nil {
		return fmt.Errorf("lock queue of account %d: %w", owner, err)
	}

	return nil
}

func (s *Store) Queue(ctx context.Context, owner account.ID) ([]room.Track, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, video_id, title, channel, thumbnail, duration_ms
		FROM queue_items WHERE account_id = $1 ORDER BY position`, int64(owner))
	if err != nil {
		return nil, fmt.Errorf("read queue of account %d: %w", owner, err)
	}

	tracks := []room.Track{}

	for rows.Next() {
		var (
			id         int64
			video      string
			t          room.Track
			durationMs *int64
		)

		if err := rows.Scan(&id, &video, &t.Title, &t.Channel, &t.Thumbnail, &durationMs); err != nil {
			return nil, fmt.Errorf("read queue item: %w", err)
		}

		t.Item, t.Video = room.QueueItemID(id), room.VideoID(video)
		if durationMs != nil {
			t.Duration = time.Duration(*durationMs) * time.Millisecond
		}

		tracks = append(tracks, t)
	}

	return tracks, rows.Err()
}

func (s *Store) Queued(ctx context.Context, owner account.ID, video room.VideoID) (room.Track, bool, error) {
	return queuedCopy(ctx, s.pool, owner, video)
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func queuedCopy(ctx context.Context, q rowQuerier, owner account.ID, video room.VideoID) (room.Track, bool, error) {
	var (
		id         int64
		t          = room.Track{Video: video}
		durationMs *int64
	)

	err := q.QueryRow(ctx, `
		SELECT id, title, channel, thumbnail, duration_ms
		FROM queue_items WHERE account_id = $1 AND video_id = $2`, int64(owner), string(video)).
		Scan(&id, &t.Title, &t.Channel, &t.Thumbnail, &durationMs)
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Track{}, false, nil
	}
	if err != nil {
		return room.Track{}, false, fmt.Errorf("look for video %s in queue of account %d: %w", video, owner, err)
	}

	t.Item = room.QueueItemID(id)
	if durationMs != nil {
		t.Duration = time.Duration(*durationMs) * time.Millisecond
	}

	return t, true, nil
}

// Add puts a song at the top, or right after the one this account is playing in a room now.
func (s *Store) Add(ctx context.Context, owner account.ID, t room.Track) (room.Track, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return room.Track{}, fmt.Errorf("begin queue add: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockQueue(ctx, tx, owner); err != nil {
		return room.Track{}, err
	}

	position, err := insertPosition(ctx, tx, owner)
	if err != nil {
		return room.Track{}, err
	}

	var durationMs *int64
	if length, known := t.KnownDuration(); known {
		durationMs = ptr(length.Milliseconds())
	}

	var id int64

	err = tx.QueryRow(ctx, `
		INSERT INTO queue_items (account_id, video_id, title, channel, thumbnail, duration_ms, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (account_id, video_id) DO NOTHING
		RETURNING id`,
		int64(owner), string(t.Video), t.Title, t.Channel, t.Thumbnail, durationMs, position).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Track{}, alreadyQueued(ctx, tx, owner, t.Video)
	}
	if err != nil {
		return room.Track{}, fmt.Errorf("add to queue of account %d: %w", owner, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return room.Track{}, fmt.Errorf("commit queue add: %w", err)
	}

	t.Item = room.QueueItemID(id)

	return t, nil
}

// alreadyQueued names the copy that won a race of two adds; the lock makes it visible here.
func alreadyQueued(ctx context.Context, tx pgx.Tx, owner account.ID, video room.VideoID) error {
	queued, ok, err := queuedCopy(ctx, tx, owner, video)
	if err != nil {
		return err
	}

	if !ok {
		return queue.ErrAlreadyQueued
	}

	return &queue.AlreadyQueuedError{Track: queued}
}

func insertPosition(ctx context.Context, tx pgx.Tx, owner account.ID) (int64, error) {
	var playing int64

	err := tx.QueryRow(ctx, `
		SELECT q.position FROM room_stage s JOIN queue_items q ON q.id = s.item_id
		WHERE s.phase = $2 AND s.dj = $1 AND q.account_id = $1
		ORDER BY q.position LIMIT 1`, int64(owner), phasePlaying).Scan(&playing)
	if err == nil {
		_, err := tx.Exec(ctx, `UPDATE queue_items SET position = position + 1 WHERE account_id = $1 AND position > $2`, int64(owner), playing)
		if err != nil {
			return 0, fmt.Errorf("make room after the playing song: %w", err)
		}

		return playing + 1, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find the playing song of account %d: %w", owner, err)
	}

	var top int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(min(position), 1) - 1 FROM queue_items WHERE account_id = $1`, int64(owner)).Scan(&top); err != nil {
		return 0, fmt.Errorf("find the top of queue of account %d: %w", owner, err)
	}

	return top, nil
}

func (s *Store) Remove(ctx context.Context, owner account.ID, items []room.QueueItemID) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin queue removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockQueue(ctx, tx, owner); err != nil {
		return 0, err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM queue_items WHERE account_id = $1 AND id = ANY($2)`, int64(owner), itemIDs(items))
	if err != nil {
		return 0, fmt.Errorf("remove from queue of account %d: %w", owner, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit queue removal: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// Reorder takes the whole queue in its new order. Anything but exactly the current songs is refused.
func (s *Store) Reorder(ctx context.Context, owner account.ID, order []room.QueueItemID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin queue reorder: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockQueue(ctx, tx, owner); err != nil {
		return err
	}

	var matches bool

	err = tx.QueryRow(ctx, `
		SELECT array(SELECT id FROM queue_items WHERE account_id = $1 ORDER BY id)
		     = array(SELECT DISTINCT unnest($2::bigint[]) ORDER BY 1)
		   AND cardinality($2::bigint[]) = (SELECT count(*) FROM queue_items WHERE account_id = $1)`,
		int64(owner), itemIDs(order)).Scan(&matches)
	if err != nil {
		return fmt.Errorf("check new order of account %d: %w", owner, err)
	}

	if !matches {
		return queue.ErrStaleOrder
	}

	_, err = tx.Exec(ctx, `
		UPDATE queue_items q SET position = o.position
		FROM unnest($2::bigint[]) WITH ORDINALITY AS o(id, position)
		WHERE q.id = o.id AND q.account_id = $1`, int64(owner), itemIDs(order))
	if err != nil {
		return fmt.Errorf("reorder queue of account %d: %w", owner, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit queue reorder: %w", err)
	}

	return nil
}

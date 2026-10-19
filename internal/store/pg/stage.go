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

// Phase codes in room_stage.phase; the table's CHECK allows exactly these.
const (
	phaseIdle     = 1
	phaseChoosing = 2
	phasePlaying  = 3
	phaseResting  = 4
)

// stageRow is room_stage as nullable columns. Which ones are set depends on the phase.
type stageRow struct {
	phase      int16
	dj         *int64
	passed     int32
	failed     []int64
	votes      []int64
	playToken  *int64
	itemID     *int64
	videoID    *string
	title      *string
	channel    *string
	thumbnail  *string
	durationMs *int64
	restReason *int16
	restUntil  *time.Time
}

func encodeStage(p room.Phase) stageRow {
	switch p := p.(type) {
	case room.Idle:
		return stageRow{phase: phaseIdle, failed: []int64{}, votes: []int64{}}

	case room.Choosing:
		return stageRow{phase: phaseChoosing, dj: ptr(int64(p.DJ)), passed: int32(p.Passed), failed: itemIDs(p.Failed), votes: []int64{}}

	case room.Playing:
		t := p.Play.Track
		row := stageRow{
			phase:     phasePlaying,
			dj:        ptr(int64(p.Play.DJ)),
			failed:    itemIDs(p.Failed),
			votes:     accountIDs(p.Votes),
			playToken: ptr(int64(p.Play.Token)),
			itemID:    ptr(int64(t.Item)),
			videoID:   ptr(string(t.Video)),
			title:     ptr(t.Title),
			channel:   ptr(t.Channel),
			thumbnail: ptr(t.Thumbnail),
		}

		if length, known := t.KnownDuration(); known {
			row.durationMs = ptr(length.Milliseconds())
		}

		return row

	case room.Resting:
		return stageRow{
			phase:      phaseResting,
			dj:         ptr(int64(p.DJ)),
			failed:     []int64{},
			votes:      []int64{},
			restReason: ptr(int16(p.Reason)),
			restUntil:  ptr(p.Until),
		}
	}

	panic(fmt.Sprintf("encodeStage: unknown phase %T", p))
}

var errBadStage = errors.New("room_stage row does not match its phase")

func decodeStage(r stageRow) (room.Phase, error) {
	switch r.phase {
	case phaseIdle:
		return room.Idle{}, nil

	case phaseChoosing:
		if r.dj == nil {
			return nil, errBadStage
		}

		return room.Choosing{DJ: account.ID(*r.dj), Failed: queueItemIDs(r.failed), Passed: int(r.passed)}, nil

	case phasePlaying:
		if r.dj == nil || r.playToken == nil || r.itemID == nil || r.videoID == nil || r.title == nil {
			return nil, errBadStage
		}

		track := room.Track{
			Item:      room.QueueItemID(*r.itemID),
			Video:     room.VideoID(*r.videoID),
			Title:     *r.title,
			Channel:   deref(r.channel),
			Thumbnail: deref(r.thumbnail),
		}
		if r.durationMs != nil {
			track.Duration = time.Duration(*r.durationMs) * time.Millisecond
		}

		return room.Playing{
			Play:   room.Play{Token: room.PlayToken(*r.playToken), Track: track, DJ: account.ID(*r.dj)},
			Failed: queueItemIDs(r.failed),
			Votes:  memberIDs(r.votes),
		}, nil

	case phaseResting:
		if r.dj == nil || r.restReason == nil || r.restUntil == nil {
			return nil, errBadStage
		}

		return room.Resting{DJ: account.ID(*r.dj), Reason: room.RestReason(*r.restReason), Until: r.restUntil.UTC()}, nil
	}

	return nil, fmt.Errorf("%w: unknown phase %d", errBadStage, r.phase)
}

// loadStage reads the stage; a room never saved with one is idle.
func loadStage(ctx context.Context, tx pgx.Tx, id room.ID) (room.Phase, error) {
	var r stageRow

	err := tx.QueryRow(ctx, `
		SELECT phase, dj, passed, failed, votes, play_token, item_id, video_id, title,
		       channel, thumbnail, duration_ms, rest_reason, rest_until
		FROM room_stage WHERE room_id = $1`, id).
		Scan(&r.phase, &r.dj, &r.passed, &r.failed, &r.votes, &r.playToken, &r.itemID, &r.videoID, &r.title,
			&r.channel, &r.thumbnail, &r.durationMs, &r.restReason, &r.restUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Idle{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load stage of room %d: %w", id, err)
	}

	phase, err := decodeStage(r)
	if err != nil {
		return nil, fmt.Errorf("load stage of room %d: %w", id, err)
	}

	return phase, nil
}

func saveStage(ctx context.Context, tx pgx.Tx, id room.ID, p room.Phase) error {
	r := encodeStage(p)

	_, err := tx.Exec(ctx, `
		INSERT INTO room_stage (room_id, phase, dj, passed, failed, votes, play_token, item_id, video_id,
		                        title, channel, thumbnail, duration_ms, rest_reason, rest_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (room_id) DO UPDATE SET
			phase = EXCLUDED.phase, dj = EXCLUDED.dj, passed = EXCLUDED.passed, failed = EXCLUDED.failed,
			votes = EXCLUDED.votes, play_token = EXCLUDED.play_token, item_id = EXCLUDED.item_id,
			video_id = EXCLUDED.video_id, title = EXCLUDED.title, channel = EXCLUDED.channel,
			thumbnail = EXCLUDED.thumbnail, duration_ms = EXCLUDED.duration_ms,
			rest_reason = EXCLUDED.rest_reason, rest_until = EXCLUDED.rest_until`,
		id, r.phase, r.dj, r.passed, r.failed, r.votes, r.playToken, r.itemID, r.videoID,
		r.title, r.channel, r.thumbnail, r.durationMs, r.restReason, r.restUntil)
	if err != nil {
		return fmt.Errorf("save stage of room %d: %w", id, err)
	}

	return nil
}

func ptr[T any](v T) *T { return &v }

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func itemIDs(items []room.QueueItemID) []int64 {
	out := make([]int64, len(items))
	for i, item := range items {
		out[i] = int64(item)
	}

	return out
}

func queueItemIDs(ids []int64) []room.QueueItemID {
	if len(ids) == 0 {
		return nil
	}

	out := make([]room.QueueItemID, len(ids))
	for i, id := range ids {
		out[i] = room.QueueItemID(id)
	}

	return out
}

func accountIDs(ids []account.ID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}

	return out
}

func memberIDs(ids []int64) []account.ID {
	if len(ids) == 0 {
		return nil
	}

	out := make([]account.ID, len(ids))
	for i, id := range ids {
		out[i] = account.ID(id)
	}

	return out
}

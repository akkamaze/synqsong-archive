package pg

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"synqsong/internal/account"
	"synqsong/internal/room"
	"synqsong/internal/roomhost"
)

// Commit saves the change from before to next and its facts in one transaction.
// It returns roomhost.ErrFenced when the saved rev is no longer prev.
func (s *Store) Commit(ctx context.Context, id room.ID, prev roomhost.Rev, before, next room.State, facts []room.Fact) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin commit of room %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := advanceRev(ctx, tx, id, prev, next); err != nil {
		return err
	}

	steps := []func() error{
		func() error { return saveRoles(ctx, tx, id, before.Roles, next.Roles) },
		func() error { return saveBooth(ctx, tx, id, before.Booth, next.Booth) },
		func() error { return dropMembers(ctx, tx, id, before.Roles, next.Roles) },
		func() error { return saveBans(ctx, tx, id, before.Banned, next.Banned) },
		func() error { return saveStageIfChanged(ctx, tx, id, before.Phase, next.Phase) },
		func() error { return saveFacts(ctx, tx, id, facts) },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit room %d: %w", id, err)
	}

	return nil
}

// advanceRev fences the commit: it only succeeds while the saved rev is still prev.
func advanceRev(ctx context.Context, tx pgx.Tx, id room.ID, prev roomhost.Rev, next room.State) error {
	tag, err := tx.Exec(ctx, `
		UPDATE rooms
		SET rev = rev + 1, skip_policy = $3, skip_votes = $4, last_token = $5,
		    booth_head_leaving = $6, saved_at = now()
		WHERE id = $1 AND rev = $2`,
		id, prev, int16(next.Settings.Skip), int16(next.Settings.SkipVotes), int64(next.LastToken), next.Booth.HeadLeaving())
	if err != nil {
		return fmt.Errorf("save room %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return roomhost.ErrFenced
	}

	return nil
}

// saveRoles writes members whose role is new or changed. Guests are never stored.
func saveRoles(ctx context.Context, tx pgx.Tx, id room.ID, before, next map[account.ID]room.Role) error {
	for who, role := range next {
		if role == room.RoleGuest || before[who] == role {
			continue
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO memberships (room_id, account_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (room_id, account_id) DO UPDATE SET role = EXCLUDED.role`,
			id, int64(who), int16(role))
		if err != nil {
			return fmt.Errorf("save member %d of room %d: %w", who, id, err)
		}
	}

	return nil
}

func dropMembers(ctx context.Context, tx pgx.Tx, id room.ID, before, next map[account.ID]room.Role) error {
	for who := range before {
		if _, stays := next[who]; stays {
			continue
		}

		if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE room_id = $1 AND account_id = $2`, id, int64(who)); err != nil {
			return fmt.Errorf("remove member %d of room %d: %w", who, id, err)
		}
	}

	return nil
}

func saveBans(ctx context.Context, tx pgx.Tx, id room.ID, before, next map[account.ID]struct{}) error {
	for _, who := range slices.Sorted(maps.Keys(next)) {
		if _, known := before[who]; known {
			continue
		}

		_, err := tx.Exec(ctx, `INSERT INTO room_bans (room_id, account_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, int64(who))
		if err != nil {
			return fmt.Errorf("save ban of %d in room %d: %w", who, id, err)
		}
	}

	return nil
}

// saveBooth rewrites the seats when the order changed; a booth holds at most a few dozen DJs.
func saveBooth(ctx context.Context, tx pgx.Tx, id room.ID, before, next room.Booth) error {
	djs := next.DJs()
	if slices.Equal(before.DJs(), djs) {
		return nil
	}

	if _, err := tx.Exec(ctx, `DELETE FROM booth_seats WHERE room_id = $1`, id); err != nil {
		return fmt.Errorf("clear booth of room %d: %w", id, err)
	}

	for position, dj := range djs {
		_, err := tx.Exec(ctx, `INSERT INTO booth_seats (room_id, position, account_id) VALUES ($1, $2, $3)`, id, position, int64(dj))
		if err != nil {
			return fmt.Errorf("seat DJ %d in room %d: %w", dj, id, err)
		}
	}

	return nil
}

func saveStageIfChanged(ctx context.Context, tx pgx.Tx, id room.ID, before, next room.Phase) error {
	if reflect.DeepEqual(before, next) {
		return nil
	}

	return saveStage(ctx, tx, id, next)
}

func saveFacts(ctx context.Context, tx pgx.Tx, id room.ID, facts []room.Fact) error {
	for _, fact := range facts {
		if err := saveFact(ctx, tx, id, fact); err != nil {
			return err
		}
	}

	return nil
}

func saveFact(ctx context.Context, tx pgx.Tx, id room.ID, fact room.Fact) error {
	switch f := fact.(type) {
	case room.Finished:
		return finishPlay(ctx, tx, id, f)

	case room.LengthLearned:
		_, err := tx.Exec(ctx, `UPDATE queue_items SET duration_ms = $2 WHERE id = $1`, int64(f.Item), f.Duration.Milliseconds())
		if err != nil {
			return fmt.Errorf("remember length of queue item %d: %w", f.Item, err)
		}

	case room.Said:
		_, err := tx.Exec(ctx, `INSERT INTO chat_messages (room_id, account_id, body, said_at) VALUES ($1, $2, $3, $4)`,
			id, int64(f.By), string(f.Body), f.At)
		if err != nil {
			return fmt.Errorf("save chat line in room %d: %w", id, err)
		}

	case room.Removed:
		// Membership and booth seat follow from the role change; the ban from Banned.
	}

	return nil
}

// finishPlay records the play once per token and sends its song to the bottom of
// its owner's queue, only if it is still at the top, so saving it twice moves it once.
func finishPlay(ctx context.Context, tx pgx.Tx, id room.ID, f room.Finished) error {
	p := f.Play

	var durationMs *int64
	if length, known := p.Track.KnownDuration(); known {
		durationMs = ptr(length.Milliseconds())
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO play_history (room_id, play_token, dj, item_id, video_id, title, duration_ms, reason, ended_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (room_id, play_token) DO NOTHING`,
		id, int64(p.Token), int64(p.DJ), int64(p.Track.Item), string(p.Track.Video), p.Track.Title, durationMs, int16(f.Reason))
	if err != nil {
		return fmt.Errorf("record play %d in room %d: %w", p.Token, id, err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE queue_items
		SET position = (SELECT max(position) + 1 FROM queue_items WHERE account_id = $2)
		WHERE id = $1 AND account_id = $2
		  AND position = (SELECT min(position) FROM queue_items WHERE account_id = $2)`,
		int64(p.Track.Item), int64(p.DJ))
	if err != nil {
		return fmt.Errorf("move played item %d to the bottom: %w", p.Track.Item, err)
	}

	return nil
}

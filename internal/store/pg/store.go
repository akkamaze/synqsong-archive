// Package pg keeps rooms in Postgres. It is the only package that knows the schema.
package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"synqsong/internal/account"
	"synqsong/internal/room"
	"synqsong/internal/roomhost"
)

// Store connects as the app role, which may change rooms and queues but only add to history and chat.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var ErrRoomNotFound = errors.New("no room with that id")

// Load reads a room as last saved, with its booth DJs away since that save.
func (s *Store) Load(ctx context.Context, id room.ID) (room.State, roomhost.Rev, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return room.State{}, 0, fmt.Errorf("begin load of room %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := loadRoom(ctx, tx, id)
	if err != nil {
		return room.State{}, 0, err
	}

	state := room.State{Settings: saved.settings, LastToken: saved.lastToken, LastSaid: map[account.ID]time.Time{}}

	if state.Roles, err = loadRoles(ctx, tx, id); err != nil {
		return room.State{}, 0, err
	}

	if state.Banned, err = loadBans(ctx, tx, id); err != nil {
		return room.State{}, 0, err
	}

	djs, err := loadBooth(ctx, tx, id)
	if err != nil {
		return room.State{}, 0, err
	}

	state.Booth = room.BoothOf(djs, saved.headLeaving)

	if state.Phase, err = loadStage(ctx, tx, id); err != nil {
		return room.State{}, 0, err
	}

	return room.Restored(state, saved.savedAt), saved.rev, nil
}

// roomRow is one row of rooms: the room-wide part of its state.
type roomRow struct {
	rev         roomhost.Rev
	settings    room.Settings
	lastToken   room.PlayToken
	headLeaving bool
	savedAt     time.Time
}

func loadRoom(ctx context.Context, tx pgx.Tx, id room.ID) (roomRow, error) {
	var (
		row        roomRow
		rev        int64
		skipPolicy int16
		skipVotes  int16
		lastToken  int64
	)

	err := tx.QueryRow(ctx, `
		SELECT rev, skip_policy, skip_votes, last_token, booth_head_leaving, saved_at
		FROM rooms WHERE id = $1`, id).
		Scan(&rev, &skipPolicy, &skipVotes, &lastToken, &row.headLeaving, &row.savedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return roomRow{}, fmt.Errorf("load room %d: %w", id, ErrRoomNotFound)
	}
	if err != nil {
		return roomRow{}, fmt.Errorf("load room %d: %w", id, err)
	}

	row.savedAt = row.savedAt.UTC()
	row.rev = roomhost.Rev(rev)
	row.settings = room.Settings{Skip: room.SkipPolicy(skipPolicy), SkipVotes: room.SkipVotes(skipVotes)}
	row.lastToken = room.PlayToken(lastToken)

	return row, nil
}

func loadRoles(ctx context.Context, tx pgx.Tx, id room.ID) (map[account.ID]room.Role, error) {
	rows, err := tx.Query(ctx, `SELECT account_id, role FROM memberships WHERE room_id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("load members of room %d: %w", id, err)
	}

	roles := map[account.ID]room.Role{}

	for rows.Next() {
		var who int64
		var role int16

		if err := rows.Scan(&who, &role); err != nil {
			return nil, fmt.Errorf("read member of room %d: %w", id, err)
		}

		roles[account.ID(who)] = room.Role(role)
	}

	return roles, rows.Err()
}

func loadBans(ctx context.Context, tx pgx.Tx, id room.ID) (map[account.ID]struct{}, error) {
	rows, err := tx.Query(ctx, `SELECT account_id FROM room_bans WHERE room_id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("load bans of room %d: %w", id, err)
	}

	banned := map[account.ID]struct{}{}

	for rows.Next() {
		var who int64
		if err := rows.Scan(&who); err != nil {
			return nil, fmt.Errorf("read ban of room %d: %w", id, err)
		}

		banned[account.ID(who)] = struct{}{}
	}

	return banned, rows.Err()
}

func loadBooth(ctx context.Context, tx pgx.Tx, id room.ID) ([]account.ID, error) {
	rows, err := tx.Query(ctx, `SELECT account_id FROM booth_seats WHERE room_id = $1 ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("load booth of room %d: %w", id, err)
	}

	var djs []account.ID

	for rows.Next() {
		var who int64
		if err := rows.Scan(&who); err != nil {
			return nil, fmt.Errorf("read seat of room %d: %w", id, err)
		}

		djs = append(djs, account.ID(who))
	}

	return djs, rows.Err()
}

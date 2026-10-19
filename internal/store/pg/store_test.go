package pg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"synqsong/db"
	"synqsong/internal/account"
	"synqsong/internal/room"
)

// testPool connects as the app role to a disposable database, or is nil when none is configured.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	appURL, ownerURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_MIGRATION_DATABASE_URL")
	if appURL == "" || ownerURL == "" {
		fmt.Println("pg: TEST_DATABASE_URL and TEST_MIGRATION_DATABASE_URL not set, skipping database tests")
		os.Exit(m.Run())
	}

	if err := resetSchema(ownerURL); err != nil {
		fmt.Println("pg: reset test schema:", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), appURL)
	if err != nil {
		fmt.Println("pg: connect as app:", err)
		os.Exit(1)
	}

	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// resetSchema rebuilds the test database from the migrations as the owner role.
func resetSchema(ownerURL string) error {
	conn, err := sql.Open("pgx", ownerURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// Up first so Reset finds goose's version table even in a brand-new database.
	for _, step := range []func(*sql.DB, string, ...goose.OptionsFunc) error{goose.Up, goose.Reset, goose.Up} {
		if err := step(conn, "migrations"); err != nil {
			return err
		}
	}

	return nil
}

func needDB(t *testing.T) *Store {
	t.Helper()

	if testPool == nil {
		t.Skip("no test database configured")
	}

	return New(testPool)
}

var handles atomic.Int64

type fixture struct {
	id                   room.ID
	owner, ann, bob, cat account.ID
}

// newRoom creates four accounts and a room owned by the first, the way room creation will.
func newRoom(t *testing.T) fixture {
	t.Helper()

	ctx := context.Background()
	ids := make([]account.ID, 4)

	for i := range ids {
		var id int64
		if err := testPool.QueryRow(ctx, `INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
			t.Fatalf("create account: %v", err)
		}

		ids[i] = account.ID(id)
	}

	var roomID int64
	handle := fmt.Sprintf("test-%d", handles.Add(1))

	if err := testPool.QueryRow(ctx, `INSERT INTO rooms (handle) VALUES ($1) RETURNING id`, handle).Scan(&roomID); err != nil {
		t.Fatalf("create room: %v", err)
	}

	if _, err := testPool.Exec(ctx, `INSERT INTO memberships (room_id, account_id, role) VALUES ($1, $2, 4)`, roomID, int64(ids[0])); err != nil {
		t.Fatalf("make owner: %v", err)
	}

	return fixture{id: room.ID(roomID), owner: ids[0], ann: ids[1], bob: ids[2], cat: ids[3]}
}

func addQueueItem(t *testing.T, owner account.ID, video string, position int64) room.QueueItemID {
	t.Helper()

	var id int64

	err := testPool.QueryRow(context.Background(), `
		INSERT INTO queue_items (account_id, video_id, title, position) VALUES ($1, $2, $3, $4) RETURNING id`,
		int64(owner), video, "song "+video, position).Scan(&id)
	if err != nil {
		t.Fatalf("add queue item: %v", err)
	}

	return room.QueueItemID(id)
}

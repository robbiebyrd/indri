package sqlite_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
)

func TestOpen_CreatesTablesWithMemoryDSN(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	for _, tbl := range []string{"games", "users", "sessions"} {
		var name string
		row := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		)
		if err := row.Scan(&name); err != nil || name != tbl {
			t.Errorf("table %q not found: scan err=%v", tbl, err)
		}
	}
}

// TestOpen_MemoryDSN_ConcurrentConnectionsShareSchema proves the pool is
// pinned to a single connection: without that, a ":memory:" DSN gives each
// new pooled connection its own empty database, and concurrent callers would
// see "no such table" instead of the schema Open just migrated.
func TestOpen_MemoryDSN_ConcurrentConnectionsShareSchema(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const n = 50
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var name string
			row := db.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='table' AND name=?", "games",
			)
			if err := row.Scan(&name); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", i, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestOpen_AddsSessionCreatedAtToAnExistingDatabase opens a database made
// before sessions had created_at: Open adds the column, gives the existing
// session a creation time so it gets a full lifetime rather than expiring
// at once, and stays safe to run again.
func TestOpen_AddsSessionCreatedAtToAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := old.Exec(`
CREATE TABLE sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    TEXT NOT NULL
);
INSERT INTO sessions (id, token, user_id, data) VALUES ('s1', 't1', 'u1', '{}');`); err != nil {
		t.Fatalf("creating the old schema: %v", err)
	}
	old.Close()

	before := time.Now().UnixNano()
	var first int64
	for i := 0; i < 2; i++ {
		db, err := sqlite.Open(path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i+1, err)
		}

		var createdAt int64
		if err := db.QueryRow(`SELECT created_at FROM sessions WHERE id = 's1'`).Scan(&createdAt); err != nil {
			t.Fatalf("reading created_at: %v", err)
		}
		if createdAt < before {
			t.Errorf("created_at = %d, want the migration time (>= %d)", createdAt, before)
		}
		if i == 0 {
			first = createdAt
		} else if createdAt != first {
			t.Errorf("reopening changed created_at from %d to %d", first, createdAt)
		}
		db.Close()
	}
}

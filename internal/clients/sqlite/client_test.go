package sqlite_test

import (
	"fmt"
	"sync"
	"testing"

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

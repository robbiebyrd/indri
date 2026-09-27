package sqlite_test

import (
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

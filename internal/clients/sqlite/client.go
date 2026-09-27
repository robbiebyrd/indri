// Package sqlite opens a shared SQLite database used by all SQLiteStore
// implementations. It sets WAL journal mode for concurrent reader throughput
// and runs the schema DDL on first open.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS games (
    id      TEXT    PRIMARY KEY,
    code    TEXT    UNIQUE NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    private INTEGER NOT NULL DEFAULT 0,
    data    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id       TEXT PRIMARY KEY,
    email    TEXT UNIQUE NOT NULL,
    password TEXT,
    data     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    TEXT NOT NULL
);
`

// Open opens (or creates) the SQLite database at dsn, enables WAL mode,
// enforces foreign keys, and runs the schema DDL. Pass ":memory:" for tests.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", dsn, err)
	}

	// Pin the pool to a single connection. SQLite is single-writer regardless,
	// a ":memory:" DSN gives each new connection its own empty database (there
	// is nothing to share without a shared-cache DSN), and pragmas such as
	// foreign_keys are per-connection — so more than one pooled connection
	// would see an unmigrated schema and/or a different pragma state.
	db.SetMaxOpenConns(1)

	// WAL gives concurrent readers without blocking writers.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma foreign_keys: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}

	return db, nil
}

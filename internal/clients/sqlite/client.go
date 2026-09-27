// Package sqlite opens a shared SQLite database used by all SQLiteStore
// implementations. It sets WAL journal mode for concurrent reader throughput
// and runs the schema DDL on first open.
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

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
    id         TEXT PRIMARY KEY,
    token      TEXT UNIQUE,
    user_id    TEXT UNIQUE,
    data       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT 0
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

	if err := migrateSessionCreatedAt(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}

	return db, nil
}

// migrateSessionCreatedAt gives a sessions table created before created_at
// existed that column. created_at (Unix nanoseconds) backs session expiry.
// Sessions that predate the column are stamped with the migration time, so
// they get a full lifetime instead of expiring at once.
//
// The ALTER and the stamp commit together (SQLite DDL is transactional), so
// a crash cannot leave the column added but its rows unstamped. SQLite has
// no ADD COLUMN IF NOT EXISTS, so "duplicate column name" means the column
// is already there: a current schema, or another process migrated first.
func migrateSessionCreatedAt(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("adding sessions.created_at: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("adding sessions.created_at: %w", err)
		}
		// Release the transaction now: the pool has one connection, which
		// the index statement below needs.
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("adding sessions.created_at: %w", err)
		}
	} else {
		if _, err := tx.Exec(`UPDATE sessions SET created_at = ?`, time.Now().UnixNano()); err != nil {
			return fmt.Errorf("stamping existing sessions: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("adding sessions.created_at: %w", err)
		}
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions (created_at)`); err != nil {
		return fmt.Errorf("indexing sessions.created_at: %w", err)
	}

	return nil
}

// IsUniqueViolation reports whether err is a UNIQUE constraint violation.
// modernc.org/sqlite reports one only through its message.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

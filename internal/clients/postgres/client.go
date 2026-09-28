// Package postgres opens the single PostgreSQL connection pool shared by all
// PostgresStore implementations. Each store runs its own schema DDL.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

// MaxOpenConns caps the shared pool. PostgreSQL's default max_connections is
// 100; 25 per server process leaves room for a few horizontally scaled
// instances plus admin/migration sessions without exhausting the server.
const MaxOpenConns = 25

// Open opens a pool for uri, caps it at MaxOpenConns, and verifies the server
// is reachable. The caller owns the returned *sql.DB and must Close it.
func Open(ctx context.Context, uri string) (*sql.DB, error) {
	if uri == "" {
		return nil, errors.New("postgres URI is required")
	}

	db, err := sql.Open("pgx", uri)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	db.SetMaxOpenConns(MaxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return db, nil
}

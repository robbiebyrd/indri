package game

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/robbiebyrd/indri/internal/repo/sqldialect"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// gamesSchema is this store's own DDL. Each PostgreSQL store migrates the table
// it owns, rather than one package knowing every table in the database.
const gamesSchema = `
CREATE TABLE IF NOT EXISTS games (
    id      TEXT    PRIMARY KEY,
    code    TEXT    UNIQUE NOT NULL,
    version BIGINT  NOT NULL DEFAULT 1,
    private BOOLEAN NOT NULL DEFAULT FALSE,
    data    JSONB   NOT NULL
);
`

// PostgresStore is a game store backed by PostgreSQL. Like every other backend
// it is core plus a persistence port, so the rules are the same code the
// MongoDB store runs.
type PostgresStore struct {
	core
}

// NewPostgresStore builds a PostgreSQL-backed game store over an already-open
// pool and creates its table. The caller owns db.
func NewPostgresStore(
	ctx context.Context,
	db *sql.DB,
	locks lock.Manager,
	publisher events.Publisher,
) (*PostgresStore, error) {
	if locks == nil {
		locks = lock.NewInProcess()
	}

	if _, err := db.ExecContext(ctx, gamesSchema); err != nil {
		return nil, fmt.Errorf("creating the games table: %w", err)
	}

	return &PostgresStore{
		core: core{
			ctx:       ctx,
			docs:      sqlDocs{db: db, dialect: sqldialect.Postgres},
			locks:     locks,
			publisher: publisher,
		},
	}, nil
}

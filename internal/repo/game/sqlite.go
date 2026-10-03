package game

import (
	"context"
	"database/sql"

	"github.com/robbiebyrd/indri/internal/repo/sqldialect"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// SQLiteStore is a game store backed by SQLite. Like every other backend it is
// core plus a persistence port, so the rules — the mutation bracket, the
// version fence, the change deltas — are the same code the MongoDB store runs.
type SQLiteStore struct {
	core
}

// NewSQLiteStore builds a SQLite-backed game store over an already-open
// database. The caller owns db; internal/clients/sqlite.Open creates the schema.
func NewSQLiteStore(
	ctx context.Context,
	db *sql.DB,
	locks lock.Manager,
	publisher events.Publisher,
) *SQLiteStore {
	if locks == nil {
		locks = lock.NewInProcess()
	}

	return &SQLiteStore{
		core: core{
			ctx:       ctx,
			docs:      sqlDocs{db: db, dialect: sqldialect.SQLite},
			locks:     locks,
			publisher: publisher,
		},
	}
}

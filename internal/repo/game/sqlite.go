package game

import (
	"context"
	"database/sql"

	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
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
			docs:      sqlDocs{db: db, dialect: sqliteDialect},
			locks:     locks,
			publisher: publisher,
		},
	}
}

// sqliteDialect: SQLite binds with "?" and reports a unique violation only
// through the driver's error text.
var sqliteDialect = dialect{
	placeholder:       func(int) string { return "?" },
	isUniqueViolation: sqliteClient.IsUniqueViolation,
}

package sqldialect

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRebind(t *testing.T) {
	const query = `UPDATE t SET a = ?, b = ? WHERE id = ?`

	if got := SQLite.Rebind(query); got != query {
		t.Errorf("SQLite.Rebind = %q, want the query unchanged", got)
	}

	if got, want := Postgres.Rebind(query), `UPDATE t SET a = $1, b = $2 WHERE id = $3`; got != want {
		t.Errorf("Postgres.Rebind = %q, want %q", got, want)
	}
}

func TestLockForUpdate(t *testing.T) {
	// SQLite has no row locks: its pool is one connection, so a transaction
	// already holds the whole database. PostgreSQL has to be asked.
	if got := SQLite.LockForUpdate(); got != "" {
		t.Errorf("SQLite.LockForUpdate = %q, want none", got)
	}

	if got := Postgres.LockForUpdate(); got != " FOR UPDATE" {
		t.Errorf("Postgres.LockForUpdate = %q, want \" FOR UPDATE\"", got)
	}
}

func TestIsUniqueViolation(t *testing.T) {
	sqliteErr := errors.New("constraint failed: UNIQUE constraint failed: users.email (2067)")
	pgErr := fmt.Errorf("inserting: %w", &pgconn.PgError{Code: "23505"})
	other := errors.New("disk full")

	for _, tc := range []struct {
		name    string
		dialect Dialect
		err     error
		want    bool
	}{
		{"sqlite unique", SQLite, sqliteErr, true},
		{"sqlite other", SQLite, other, false},
		{"sqlite nil", SQLite, nil, false},
		{"postgres 23505, wrapped", Postgres, pgErr, true},
		{"postgres other code", Postgres, &pgconn.PgError{Code: "23503"}, false},
		{"postgres other", Postgres, other, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.dialect.IsUniqueViolation(tc.err); got != tc.want {
				t.Errorf("IsUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

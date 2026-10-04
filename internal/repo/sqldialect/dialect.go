// Package sqldialect is the small amount that genuinely differs between SQLite
// and PostgreSQL, shared by every SQL-backed store. Each store writes its
// statements once, with "?" placeholders, and serves both databases.
package sqldialect

import (
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
)

// Dialect is how one SQL database spells what the others spell differently.
// Anything larger than this belongs in the store, shared.
type Dialect struct {
	// placeholder renders the n-th bind parameter, 1-based.
	placeholder func(n int) string

	// isUniqueViolation reports whether err is the database's way of saying a
	// unique index rejected the write. Neither driver does so with a sentinel.
	isUniqueViolation func(error) bool

	// lockForUpdate is the clause that locks a row read inside a transaction
	// until it commits.
	lockForUpdate string

	// claimLock is lockForUpdate for a reader that wants any one matching row
	// rather than a particular one: rows another transaction holds are passed
	// over instead of waited for.
	claimLock string
}

// SQLite binds with "?" and reports a unique violation only through the
// driver's error text.
var SQLite = Dialect{
	placeholder:       func(int) string { return "?" },
	isUniqueViolation: sqliteClient.IsUniqueViolation,

	// None: the pool is pinned to one connection, so a transaction already
	// holds the whole database until it ends.
	lockForUpdate: "",
	claimLock:     "",
}

// Postgres binds with "$n" and reports a unique violation as SQLSTATE 23505,
// which pgx surfaces as a typed error rather than in the text.
var Postgres = Dialect{
	placeholder: func(n int) string { return "$" + strconv.Itoa(n) },
	isUniqueViolation: func(err error) bool {
		var pgErr *pgconn.PgError

		return errors.As(err, &pgErr) && pgErr.Code == "23505"
	},
	lockForUpdate: " FOR UPDATE",
	claimLock:     " FOR UPDATE SKIP LOCKED",
}

// Rebind renders query, written with "?" placeholders, in this dialect.
func (d Dialect) Rebind(query string) string {
	if d.placeholder(1) == "?" {
		return query
	}

	var b strings.Builder

	n := 0

	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString(d.placeholder(n))

			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

// LockForUpdate is the clause to append to a SELECT inside a transaction so the
// rows it reads stay locked until the transaction ends: what makes a
// read-modify-write safe against a concurrent one.
func (d Dialect) LockForUpdate() string {
	return d.lockForUpdate
}

// ClaimLock is the clause to append to a SELECT that picks one row of many to
// take for itself, inside a transaction: the row it reads stays locked, and a
// row another claimer has already locked is skipped, so concurrent claimers
// take different rows instead of queueing behind the first.
func (d Dialect) ClaimLock() string {
	return d.claimLock
}

// IsUniqueViolation reports whether err is a unique-index rejection.
func (d Dialect) IsUniqueViolation(err error) bool {
	return err != nil && d.isUniqueViolation(err)
}

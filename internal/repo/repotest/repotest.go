// Package repotest opens the databases a repository's contract tests run
// against. Every store package runs one suite against every backend, and each
// needs the same three things: a private SQLite file, a private PostgreSQL
// schema, and a MongoDB client. They live here once.
//
// A backend whose database is not reachable skips. That is the only skip a
// contract suite is allowed: the in-memory backend and SQLite always run, so a
// machine with no database still executes every assertion.
package repotest

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	postgresClient "github.com/robbiebyrd/indri/internal/clients/postgres"
	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

// SQLite opens a private, file-backed database in the test's temp directory.
//
// A file rather than ":memory:" because tests run in parallel goroutines
// against one *sql.DB whose pool is pinned to a single connection; a file gives
// the same isolation without serialising every test behind one handle.
func SQLite(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqliteClient.Open(filepath.Join(t.TempDir(), "indri.db"))
	if err != nil {
		t.Fatalf("opening the test SQLite database: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// Postgres connects to INDRI_TEST_POSTGRES_URI with a schema of its own, so
// parallel runs and rows left by an earlier run cannot collide. The schema is
// dropped on cleanup. It skips when the variable is unset or the server is
// unreachable.
//
// The schema is selected in the connection string rather than with a SET:
// search_path is per connection, and a SET reaches only the one connection it
// ran on, so a second pooled connection would quietly use the public schema.
func Postgres(t *testing.T) *sql.DB {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping the PostgreSQL backend")
	}

	admin := openPostgres(t, uri)

	schema := "indri_test_" + strings.ReplaceAll(ids.New(), "-", "")
	if _, err := admin.ExecContext(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("creating the test schema: %v", err)
	}

	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	})

	scoped, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parsing INDRI_TEST_POSTGRES_URI: %v", err)
	}

	query := scoped.Query()
	query.Set("search_path", schema)
	scoped.RawQuery = query.Encode()

	return openPostgres(t, scoped.String())
}

func openPostgres(t *testing.T, uri string) *sql.DB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := postgresClient.Open(ctx, uri)
	if err != nil {
		t.Skipf("skipping: PostgreSQL not reachable: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// Mongo connects to a local MongoDB (INDRI_TEST_MONGO_URI, or localhost), in
// the indri_test database. It skips when none is reachable.
func Mongo(t *testing.T) *mongodb.Client {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}

	// os.Setenv rather than t.Setenv, which refuses to run in a parallel test.
	_ = os.Setenv("INDRI_MONGO_URI", uri)
	_ = os.Setenv("INDRI_MONGO_DATABASE", "indri_test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongodb.New(ctx)
	if err != nil {
		t.Skipf("skipping: MongoDB not reachable: %v", err)
	}

	return client
}

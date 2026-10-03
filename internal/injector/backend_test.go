package injector

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/repotest"
	scheduleRepo "github.com/robbiebyrd/indri/internal/repo/schedule"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// captureLog collects what the code under test logs, for the test to assert on
// and to keep out of the test binary's own output.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	return &buf
}

func clientsWith(db *sql.DB) *ClientsInjector {
	return &ClientsInjector{
		SQLDB:       db,
		LockManager: lock.NewInProcess(),
		Publisher:   events.NewInProcess(),
	}
}

// typeName renders a store's concrete type, which is what says which backend
// was chosen.
func typeName(v any) string {
	return fmt.Sprintf("%T", v)
}

// INDRI_DB_BACKEND picks one backend for games, users and sessions together: a
// server must never keep its users in one database and their sessions in
// another.
func TestOpenStores_EveryRepoFollowsTheBackend(t *testing.T) {
	for _, tc := range []struct {
		backend string
		db      func(t *testing.T) *sql.DB
		want    [4]any

		// warns: the backend keeps timers in memory although its other data
		// is durable, which an operator has to be told.
		warns bool
	}{
		{
			backend: "memory",
			db:      func(*testing.T) *sql.DB { return nil },
			want:    [4]any{&gameRepo.MemoryStore{}, &userRepo.MemoryStore{}, &sessionRepo.MemoryStore{}, &scheduleRepo.MemoryStore{}},
		},
		{
			backend: "sqlite",
			db:      repotest.SQLite,
			want:    [4]any{&gameRepo.SQLiteStore{}, &userRepo.SQLStore{}, &sessionRepo.SQLStore{}, &scheduleRepo.MemoryStore{}},
			warns:   true,
		},
		{
			backend: "postgres",
			db:      repotest.Postgres,
			want:    [4]any{&gameRepo.PostgresStore{}, &userRepo.SQLStore{}, &sessionRepo.SQLStore{}, &scheduleRepo.MemoryStore{}},
			warns:   true,
		},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			db := tc.db(t)
			logged := captureLog(t)

			got, err := openStores(context.Background(), tc.backend, clientsWith(db))
			if err != nil {
				t.Fatalf("openStores(%s): %v", tc.backend, err)
			}

			if warned := strings.Contains(logged.String(), "timers (indri.after) are kept in memory"); warned != tc.warns {
				t.Errorf("warned about in-memory timers = %v, want %v; logged %q", warned, tc.warns, logged.String())
			}

			for i, store := range []any{got.game, got.user, got.session, got.schedule} {
				if typeName(store) != typeName(tc.want[i]) {
					t.Errorf("store %d is %s, want %s", i, typeName(store), typeName(tc.want[i]))
				}
			}
		})
	}
}

func TestOpenStores_MongoDBUsesMongoForEverything(t *testing.T) {
	clients := clientsWith(nil)
	clients.MongoDBClient = repotest.Mongo(t)

	got, err := openStores(context.Background(), "mongodb", clients)
	if err != nil {
		t.Fatalf("openStores(mongodb): %v", err)
	}

	want := []any{&gameRepo.Store{}, &userRepo.Store{}, &sessionRepo.Store{}, &scheduleRepo.Store{}}
	for i, store := range []any{got.game, got.user, got.session, got.schedule} {
		if typeName(store) != typeName(want[i]) {
			t.Errorf("store %d is %s, want %s", i, typeName(store), typeName(want[i]))
		}
	}
}

// A misconfigured backend is refused at boot, naming the problem, rather than
// starting a server that fails on its first request.
func TestOpenStores_RefusesAMisconfiguredBackend(t *testing.T) {
	for name, tc := range map[string]struct {
		backend string
		clients *ClientsInjector
		want    string
	}{
		"an unknown backend":          {"oracle", clientsWith(nil), `"oracle"`},
		"mongodb with no client":      {"mongodb", clientsWith(nil), "mongodb"},
		"sqlite with no database":     {"sqlite", clientsWith(nil), "sqlite"},
		"postgres with no connection": {"postgres", clientsWith(nil), "postgres"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := openStores(context.Background(), tc.backend, tc.clients)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("openStores(%s) = %v, want an error mentioning %s", tc.backend, err, tc.want)
			}
		})
	}
}

// The clients layer opens only the database the backend needs: a SQLite server
// does not need MongoDB to be reachable.
func TestOpenDatabase_OpensOnlyWhatTheBackendNeeds(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		mongo, db, err := openDatabase(context.Background(), &envVars.Vars{
			DBBackend:  "sqlite",
			SQLitePath: filepath.Join(t.TempDir(), "indri.db"),
		})
		if err != nil {
			t.Fatalf("openDatabase: %v", err)
		}

		t.Cleanup(func() { _ = db.Close() })

		if mongo != nil || db == nil {
			t.Fatalf("sqlite opened mongo=%v sql=%v, want only SQL", mongo, db)
		}
	})

	t.Run("memory", func(t *testing.T) {
		mongo, db, err := openDatabase(context.Background(), &envVars.Vars{DBBackend: "memory"})
		if err != nil || mongo != nil || db != nil {
			t.Fatalf("memory = %v, %v, %v; want nothing opened", mongo, db, err)
		}
	})

	t.Run("postgres with no URI", func(t *testing.T) {
		if _, _, err := openDatabase(context.Background(), &envVars.Vars{DBBackend: "postgres"}); err == nil ||
			!strings.Contains(err.Error(), "INDRI_POSTGRES_URI") {
			t.Fatalf("openDatabase(postgres, no URI) = %v, want an error naming INDRI_POSTGRES_URI", err)
		}
	})

	// Redis is the multi-instance switch: instances share locks and change
	// events. Storage that lives inside one instance cannot be shared with
	// it, so every instance would hold its own users, sessions and games.
	for _, local := range []string{"memory", "sqlite"} {
		t.Run(local+" with redis", func(t *testing.T) {
			_, _, err := openDatabase(context.Background(), &envVars.Vars{
				DBBackend:   local,
				LockBackend: "redis",
				SQLitePath:  filepath.Join(t.TempDir(), "indri.db"),
			})
			if err == nil || !strings.Contains(err.Error(), "INDRI_LOCK_BACKEND") {
				t.Fatalf("openDatabase(%s, redis) = %v, want an error naming INDRI_LOCK_BACKEND", local, err)
			}
		})
	}

	t.Run("an unknown backend", func(t *testing.T) {
		if _, _, err := openDatabase(context.Background(), &envVars.Vars{DBBackend: "oracle"}); err == nil ||
			!strings.Contains(err.Error(), `"oracle"`) {
			t.Fatalf("openDatabase(oracle) = %v, want an error naming it", err)
		}
	})
}

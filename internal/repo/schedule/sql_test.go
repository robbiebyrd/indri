package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/repo/repotest"
)

// forEachSQLDialect runs test against SQLite and, where one is reachable,
// PostgreSQL.
func forEachSQLDialect(t *testing.T, test func(t *testing.T, store *SQLStore)) {
	t.Helper()

	cfg := Config{InstanceID: "instance-a", OrphanGrace: time.Hour}

	t.Run("sqlite", func(t *testing.T) {
		test(t, NewSQLiteStore(context.Background(), repotest.SQLite(t), cfg))
	})

	t.Run("postgres", func(t *testing.T) {
		store, err := NewPostgresStore(context.Background(), repotest.Postgres(t), cfg)
		if err != nil {
			t.Fatalf("opening the PostgreSQL schedule store: %v", err)
		}

		test(t, store)
	})
}

// SQL has no TTL index, so the leak stop MongoDB gets from one on fireAt is
// done by the store: an entry OrphanGrace past its fire time goes, whatever its
// state, because an abandoned game's timer is never claimed and nothing in this
// repo deletes a game.
func TestSQLStore_PrunesEntriesPastTheirOrphanGrace(t *testing.T) {
	forEachSQLDialect(t, func(t *testing.T, store *SQLStore) {
		now := time.Now()

		orphan := mustScheduleOn(t, store, "game-abandoned", now.Add(-2*time.Hour))
		recent := mustScheduleOn(t, store, "game-live", now.Add(-30*time.Minute))

		// Scheduling is what prunes: it is how a store gains entries, so it is
		// where a store that keeps growing gets the chance to shrink.
		mustScheduleOn(t, store, "game-live", now.Add(time.Hour))

		if _, err := store.Get(orphan.ID.Hex()); !errors.Is(err, ErrNotFound) {
			t.Errorf("an entry two hours past its fire time with a one-hour grace is still stored: %v", err)
		}

		if _, err := store.Get(recent.ID.Hex()); err != nil {
			t.Errorf("an entry still inside its grace was pruned: %v", err)
		}
	})
}

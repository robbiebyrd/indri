package game

import (
	"context"
	"testing"
)

// docsOf reaches the persistence port underneath a store.
//
// The fence is below the lock, and every test that goes through Mutate holds
// the lock for the whole read-modify-write — so a backend whose fence does
// nothing at all still passes them. Reaching the port directly is the only way
// to put a stale write to the database and see what it does.
func docsOf(t *testing.T, store Storer) docs {
	t.Helper()

	switch s := store.(type) {
	case *Store:
		return s.docs
	case *MemoryStore:
		return s.docs
	case *SQLiteStore:
		return s.docs
	case *PostgresStore:
		return s.docs
	default:
		t.Fatalf("no persistence port known for %T", store)

		return nil
	}
}

// TestSaveVersioned_RejectsAStaleWrite is the version fence itself.
//
// mutation.Run serialises writers with a distributed lock, which makes a lost
// update rare; the fence is what makes it impossible, because a lease can
// expire mid-mutation and leave two writers believing they hold it. This test
// deliberately bypasses the lock: it saves once to move the version on, then
// replays a write that still carries the old version. That second write must
// not commit.
//
// Without this, a backend could drop the version predicate from its UPDATE
// entirely and every other test in the package would still pass.
func TestSaveVersioned_RejectsAStaleWrite(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		d := docsOf(t, b.store)

		created := newGame(t, b.store, nil)

		loaded, err := d.load(ctx, created.ID)
		if err != nil {
			t.Fatalf("loading the game: %v", err)
		}

		staleVersion := loaded.Version

		// The write a healthy writer makes: it holds the current version, so it
		// commits and moves the stored version on.
		committed, err := d.saveVersioned(ctx, created.ID, loaded, staleVersion)
		if err != nil {
			t.Fatalf("first save: %v", err)
		}

		if !committed {
			t.Fatalf("first save did not commit, so the fence cannot be tested")
		}

		// The write a writer with an expired lease makes: it still believes the
		// version it loaded is current. The fence has to refuse it.
		replayed, err := d.load(ctx, created.ID)
		if err != nil {
			t.Fatalf("reloading the game: %v", err)
		}

		committed, err = d.saveVersioned(ctx, created.ID, replayed, staleVersion)
		if err != nil {
			t.Fatalf("stale save returned an error rather than refusing: %v", err)
		}

		if committed {
			t.Error("a write carrying a stale version committed; the version fence is not enforced")
		}
	})
}

// TestSaveVersioned_BumpsTheVersionOnCommit pins the other half of the fence:
// a committed write has to move the version on, or the next stale write would
// be indistinguishable from a current one.
func TestSaveVersioned_BumpsTheVersionOnCommit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		d := docsOf(t, b.store)

		created := newGame(t, b.store, nil)

		loaded, err := d.load(ctx, created.ID)
		if err != nil {
			t.Fatalf("loading the game: %v", err)
		}

		before := loaded.Version

		if _, err := d.saveVersioned(ctx, created.ID, loaded, before); err != nil {
			t.Fatalf("saving: %v", err)
		}

		reloaded, err := d.load(ctx, created.ID)
		if err != nil {
			t.Fatalf("reloading: %v", err)
		}

		if reloaded.Version != before+1 {
			t.Errorf("version after a committed save = %d, want %d", reloaded.Version, before+1)
		}
	})
}

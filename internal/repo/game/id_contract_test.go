package game

import (
	"regexp"
	"testing"
)

// uuidV4 is the shape ids.New mints. Pinning it here rather than just
// "any non-empty string" is the point of the test: an id that is still a
// Mongo ObjectID hex would pass a non-empty check and then fail to round
// trip through a SQLite or Postgres TEXT primary key, which is exactly the
// bug this contract exists to catch.
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestNew_MintsAStringIDEveryBackendCanStore proves a new game's identifier is
// a backend-neutral string, not a Mongo ObjectID.
//
// The identifier has to be something every store can hold: the SQL schemas
// declare `id TEXT PRIMARY KEY`, and an ObjectID only exists because MongoDB
// mints one. If this regresses, the Mongo backend keeps working and the SQL
// ones cannot store a game at all — a failure that would otherwise surface
// far from its cause.
func TestNew_MintsAStringIDEveryBackendCanStore(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		g := newGame(t, b.store, nil)

		if !uuidV4.MatchString(g.ID) {
			t.Fatalf("new game ID = %q, want a UUIDv4 string", g.ID)
		}
	})
}

// TestNew_IDRoundTripsThroughGet proves the id a store hands back is the same
// one it answers to. A store that minted one identifier and indexed another
// would still pass the shape check above.
func TestNew_IDRoundTripsThroughGet(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		created := newGame(t, b.store, nil)

		got, err := b.store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get(%q): %v", created.ID, err)
		}

		if got.ID != created.ID {
			t.Errorf("Get returned ID %q, want %q", got.ID, created.ID)
		}
	})
}

// TestNew_IDsAreUnique guards the obvious way a string id could go wrong:
// minting a constant, or deriving it from something two games can share.
func TestNew_IDsAreUnique(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		seen := make(map[string]struct{}, 25)

		for i := 0; i < 25; i++ {
			g := newGame(t, b.store, nil)
			if _, dup := seen[g.ID]; dup {
				t.Fatalf("duplicate game ID %q at iteration %d", g.ID, i)
			}
			seen[g.ID] = struct{}{}
		}
	})
}

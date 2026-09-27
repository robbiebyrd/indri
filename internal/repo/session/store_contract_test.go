package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
)

// Behavior every session store must share. Each test runs against every
// backend available here: memory and SQLite always, MongoDB when reachable,
// PostgreSQL when INDRI_TEST_POSTGRES_URI is set.

func eachStore(t *testing.T, test func(t *testing.T, store Storer)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) { test(t, newMemoryFixture(t)) })
	t.Run("sqlite", func(t *testing.T) { test(t, newSQLiteFixture(t)) })
	t.Run("mongo", func(t *testing.T) { test(t, newTestStore(t)) })
	t.Run("postgres", func(t *testing.T) { test(t, newPostgresSessionFixture(t)) })
}

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// assertPlace checks the session's game, team, and slot.
func assertPlace(t *testing.T, s *models.Session, game, team, slot string) {
	t.Helper()

	if str(s.GameID) != game || str(s.TeamID) != team || str(s.SlotID) != slot {
		t.Fatalf("session place = game %s, team %s, slot %s; want %s, %s, %s",
			str(s.GameID), str(s.TeamID), str(s.SlotID), game, team, slot)
	}
}

func TestStore_NewKeepsEveryField(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		user := fmt.Sprintf("u-%d", time.Now().UnixNano())
		created, err := store.New(models.CreateSession{UserID: user, Token: "tok-" + user, GameID: "g1", TeamID: "red", SlotID: "p0"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assertPlace(t, got, "g1", "red", "p0")
		if str(got.UserID) != user {
			t.Fatalf("UserID = %s, want %s", str(got.UserID), user)
		}
	})
}

// Update changes only the fields it is given: the session keeps its user.
func TestStore_UpdateSetsTheGameTeamAndSlot(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		user := fmt.Sprintf("u-%d", time.Now().UnixNano())
		created, err := store.New(models.CreateSession{UserID: user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Update(created.ID, &models.UpdateSession{GameID: "g2", TeamID: "blue", SlotID: "p3"}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assertPlace(t, got, "g2", "blue", "p3")
		if str(got.UserID) != user {
			t.Fatalf("UserID = %s after Update, want %s kept", str(got.UserID), user)
		}
	})
}

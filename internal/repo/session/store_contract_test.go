package session

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
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

// Updating a session that does not exist, or no longer does, reports
// repo.ErrNotFound rather than succeeding or failing opaquely.
func TestStore_UpdateMissingSessionIsNotFound(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		user := fmt.Sprintf("u-%d", time.Now().UnixNano())
		created, err := store.New(models.CreateSession{UserID: user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := store.Delete(created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		for _, id := range []string{created.ID, "no-such-session"} {
			err := store.Update(id, &models.UpdateSession{GameID: "g1"})
			if !errors.Is(err, repoErrors.ErrNotFound) {
				t.Errorf("Update(%q) = %v, want repo.ErrNotFound", id, err)
			}
		}
	})
}

// Moving a session to another user moves it in every lookup: it is found by
// its new user, and it is that user's one session.
func TestStore_UpdateMovesTheSessionToItsNewUser(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		from := fmt.Sprintf("u-from-%d", time.Now().UnixNano())
		to := fmt.Sprintf("u-to-%d", time.Now().UnixNano())
		created, err := store.New(models.CreateSession{UserID: from})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Update(created.ID, &models.UpdateSession{UserID: to}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		found, err := store.FindFirst("userId", to)
		if err != nil || found.ID != created.ID {
			t.Fatalf("FindFirst(userId=%s) = %v, %v; want session %s", to, found, err, created.ID)
		}

		again, err := store.New(models.CreateSession{UserID: to})
		if err != nil {
			t.Fatalf("New for the new user: %v", err)
		}
		if again.ID != created.ID {
			t.Fatalf("New for the new user created session %s; want its existing session %s", again.ID, created.ID)
		}
	})
}

// A user has one session, so moving a session onto a user who already has
// one is refused.
func TestStore_UpdateRefusesAUserWhoHasASession(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		a := fmt.Sprintf("u-a-%d", time.Now().UnixNano())
		b := fmt.Sprintf("u-b-%d", time.Now().UnixNano())
		sa, err := store.New(models.CreateSession{UserID: a})
		if err != nil {
			t.Fatalf("New a: %v", err)
		}
		sb, err := store.New(models.CreateSession{UserID: b})
		if err != nil {
			t.Fatalf("New b: %v", err)
		}

		if err := store.Update(sa.ID, &models.UpdateSession{UserID: b}); err == nil {
			t.Fatal("Update moved a session onto a user who already has one")
		}

		if found, err := store.FindFirst("userId", b); err != nil || found.ID != sb.ID {
			t.Fatalf("FindFirst(userId=%s) = %v, %v; want session %s", b, found, err, sb.ID)
		}
	})
}

// backdateSession makes a stored session older than sessionMaxAge.
func backdateSession(t *testing.T, store Storer, id string) {
	t.Helper()

	expired := time.Now().Add(-sessionMaxAge - time.Hour)
	switch s := store.(type) {
	case *MemoryStore:
		s.mu.Lock()
		s.byID[id].CreatedAt = expired
		s.mu.Unlock()
	case *SQLiteStore:
		res, err := s.db.Exec(`UPDATE sessions SET created_at = ? WHERE id = ?`, expired.UnixNano(), id)
		if err != nil {
			t.Fatalf("backdating session: %v", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			t.Fatalf("backdating session: want 1 row affected, got %d", n)
		}
	case *PostgresStore:
		backdatePostgresSession(t, s, id)
	case *MongoStore:
		t.Skip("MongoDB expires sessions through a TTL index whose background sweep is asynchronous, so a backdated session stays readable until the sweep runs")
	default:
		t.Fatalf("no backdating hook for %T", store)
	}
}

// A session older than sessionMaxAge has expired: no read finds it, and New
// gives its user a fresh session.
func TestStore_ExpiredSessionIsGone(t *testing.T) {
	eachStore(t, func(t *testing.T, store Storer) {
		user := fmt.Sprintf("u-%d", time.Now().UnixNano())
		token := "tok-" + user
		old, err := store.New(models.CreateSession{UserID: user, Token: token})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		backdateSession(t, store, old.ID)

		if _, err := store.Get(old.ID); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("Get = %v, want repo.ErrNotFound", err)
		}
		if _, err := store.GetByToken(token); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("GetByToken = %v, want repo.ErrNotFound", err)
		}
		if _, err := store.FindFirst("userId", user); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("FindFirst(userId) = %v, want repo.ErrNotFound", err)
		}
		if exists, err := store.Exists(old.ID); err != nil || exists {
			t.Errorf("Exists = %v, %v; want false, nil", exists, err)
		}

		fresh, err := store.New(models.CreateSession{UserID: user, Token: "tok2-" + user})
		if err != nil {
			t.Fatalf("New after expiry: %v", err)
		}
		if fresh.ID == old.ID {
			t.Fatalf("New returned the expired session %s", old.ID)
		}
		if got, err := store.FindFirst("userId", user); err != nil || got.ID != fresh.ID {
			t.Fatalf("FindFirst(userId) = %v, %v; want the fresh session %s", got, err, fresh.ID)
		}
	})
}

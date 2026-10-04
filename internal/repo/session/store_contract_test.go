package session

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/repo/repotest"
)

// The rules a session store keeps — one session per user, a bearer token that
// stops working when the session expires or is deleted, updates that change
// only what they are given, and errors callers can test with errors.Is —
// belong to the store, not to a database. Each test runs against every
// backend: memory and SQLite always, MongoDB and PostgreSQL wherever they are
// reachable.

func forEachBackend(t *testing.T, test func(t *testing.T, store Storer)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		test(t, NewMemoryStore())
	})

	t.Run("mongodb", func(t *testing.T) {
		store, err := NewStore(context.Background(), repotest.Mongo(t))
		if err != nil {
			t.Fatalf("creating the MongoDB session store: %v", err)
		}

		test(t, store)
	})

	t.Run("sqlite", func(t *testing.T) {
		test(t, NewSQLiteStore(context.Background(), repotest.SQLite(t)))
	})

	t.Run("postgres", func(t *testing.T) {
		store, err := NewPostgresStore(context.Background(), repotest.Postgres(t))
		if err != nil {
			t.Fatalf("creating the PostgreSQL session store: %v", err)
		}

		test(t, store)
	})
}

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return *s
}

// assertPlace checks the session's game, team and slot.
func assertPlace(t *testing.T, s *models.Session, game, team, slot string) {
	t.Helper()

	if str(s.GameID) != game || str(s.TeamID) != team || str(s.SlotID) != slot {
		t.Fatalf("session place = game %s, team %s, slot %s; want %s, %s, %s",
			str(s.GameID), str(s.TeamID), str(s.SlotID), game, team, slot)
	}
}

// uniqueUser keeps tests apart in the MongoDB database they share.
func uniqueUser() string {
	return "user-" + ids.New()
}

func TestNew_KeepsEveryField(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()

		created, err := store.New(models.CreateSession{
			UserID: user, Token: "tok-" + user, GameID: "g1", TeamID: "red", SlotID: "p0",
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		for name, get := range map[string]func() (*models.Session, error){
			"Get":        func() (*models.Session, error) { return store.Get(created.ID) },
			"GetByToken": func() (*models.Session, error) { return store.GetByToken("tok-" + user) },
			"FindFirst":  func() (*models.Session, error) { return store.FindFirst("userId", user) },
		} {
			got, err := get()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}

			assertPlace(t, got, "g1", "red", "p0")

			if got.ID != created.ID || str(got.UserID) != user {
				t.Fatalf("%s found %s for user %s, want %s for %s", name, got.ID, str(got.UserID), created.ID, user)
			}

			// Token is json:"-", so a store that keeps the session as JSON has
			// to keep it some other way; reconnect depends on it.
			if got.Token != "tok-"+user {
				t.Fatalf("%s returned token %q, want tok-%s", name, got.Token, user)
			}
		}

		if exists, err := store.Exists(created.ID); err != nil || !exists {
			t.Fatalf("Exists = %v, %v; want true, nil", exists, err)
		}
	})
}

func TestNew_RefusesASessionWithNoUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if _, err := store.New(models.CreateSession{Token: "tok-" + ids.New()}); err == nil {
			t.Fatal("New with no user id succeeded")
		}
	})
}

// A user has one session: logging in again returns the session they have.
func TestNew_ReturnsTheUsersExistingSession(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()

		first, err := store.New(models.CreateSession{UserID: user, Token: "tok-1-" + user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		again, err := store.New(models.CreateSession{UserID: user, Token: "tok-2-" + user})
		if err != nil {
			t.Fatalf("New again: %v", err)
		}

		if again.ID != first.ID {
			t.Fatalf("a second New for %s made session %s, want the existing %s", user, again.ID, first.ID)
		}
	})
}

// Concurrent logins for one user must end with one session between them, which
// only the unique index on the user can guarantee.
func TestNew_OneSessionPerUserUnderARace(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()

		const racers = 8

		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			seen = map[string]bool{}
		)

		for i := range racers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				s, err := store.New(models.CreateSession{UserID: user, Token: ids.New()})
				if err != nil {
					t.Errorf("racing New %d: %v", i, err)

					return
				}

				mu.Lock()
				seen[s.ID] = true
				mu.Unlock()
			}()
		}

		wg.Wait()

		if len(seen) != 1 {
			t.Fatalf("racing logins for one user returned %d sessions, want 1: %v", len(seen), seen)
		}
	})
}

// Update changes only the fields it is given: the session keeps its user and
// whatever it is not told about.
func TestUpdate_SetsOnlyTheFieldsItIsGiven(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()

		created, err := store.New(models.CreateSession{UserID: user, Token: "tok-" + user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Update(created.ID, &models.UpdateSession{GameID: "g2", TeamID: "blue", SlotID: "p3"}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if err := store.Update(created.ID, &models.UpdateSession{TeamID: "red"}); err != nil {
			t.Fatalf("Update team: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assertPlace(t, got, "g2", "red", "p3")

		if str(got.UserID) != user || got.Token != "tok-"+user {
			t.Fatalf("Update lost the user or token: user %s, token %q", str(got.UserID), got.Token)
		}
	})
}

// Updating a session that does not exist, or no longer does, reports
// repo.ErrNotFound rather than succeeding or failing opaquely.
func TestUpdate_ReportsAMissingSessionAsNotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		created, err := store.New(models.CreateSession{UserID: uniqueUser()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Delete(created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		for _, id := range []string{created.ID, "no-such-session"} {
			if err := store.Update(id, &models.UpdateSession{GameID: "g1"}); !errors.Is(err, repoErrors.ErrNotFound) {
				t.Errorf("Update(%q) = %v, want repo.ErrNotFound", id, err)
			}
		}
	})
}

// Moving a session to another user moves it in every lookup: it is found by
// its new user, and it is that user's one session.
func TestUpdate_MovesTheSessionToItsNewUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		from, to := uniqueUser(), uniqueUser()

		created, err := store.New(models.CreateSession{UserID: from})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Update(created.ID, &models.UpdateSession{UserID: to}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if found, err := store.FindFirst("userId", to); err != nil || found.ID != created.ID {
			t.Fatalf("FindFirst(userId=%s) = %v, %v; want session %s", to, found, err, created.ID)
		}

		again, err := store.New(models.CreateSession{UserID: to})
		if err != nil {
			t.Fatalf("New for the new user: %v", err)
		}

		if again.ID != created.ID {
			t.Fatalf("New for the new user made session %s; want its existing session %s", again.ID, created.ID)
		}
	})
}

// A user has one session, so moving a session onto a user who already has one
// is refused.
func TestUpdate_RefusesAUserWhoHasASession(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		a, b := uniqueUser(), uniqueUser()

		sa, err := store.New(models.CreateSession{UserID: a})
		if err != nil {
			t.Fatalf("New a: %v", err)
		}

		sb, err := store.New(models.CreateSession{UserID: b})
		if err != nil {
			t.Fatalf("New b: %v", err)
		}

		if err := store.Update(sa.ID, &models.UpdateSession{UserID: b}); !errors.Is(err, repoErrors.ErrDuplicate) {
			t.Fatalf("Update onto a user who already has a session = %v, want repo.ErrDuplicate", err)
		}

		if found, err := store.FindFirst("userId", b); err != nil || found.ID != sb.ID {
			t.Fatalf("FindFirst(userId=%s) = %v, %v; want session %s", b, found, err, sb.ID)
		}
	})
}

func TestLookups_ReportAMissingSessionAsNotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if _, err := store.Get("no-such-session"); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("Get(missing) = %v, want repo.ErrNotFound", err)
		}

		if _, err := store.GetByToken("no-such-token"); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("GetByToken(missing) = %v, want repo.ErrNotFound", err)
		}

		if _, err := store.FindFirst("userId", uniqueUser()); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("FindFirst(no match) = %v, want repo.ErrNotFound", err)
		}

		if _, err := store.GetByToken(""); err == nil {
			t.Error("GetByToken(\"\") succeeded; an empty token must never match")
		}
	})
}

// Find answers the lookups the server makes — everyone in a game, a user's
// session — and refuses a key it cannot look up rather than finding nothing.
func TestFind_ByGameAndUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		game := "game-" + ids.New()
		in := map[string]bool{}

		for range 3 {
			s, err := store.New(models.CreateSession{UserID: uniqueUser(), GameID: game})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			in[s.ID] = true
		}

		if _, err := store.New(models.CreateSession{UserID: uniqueUser(), GameID: "game-" + ids.New()}); err != nil {
			t.Fatalf("New elsewhere: %v", err)
		}

		// Joining moves a session into the game with Update, and only the new
		// game must find it afterwards.
		moved, err := store.New(models.CreateSession{UserID: uniqueUser()})
		if err != nil {
			t.Fatalf("New lobby: %v", err)
		}

		if err := store.Update(moved.ID, &models.UpdateSession{GameID: game}); err != nil {
			t.Fatalf("Update into the game: %v", err)
		}

		in[moved.ID] = true

		mover, err := store.New(models.CreateSession{UserID: uniqueUser(), GameID: game})
		if err != nil {
			t.Fatalf("New mover: %v", err)
		}

		if err := store.Update(mover.ID, &models.UpdateSession{GameID: "game-" + ids.New()}); err != nil {
			t.Fatalf("Update out of the game: %v", err)
		}

		// An expired session in the game must not receive its broadcasts.
		expired, err := store.New(models.CreateSession{UserID: uniqueUser(), GameID: game})
		if err != nil {
			t.Fatalf("New expired: %v", err)
		}

		backdate(t, store, expired.ID, time.Now().Add(-sessionMaxAge-time.Minute))

		if found, err := store.Find("gameId", "game-none-"+ids.New()); err != nil || len(found) != 0 {
			t.Fatalf("Find(gameId) for an empty game = %v, %v; want none", found, err)
		}

		found, err := store.Find("gameId", game)
		if err != nil {
			t.Fatalf("Find(gameId): %v", err)
		}

		if len(found) != len(in) {
			t.Fatalf("Find(gameId) found %d sessions, want %d", len(found), len(in))
		}

		for _, s := range found {
			if !in[s.ID] {
				t.Errorf("Find(gameId) returned %s, which is not in the game", s.ID)
			}
		}

		if _, err := store.Find("token", "anything"); err == nil {
			t.Error("Find by token succeeded; tokens are looked up with GetByToken only")
		}
	})
}

// Deleting a session invalidates its bearer token: logout's guarantee.
func TestDelete_InvalidatesTheToken(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()
		token := "tok-" + user

		created, err := store.New(models.CreateSession{Token: token, UserID: user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.Delete(created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if _, err := store.GetByToken(token); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Fatalf("GetByToken after Delete = %v, want repo.ErrNotFound", err)
		}

		if err := store.Delete(created.ID); err != nil {
			t.Errorf("deleting an already-deleted session = %v, want nil", err)
		}
	})
}

// A session older than sessionMaxAge has expired: no read finds it, its token
// is dead, and New gives its user a fresh session.
func TestExpiredSessionIsGone(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()
		token := "tok-" + user

		old, err := store.New(models.CreateSession{UserID: user, Token: token})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		backdate(t, store, old.ID, time.Now().Add(-sessionMaxAge-time.Minute))

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

		if err := store.Update(old.ID, &models.UpdateSession{GameID: "g1"}); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("Update = %v, want repo.ErrNotFound", err)
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

// A session just short of sessionMaxAge is still alive.
func TestSessionShortOfMaxAgeIsAlive(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user := uniqueUser()

		s, err := store.New(models.CreateSession{UserID: user, Token: "tok-" + user})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		backdate(t, store, s.ID, time.Now().Add(-sessionMaxAge+time.Minute))

		if _, err := store.GetByToken("tok-" + user); err != nil {
			t.Fatalf("GetByToken a minute before expiry = %v, want the session", err)
		}
	})
}

// Updates to different fields of one session, racing, must all land: a store
// that read the whole session, changed one field and wrote the whole thing
// back would put a stale copy of the others over a concurrent update.
func TestUpdate_ConcurrentUpdatesAreNotLost(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		created, err := store.New(models.CreateSession{UserID: uniqueUser()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		for round := range 20 {
			want := models.UpdateSession{
				GameID: "g" + strconv.Itoa(round),
				TeamID: "t" + strconv.Itoa(round),
				SlotID: "p" + strconv.Itoa(round),
			}

			var wg sync.WaitGroup

			for _, u := range []models.UpdateSession{{GameID: want.GameID}, {TeamID: want.TeamID}, {SlotID: want.SlotID}} {
				wg.Add(1)

				go func() {
					defer wg.Done()

					if err := store.Update(created.ID, &u); err != nil {
						t.Errorf("round %d Update(%+v): %v", round, u, err)
					}
				}()
			}

			wg.Wait()

			got, err := store.Get(created.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			assertPlace(t, got, want.GameID, want.TeamID, want.SlotID)
		}
	})
}

// LeaveGame takes the session out of its game, team and slot, and keeps its
// user and token.
func TestLeaveGame_ClearsTheGameTeamAndSlot(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		user, game := uniqueUser(), "game-"+ids.New()

		created, err := store.New(models.CreateSession{UserID: user, Token: "tok-" + user, GameID: game, TeamID: "red", SlotID: "p0"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.LeaveGame(created.ID, game); err != nil {
			t.Fatalf("LeaveGame: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assertPlace(t, got, "<nil>", "<nil>", "<nil>")

		if str(got.UserID) != user || got.Token != "tok-"+user {
			t.Fatalf("LeaveGame lost the user or token: user %s, token %q", str(got.UserID), got.Token)
		}

		// The game's broadcasts look sessions up by game; a session that left
		// must not be found there.
		if found, err := store.Find("gameId", game); err != nil || len(found) != 0 {
			t.Fatalf("Find(gameId) after LeaveGame = %d sessions, %v; want none", len(found), err)
		}
	})
}

// LeaveGame for a game the session is no longer in changes nothing, so a late
// kick from one game cannot take the player out of the game they moved to.
func TestLeaveGame_IgnoresAnotherGame(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		created, err := store.New(models.CreateSession{UserID: uniqueUser(), GameID: "g2", TeamID: "blue", SlotID: "p3"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if err := store.LeaveGame(created.ID, "g1"); err != nil {
			t.Fatalf("LeaveGame: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		assertPlace(t, got, "g2", "blue", "p3")
	})
}

// Leaving is idempotent: a session that no longer exists is already out of
// every game.
func TestLeaveGame_OnAMissingSessionSucceeds(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if err := store.LeaveGame("missing-"+ids.New(), "g1"); err != nil {
			t.Fatalf("LeaveGame: %v", err)
		}
	})
}

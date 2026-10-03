package user

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/repo/repotest"
)

// The rules a user store keeps — one account per email, a password that
// survives every read and every unrelated edit, and errors callers can test
// with errors.Is — belong to the store, not to a database. Each test runs
// against every backend: memory and SQLite always, MongoDB and PostgreSQL
// wherever they are reachable.

func forEachBackend(t *testing.T, test func(t *testing.T, store Storer)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		test(t, NewMemoryStore())
	})

	t.Run("mongodb", func(t *testing.T) {
		store, err := NewStore(context.Background(), repotest.Mongo(t))
		if err != nil {
			t.Fatalf("creating the MongoDB user store: %v", err)
		}

		test(t, store)
	})

	t.Run("sqlite", func(t *testing.T) {
		test(t, NewSQLiteStore(context.Background(), repotest.SQLite(t)))
	})

	t.Run("postgres", func(t *testing.T) {
		store, err := NewPostgresStore(context.Background(), repotest.Postgres(t))
		if err != nil {
			t.Fatalf("creating the PostgreSQL user store: %v", err)
		}

		test(t, store)
	})
}

// uniqueEmail keeps tests apart in the MongoDB database they share.
func uniqueEmail() string {
	return ids.New() + "@example.test"
}

func ptr(s string) *string {
	return &s
}

// storeUser stores a user with a password and fails the test if it cannot.
func storeUser(t *testing.T, store Storer, email string) *models.User {
	t.Helper()

	u, err := store.New(models.CreateUser{Email: email, Name: "Ada", DisplayName: ptr("Countess"), Password: ptr("hash-1")})
	if err != nil {
		t.Fatalf("New(%s): %v", email, err)
	}

	return u
}

func TestNew_StoresEveryField(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		email := uniqueEmail()
		created := storeUser(t, store, email)

		if created.ID == "" {
			t.Fatal("New returned a user with no id")
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		if got.Email != email || got.Name != "Ada" || got.DisplayName == nil || *got.DisplayName != "Countess" {
			t.Errorf("stored user = %+v, want email %s, name Ada, display name Countess", got, email)
		}

		// Password is json:"-", so a store that keeps the user as JSON has to
		// keep it some other way, and login depends on it being there.
		if got.Password == nil || *got.Password != "hash-1" {
			t.Errorf("password = %v after a round trip, want hash-1", got.Password)
		}

		if exists, err := store.Exists(created.ID); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true, nil", exists, err)
		}
	})
}

func TestNew_RefusesATakenEmail(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		email := uniqueEmail()
		storeUser(t, store, email)

		if _, err := store.New(models.CreateUser{Email: email, Name: "Imposter"}); !errors.Is(err, repoErrors.ErrDuplicate) {
			t.Fatalf("New with a taken email = %v, want repo.ErrDuplicate", err)
		}
	})
}

// Two registrations racing for one email: the unique index, not a check in Go
// that both could pass, decides — exactly one account is created.
func TestNew_OneAccountPerEmailUnderARace(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		email := uniqueEmail()

		const racers = 8

		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			created  int
			failures []error
		)

		for range racers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				_, err := store.New(models.CreateUser{Email: email, Name: "Racer"})

				mu.Lock()
				defer mu.Unlock()

				switch {
				case err == nil:
					created++
				case !errors.Is(err, repoErrors.ErrDuplicate):
					failures = append(failures, err)
				}
			}()
		}

		wg.Wait()

		if created != 1 {
			t.Errorf("%d of %d racing registrations succeeded, want exactly 1", created, racers)
		}

		for _, err := range failures {
			t.Errorf("a losing registration failed with %v, want repo.ErrDuplicate", err)
		}
	})
}

func TestLookups_ReportAMissingUserAsNotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if _, err := store.Get("no-such-user"); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("Get(missing) = %v, want repo.ErrNotFound", err)
		}

		if _, err := store.FindFirst("email", uniqueEmail()); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Errorf("FindFirst(no match) = %v, want repo.ErrNotFound", err)
		}

		if exists, err := store.Exists("no-such-user"); err != nil || exists {
			t.Errorf("Exists(missing) = %v, %v; want false, nil", exists, err)
		}
	})
}

// Login looks a user up by email and checks the password it gets back, so the
// lookup path has to return the password too, not only Get.
func TestFindFirst_ByEmailReturnsTheWholeUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		email := uniqueEmail()
		created := storeUser(t, store, email)

		found, err := store.FindFirst("email", email)
		if err != nil {
			t.Fatalf("FindFirst: %v", err)
		}

		if found.ID != created.ID {
			t.Fatalf("FindFirst found %s, want %s", found.ID, created.ID)
		}

		if found.Password == nil || *found.Password != "hash-1" {
			t.Errorf("password = %v from FindFirst, want hash-1", found.Password)
		}

		all, err := store.Find("email", email)
		if err != nil || len(all) != 1 || all[0].ID != created.ID {
			t.Errorf("Find = %v, %v; want exactly %s", all, err, created.ID)
		}
	})
}

func TestFind_RefusesAKeyItCannotLookUp(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if _, err := store.Find("password", "hash-1"); err == nil {
			t.Error("Find by password succeeded, want an error naming the key")
		}

		if _, err := store.FindFirst("password", "hash-1"); err == nil {
			t.Error("FindFirst by password succeeded, want an error naming the key")
		}
	})
}

// An update changes the fields it is given and nothing else. A password is
// the field that hurts to lose: an unrelated profile edit that blanked it
// would lock the user out on their next login.
func TestUpdate_ChangesOnlyTheFieldsItIsGiven(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		created := storeUser(t, store, uniqueEmail())

		if err := store.Update(&models.UpdateUser{ID: created.ID, Name: "Ada L."}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		if got.Name != "Ada L." {
			t.Errorf("name = %q, want Ada L.", got.Name)
		}

		if got.Email != created.Email || got.DisplayName == nil || *got.DisplayName != "Countess" {
			t.Errorf("an update of the name changed other fields: %+v", got)
		}

		if got.Password == nil || *got.Password != "hash-1" {
			t.Errorf("password = %v after an update that omitted it, want hash-1", got.Password)
		}

		if err := store.Update(&models.UpdateUser{ID: created.ID, Password: ptr("hash-2")}); err != nil {
			t.Fatalf("Update password: %v", err)
		}

		if got, _ := store.FindFirst("email", created.Email); got == nil || got.Password == nil || *got.Password != "hash-2" {
			t.Errorf("password after changing it = %v, want hash-2", got)
		}
	})
}

func TestUpdate_ReportsAMissingUserAsNotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		if err := store.Update(&models.UpdateUser{ID: "no-such-user", Name: "Nobody"}); !errors.Is(err, repoErrors.ErrNotFound) {
			t.Fatalf("Update(missing) = %v, want repo.ErrNotFound", err)
		}
	})
}

func TestUpdate_RefusesATakenEmail(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		first := storeUser(t, store, uniqueEmail())
		second := storeUser(t, store, uniqueEmail())

		if err := store.Update(&models.UpdateUser{ID: second.ID, Email: first.Email}); !errors.Is(err, repoErrors.ErrDuplicate) {
			t.Fatalf("Update onto a taken email = %v, want repo.ErrDuplicate", err)
		}

		if got, _ := store.Get(second.ID); got == nil || got.Email != second.Email {
			t.Errorf("a refused update still changed the email: %+v", got)
		}

		// Moving to a free email is fine, and the old one is then free too.
		moved := uniqueEmail()
		if err := store.Update(&models.UpdateUser{ID: second.ID, Email: moved}); err != nil {
			t.Fatalf("Update onto a free email: %v", err)
		}

		if found, err := store.FindFirst("email", moved); err != nil || found.ID != second.ID {
			t.Errorf("FindFirst(new email) = %v, %v; want %s", found, err, second.ID)
		}

		if _, err := store.New(models.CreateUser{Email: second.Email, Name: "Next"}); err != nil {
			t.Errorf("the email a user moved off is still taken: %v", err)
		}
	})
}

// Updates to different fields of one user, racing, must all land. A lost
// password change is the dangerous one: the old password would work again.
func TestUpdate_ConcurrentUpdatesAreNotLost(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store Storer) {
		created := storeUser(t, store, uniqueEmail())

		for round := range 20 {
			name, display, password := "n"+strconv.Itoa(round), "d"+strconv.Itoa(round), "p"+strconv.Itoa(round)

			var wg sync.WaitGroup

			for _, u := range []models.UpdateUser{
				{ID: created.ID, Name: name},
				{ID: created.ID, DisplayName: ptr(display)},
				{ID: created.ID, Password: ptr(password)},
			} {
				wg.Add(1)

				go func() {
					defer wg.Done()

					if err := store.Update(&u); err != nil {
						t.Errorf("round %d Update: %v", round, err)
					}
				}()
			}

			wg.Wait()

			got, err := store.Get(created.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			if got.Name != name || got.DisplayName == nil || *got.DisplayName != display ||
				got.Password == nil || *got.Password != password {
				t.Fatalf("round %d: user = name %q, display %v, password %v; want %s, %s, %s",
					round, got.Name, got.DisplayName, got.Password, name, display, password)
			}
		}
	})
}

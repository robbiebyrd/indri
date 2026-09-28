package user

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestUserSQLiteStore_New_AssignsID(t *testing.T) {
	s := newSQLiteFixture(t)
	u, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID empty")
	}
}

func TestUserSQLiteStore_New_DuplicateEmail(t *testing.T) {
	s := newSQLiteFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice2"})
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestUserSQLiteStore_Get_UnknownID(t *testing.T) {
	s := newSQLiteFixture(t)
	_, err := s.Get("nope")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserSQLiteStore_FindFirst(t *testing.T) {
	s := newSQLiteFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	got, err := s.FindFirst("email", "a@b.c")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindFirst: got=%v err=%v", got, err)
	}
}

func TestUserSQLiteStore_Exists(t *testing.T) {
	s := newSQLiteFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	ok, _ := s.Exists(u.ID)
	if !ok {
		t.Fatal("Exists = false for created user")
	}
	ok, _ = s.Exists("nope")
	if ok {
		t.Fatal("Exists = true for unknown id")
	}
}

func TestUserSQLiteStore_Find_ByEmail(t *testing.T) {
	s := newSQLiteFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, _ = s.New(models.CreateUser{Email: "b@c.d", Name: "Bob"})
	list, err := s.Find("email", "a@b.c")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(list) != 1 || list[0].Email != "a@b.c" {
		t.Fatalf("want 1 result with email a@b.c, got %v", list)
	}
}

func TestUserSQLiteStore_Update(t *testing.T) {
	s := newSQLiteFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	newName := "Alice2"
	err := s.Update(&models.UpdateUser{ID: u.ID, Name: newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(u.ID)
	if got.Name != newName {
		t.Errorf("Name = %q; want %q", got.Name, newName)
	}
}

func TestUserSQLiteStore_Update_PasswordRoundtrips(t *testing.T) {
	s := newSQLiteFixture(t)
	pw := "secret"
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice", Password: &pw})

	// Password is json:"-" — it must survive via the separate column.
	got, _ := s.Get(u.ID)
	if got.Password == nil || *got.Password != pw {
		t.Errorf("Password not preserved: got %v", got.Password)
	}
}

func TestUserSQLiteStore_Update_UnknownID(t *testing.T) {
	s := newSQLiteFixture(t)
	err := s.Update(&models.UpdateUser{ID: "nope", Name: "x"})
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

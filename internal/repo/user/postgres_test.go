package user

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func postgresUserURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}
	return uri
}

func newPostgresUserFixture(t *testing.T) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(context.Background(), postgresUserURI(t))
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec("TRUNCATE TABLE users")
		_ = store.db.Close()
	})
	return store
}

func TestUserPostgresStore_New_AssignsID(t *testing.T) {
	s := newPostgresUserFixture(t)
	u, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID empty")
	}
}

func TestUserPostgresStore_New_DuplicateEmail(t *testing.T) {
	s := newPostgresUserFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice2"})
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestUserPostgresStore_Get_UnknownID(t *testing.T) {
	s := newPostgresUserFixture(t)
	_, err := s.Get("nope")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserPostgresStore_FindFirst(t *testing.T) {
	s := newPostgresUserFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	got, err := s.FindFirst("email", "a@b.c")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindFirst: got=%v err=%v", got, err)
	}
}

// TestUserPostgresStore_New_PasswordRoundtrips proves models.User.Password
// (json:"-", so it is dropped from the JSONB blob) survives a New -> Get
// round-trip via the dedicated password column.
func TestUserPostgresStore_New_PasswordRoundtrips(t *testing.T) {
	s := newPostgresUserFixture(t)
	pw := "hunter2"
	created, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice", Password: &pw})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if created.Password == nil || *created.Password != pw {
		t.Fatalf("New: password not returned, got %v", created.Password)
	}

	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Password == nil || *got.Password != pw {
		t.Fatalf("Get: password did not round-trip, got %v", got.Password)
	}
}

// TestUserPostgresStore_FindFirst_PasswordRoundtrips proves the password
// column is also restored on the Find/FindFirst read path, not just Get.
func TestUserPostgresStore_FindFirst_PasswordRoundtrips(t *testing.T) {
	s := newPostgresUserFixture(t)
	pw := "hunter2"
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice", Password: &pw})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := s.FindFirst("email", "a@b.c")
	if err != nil {
		t.Fatalf("FindFirst: %v", err)
	}
	if got.Password == nil || *got.Password != pw {
		t.Fatalf("FindFirst: password did not round-trip, got %v", got.Password)
	}
}

// TestUserPostgresStore_Update_PasswordRoundtrips proves Update writes a new
// password to the dedicated column and that it survives a subsequent Get.
func TestUserPostgresStore_Update_PasswordRoundtrips(t *testing.T) {
	s := newPostgresUserFixture(t)
	oldPw := "old-pw"
	created, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice", Password: &oldPw})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	newPw := "new-pw"
	if err := s.Update(&models.UpdateUser{ID: created.ID, Password: &newPw}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Password == nil || *got.Password != newPw {
		t.Fatalf("Update: password did not update, got %v", got.Password)
	}
}

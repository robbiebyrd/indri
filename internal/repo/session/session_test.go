package session

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
)

// newTestStore connects to a local MongoDB and skips the test when none is
// reachable, so CI without a database stays green.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}
	_ = os.Setenv("INDRI_MONGO_URI", uri)
	_ = os.Setenv("INDRI_MONGO_DATABASE", "indri_test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongodb.New(ctx)
	if err != nil {
		t.Skipf("skipping: MongoDB not reachable: %v", err)
	}

	store, err := NewStore(context.Background(), client)
	if err != nil {
		t.Skipf("skipping: could not create session store: %v", err)
	}

	return store
}

// TestDelete_InvalidatesToken proves logout's security guarantee at the store
// level: once a session is deleted, its bearer token can no longer resolve a
// session, so reconnect cannot resume it.
func TestDelete_InvalidatesToken(t *testing.T) {
	store := newTestStore(t)

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	token := "tok-" + unique

	created, err := store.New(models.CreateSession{
		Token:  token,
		UserID: "user-" + unique,
	})
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}

	if _, err := store.GetByToken(token); err != nil {
		t.Fatalf("token should resolve before delete: %v", err)
	}

	if err := store.Delete(created.ID.Hex()); err != nil {
		t.Fatalf("deleting session: %v", err)
	}

	if _, err := store.GetByToken(token); err == nil {
		t.Fatal("token still resolves after delete — session was not invalidated")
	}

	// Delete is idempotent.
	if err := store.Delete(created.ID.Hex()); err != nil {
		t.Errorf("second delete should be a no-op, got: %v", err)
	}
}

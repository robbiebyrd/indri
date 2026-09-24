package game

import (
	"fmt"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
)

func TestAddPlayer_SetsUserID(t *testing.T) {
	store := newTestStore(t)

	code := fmt.Sprintf("userid-test-%d", time.Now().UnixNano())
	g, err := store.New(code, &models.Script{}, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}

	userId := "user-uid-1"
	if err := store.AddPlayer(g.ID.Hex(), userId, "Alice"); err != nil {
		t.Fatalf("AddPlayer: %v", err)
	}

	final, err := store.Get(g.ID.Hex())
	if err != nil {
		t.Fatalf("reloading game: %v", err)
	}

	p, ok := final.Players[userId]
	if !ok {
		t.Fatalf("player %q not found in game", userId)
	}
	if p.UserID != userId {
		t.Fatalf("expected UserID=%q, got %q", userId, p.UserID)
	}
}

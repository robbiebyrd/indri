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
	script := &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"TeamA": {Name: "TeamA"}},
	}
	g, err := store.New(code, script, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}

	userId := "user-uid-1"
	slotId, err := store.AssignSlot(g.ID, "TeamA", userId, "Alice")
	if err != nil {
		t.Fatalf("AssignSlot: %v", err)
	}

	final, err := store.Get(g.ID)
	if err != nil {
		t.Fatalf("reloading game: %v", err)
	}

	p, ok := final.Players[slotId]
	if !ok {
		t.Fatalf("slot %q not found in game", slotId)
	}
	if p.UserID != userId {
		t.Fatalf("expected UserID=%q, got %q", userId, p.UserID)
	}
}

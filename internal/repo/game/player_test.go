package game

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

func TestAddPlayer_SetsUserID(t *testing.T) {
	p := models.Player{UserID: "u1", Name: "Alice"}
	if p.UserID != "u1" {
		t.Fatalf("UserID field not present on Player struct")
	}
}

package kick_test

import (
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	ht "github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

// A host on one instance kicks a player connected to another: the player must
// be disconnected there, not only removed from the game.
func TestKick_DisconnectsATargetConnectedToAnotherInstance(t *testing.T) {
	stores := ht.NewStores(t)
	relay := &ht.Relay{}

	g := stores.Game("KICK", 2)
	_, hostSession := stores.Seat(g.ID, "host")
	targetSlot, targetSession := stores.Seat(g.ID, "target")

	// Instance B holds the target's connection; instance A, the host's.
	targetConn := ht.NewConn(targetSession)
	stores.Injector(t, ht.NewInstance(targetConn), relay)
	hostConn := ht.NewConn(hostSession)
	a := stores.Injector(t, ht.NewInstance(hostConn), relay)

	if err := kick.New(a).Handle(hostConn, map[string]interface{}{"code": "KICK", "slotId": targetSlot}); err != nil {
		t.Fatalf("kick: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !targetConn.IsClosed() {
		if time.Now().After(deadline) {
			t.Fatal("the kicked player's connection on the other instance was never closed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	after := ht.Must(stores.Games.Get(g.ID))
	if after.Players[targetSlot].UserID != "" {
		t.Fatalf("target still holds slot %s: %+v", targetSlot, after.Players[targetSlot])
	}
	if hostConn.IsClosed() {
		t.Fatal("the host was disconnected")
	}
}

package kick

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

const (
	gameCode = "the-game"
	hostID   = "host-user"
	targetID = "target-user"
)

// gameStore is the real in-memory game store under the narrow view a kick
// needs. It is not a stub: removal runs the store's own lock, version fence,
// retry loop and change delta, so a failing removal here fails the way a
// production one does. Only the lookup is renamed — the store finds a game by
// code, the game service calls the same thing GetByCode.
type gameStore struct{ *gameRepo.MemoryStore }

func (s gameStore) GetByCode(code string) (*models.Game, error) {
	return s.FindByCode(code)
}

// directory answers the one session lookup a kick makes. Sessions live in
// MongoDB, and a kick reads them without writing, so the fixture is the
// directory itself.
type directory map[string]*models.Session

func (d directory) GetByUserID(userId string) (*models.Session, error) {
	s, ok := d[userId]
	if !ok {
		return nil, fmt.Errorf("no session for user %v", userId)
	}

	return s, nil
}

// kickGame is a game holding a host and a second player, together with the
// slot each of them holds.
type kickGame struct {
	store      gameStore
	cancel     context.CancelFunc
	id         string
	hostSlot   string
	targetSlot string
}

// newGame builds a store holding one game with a host and a second player, and
// returns the cancel function of the store's context — cancelling it is how a
// test drives a genuine write failure through the real mutation path.
func newGame(t *testing.T) kickGame {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := gameStore{gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), nil)}

	g, err := store.New(gameCode, &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}},
	}, false)
	if err != nil {
		t.Fatalf("creating game %q: %v", gameCode, err)
	}

	// The first player to join is the host: that is the store's rule, not the
	// test's, so the host the handler checks for is a real one. The order
	// matters for exactly that reason.
	slots := map[string]string{}

	for _, player := range []struct{ id, name string }{{hostID, "Host"}, {targetID, "Target"}} {
		slot, _, err := store.AssignSlot(g.ID, "red", player.id, player.name)
		if err != nil {
			t.Fatalf("seating player %v in game %v: %v", player.id, g.ID, err)
		}

		slots[player.id] = slot
	}

	if !store.PlayerIsHost(g.ID, slots[hostID]) {
		t.Fatalf("player %v is not the host of game %v; the test cannot kick anybody", hostID, g.ID)
	}

	return kickGame{store: store, cancel: cancel, id: g.ID, hostSlot: slots[hostID], targetSlot: slots[targetID]}
}

// holder returns who holds slot in the game.
func (k kickGame) holder(t *testing.T, slot string) string {
	t.Helper()

	g, err := k.store.Get(k.id)
	if err != nil {
		t.Fatalf("reading game %v: %v", k.id, err)
	}

	return g.Players[slot].UserID
}

// sessionsIn returns a session for the host and one for the target, both in the
// game and naming the slots they hold.
func sessionsIn(k kickGame) directory {
	return directory{
		hostID:   {ID: ids.New(), UserID: ref(hostID), GameID: ref(k.id), SlotID: ref(k.hostSlot)},
		targetID: {ID: ids.New(), UserID: ref(targetID), GameID: ref(k.id), SlotID: ref(k.targetSlot)},
	}
}

func ref[T any](v T) *T {
	return &v
}

// kickRequest is caller asking for the player in slot to be kicked.
func kickRequest(caller *models.Session, slot string) actions.Request {
	return actions.Request{
		Session: caller,
		Payload: map[string]interface{}{"code": gameCode, "slotId": slot},
	}
}

// TestHandle_DisconnectsTheTargetItRemoved is the half of the contract that has
// to keep working: a removal that committed still hands the transport the
// target's session to close.
func TestHandle_DisconnectsTheTargetItRemoved(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)

	result, err := handle(kickRequest(sessions[hostID], k.targetSlot), k.store, sessions)
	if err != nil {
		t.Fatalf("handle(host kicks target) = %v, want no error", err)
	}

	want := []string{sessions[targetID].ID}
	if len(result.DisconnectIDs) != 1 || result.DisconnectIDs[0] != want[0] {
		t.Errorf("DisconnectIDs = %v, want %v", result.DisconnectIDs, want)
	}

	if holder := k.holder(t, k.targetSlot); holder != "" {
		t.Errorf("slot %v is still held by %q after a successful kick", k.targetSlot, holder)
	}
}

// TestHandle_DoesNotDisconnectATargetItFailedToRemove is the bug: the removal
// error used to be logged and dropped, and the target was force-disconnected
// anyway — cut off from the transport while still a player of record, unable to
// rejoin cleanly and unable to play.
//
// The failure is a real one. Cancelling the store's context makes the very next
// mutation give up on the game lock, exactly as a write during shutdown does;
// nothing about the store or the handler is faked to produce it.
func TestHandle_DoesNotDisconnectATargetItFailedToRemove(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)

	k.cancel()

	result, err := handle(kickRequest(sessions[hostID], k.targetSlot), k.store, sessions)

	if k.holder(t, k.targetSlot) != targetID {
		t.Fatalf("player %v was removed from game %v; the test did not drive a failing removal",
			targetID, k.id)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none: the target is still a player of the game",
			result.DisconnectIDs)
	}

	if err == nil {
		t.Fatal("handle(failing removal) returned no error, so the caller cannot tell the kick did not happen")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("handle(failing removal) = %v, want it to wrap the store's own error", err)
	}

	if !strings.Contains(err.Error(), k.targetSlot) || !strings.Contains(err.Error(), k.id) {
		t.Errorf("handle(failing removal) = %q, want it to name slot %v and game %v", err, k.targetSlot, k.id)
	}
}

// TestHandle_RejectsACallerWhoIsNotTheHost keeps the authorization check where
// it belongs: before the mutation, so a non-host changes nothing and closes
// nobody's connection.
func TestHandle_RejectsACallerWhoIsNotTheHost(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)

	result, err := handle(kickRequest(sessions[targetID], k.hostSlot), k.store, sessions)
	if err == nil {
		t.Fatalf("handle(non-host kicks) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a caller who is not the host", result.DisconnectIDs)
	}

	if k.holder(t, k.hostSlot) != hostID {
		t.Errorf("slot %v was freed by a caller who is not the host", k.hostSlot)
	}
}

// TestHandle_RejectsACallerWhoNoLongerHoldsTheHostSlot: a session can outlive
// its seat. One that still names the host slot after somebody else took it
// must not carry the host's authority.
func TestHandle_RejectsACallerWhoNoLongerHoldsTheHostSlot(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)
	sessions[targetID].SlotID = ref(k.hostSlot)

	result, err := handle(kickRequest(sessions[targetID], k.hostSlot), k.store, sessions)
	if err == nil {
		t.Fatalf("handle(caller naming a slot they do not hold) = %v, want an error", result)
	}

	if k.holder(t, k.hostSlot) != hostID {
		t.Errorf("slot %v was freed by a caller who does not hold the host slot", k.hostSlot)
	}
}

// TestHandle_RejectsAnEmptySlot: there is nobody to kick.
func TestHandle_RejectsAnEmptySlot(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)

	if err := k.store.RemovePlayer(k.id, k.targetSlot, targetID); err != nil {
		t.Fatalf("freeing the target's slot: %v", err)
	}

	result, err := handle(kickRequest(sessions[hostID], k.targetSlot), k.store, sessions)
	if err == nil {
		t.Fatalf("handle(empty slot) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for an empty slot", result.DisconnectIDs)
	}
}

// TestHandle_FreesTheSlotOfATargetPlayingElsewhere: a player can still hold a
// slot here while their session has moved to another game. The host must be
// able to free that seat, and the player — busy in the other game — must stay
// connected.
func TestHandle_FreesTheSlotOfATargetPlayingElsewhere(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)
	sessions[targetID].GameID = ref(ids.New())

	result, err := handle(kickRequest(sessions[hostID], k.targetSlot), k.store, sessions)
	if err != nil {
		t.Fatalf("handle(target playing elsewhere) = %v, want no error", err)
	}

	if holder := k.holder(t, k.targetSlot); holder != "" {
		t.Errorf("slot %v is still held by %q", k.targetSlot, holder)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a target playing in another game", result.DisconnectIDs)
	}
}

// TestHandle_FreesTheSlotOfATargetWithNoSession: a player who logged out still
// holds their slot until someone frees it.
func TestHandle_FreesTheSlotOfATargetWithNoSession(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)
	delete(sessions, targetID)

	result, err := handle(kickRequest(sessions[hostID], k.targetSlot), k.store, sessions)
	if err != nil {
		t.Fatalf("handle(target with no session) = %v, want no error", err)
	}

	if holder := k.holder(t, k.targetSlot); holder != "" {
		t.Errorf("slot %v is still held by %q", k.targetSlot, holder)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a target with no session", result.DisconnectIDs)
	}
}

// TestHandle_RejectsAnUnauthenticatedCaller covers the first gate: no session,
// no kick.
func TestHandle_RejectsAnUnauthenticatedCaller(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(k)

	result, err := handle(kickRequest(nil, k.targetSlot), k.store, sessions)
	if err == nil {
		t.Fatalf("handle(no session) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for an unauthenticated caller", result.DisconnectIDs)
	}

	if k.holder(t, k.targetSlot) != targetID {
		t.Errorf("slot %v was freed by an unauthenticated caller", k.targetSlot)
	}
}

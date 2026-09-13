package kick

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
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

// newGame builds a store holding one game with a host and a second player, and
// returns the cancel function of the store's context — cancelling it is how a
// test drives a genuine write failure through the real mutation path.
func newGame(t *testing.T) (gameStore, context.CancelFunc, string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := gameStore{gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), nil)}

	g, err := store.New(gameCode, nil, false)
	if err != nil {
		t.Fatalf("creating game %q: %v", gameCode, err)
	}

	id := g.ID.Hex()

	// The first player to join is the host: that is the store's rule, not the
	// test's, so the host the handler checks for is a real one. The order
	// matters for exactly that reason.
	for _, player := range []struct{ id, name string }{{hostID, "Host"}, {targetID, "Target"}} {
		if err := store.AddPlayer(id, player.id, player.name); err != nil {
			t.Fatalf("adding player %v to game %v: %v", player.id, id, err)
		}
	}

	if !store.PlayerIsHost(id, hostID) {
		t.Fatalf("player %v is not the host of game %v; the test cannot kick anybody", hostID, id)
	}

	return store, cancel, id
}

// sessionsIn returns a session for the host and one for the target, both in the
// given game.
func sessionsIn(gameId string) directory {
	return directory{
		hostID:   {ID: bson.NewObjectID(), UserID: ref(hostID), GameID: ref(gameId)},
		targetID: {ID: bson.NewObjectID(), UserID: ref(targetID), GameID: ref(gameId)},
	}
}

func ref[T any](v T) *T {
	return &v
}

// kickRequest is the host asking for the target to be kicked.
func kickRequest(caller *models.Session) actions.Request {
	return actions.Request{
		Session: caller,
		Payload: map[string]interface{}{"code": gameCode, "userId": targetID},
	}
}

// TestHandle_DisconnectsTheTargetItRemoved is the half of the contract that has
// to keep working: a removal that committed still hands the transport the
// target's session to close.
func TestHandle_DisconnectsTheTargetItRemoved(t *testing.T) {
	store, _, gameId := newGame(t)
	sessions := sessionsIn(gameId)

	result, err := handle(kickRequest(sessions[hostID]), store, sessions)
	if err != nil {
		t.Fatalf("handle(host kicks target) = %v, want no error", err)
	}

	want := []string{sessions[targetID].ID.Hex()}
	if len(result.DisconnectIDs) != 1 || result.DisconnectIDs[0] != want[0] {
		t.Errorf("DisconnectIDs = %v, want %v", result.DisconnectIDs, want)
	}

	if store.HasPlayer(gameId, targetID) {
		t.Errorf("player %v is still in game %v after a successful kick", targetID, gameId)
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
	store, cancel, gameId := newGame(t)
	sessions := sessionsIn(gameId)

	cancel()

	result, err := handle(kickRequest(sessions[hostID]), store, sessions)

	if !store.HasPlayer(gameId, targetID) {
		t.Fatalf("player %v was removed from game %v; the test did not drive a failing removal",
			targetID, gameId)
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

	if !strings.Contains(err.Error(), targetID) || !strings.Contains(err.Error(), gameId) {
		t.Errorf("handle(failing removal) = %q, want it to name player %v and game %v", err, targetID, gameId)
	}
}

// TestHandle_RejectsACallerWhoIsNotTheHost keeps the authorization check where
// it belongs: before the mutation, so a non-host changes nothing and closes
// nobody's connection.
func TestHandle_RejectsACallerWhoIsNotTheHost(t *testing.T) {
	store, _, gameId := newGame(t)
	sessions := sessionsIn(gameId)

	result, err := handle(kickRequest(sessions[targetID]), store, sessions)
	if err == nil {
		t.Fatalf("handle(non-host kicks) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a caller who is not the host", result.DisconnectIDs)
	}

	if !store.HasPlayer(gameId, targetID) {
		t.Errorf("player %v was removed from game %v by a caller who is not the host", targetID, gameId)
	}
}

// TestHandle_RejectsATargetInAnotherGame guards the other half of the check: a
// host may only kick out of their own game.
func TestHandle_RejectsATargetInAnotherGame(t *testing.T) {
	store, _, gameId := newGame(t)
	sessions := sessionsIn(gameId)
	sessions[targetID].GameID = ref(bson.NewObjectID().Hex())

	result, err := handle(kickRequest(sessions[hostID]), store, sessions)
	if err == nil {
		t.Fatalf("handle(target in another game) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a target in another game", result.DisconnectIDs)
	}

	if !store.HasPlayer(gameId, targetID) {
		t.Errorf("player %v was removed from game %v while their session named another game",
			targetID, gameId)
	}
}

// TestHandle_RejectsAnUnauthenticatedCaller covers the first gate: no session,
// no kick.
func TestHandle_RejectsAnUnauthenticatedCaller(t *testing.T) {
	store, _, gameId := newGame(t)
	sessions := sessionsIn(gameId)

	result, err := handle(kickRequest(nil), store, sessions)
	if err == nil {
		t.Fatalf("handle(no session) = %v, want an error", result)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for an unauthenticated caller", result.DisconnectIDs)
	}

	if !store.HasPlayer(gameId, targetID) {
		t.Errorf("player %v was removed from game %v by an unauthenticated caller", targetID, gameId)
	}
}

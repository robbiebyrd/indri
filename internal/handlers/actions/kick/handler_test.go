package kick

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
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

// directory is the real in-memory session store under the two calls a kick
// makes, plus the sessions the test made, by user.
type directory struct {
	store *sessionRepo.MemoryStore
	of    map[string]*models.Session
}

func (d directory) GetByUserID(userId string) (*models.Session, error) {
	return d.store.FindFirst("userId", userId)
}

func (d directory) LeaveGame(sessionId string, gameId string) error {
	return d.store.LeaveGame(sessionId, gameId)
}

// stored re-reads user's session from the store.
func (d directory) stored(t *testing.T, user string) *models.Session {
	t.Helper()

	s, err := d.store.Get(d.of[user].ID)
	if err != nil {
		t.Fatalf("reading %s's session: %v", user, err)
	}

	return s
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

// sessionsIn stores a session for the host and one for the target, both in the
// game and naming the slots they hold.
func sessionsIn(t *testing.T, k kickGame) directory {
	t.Helper()

	d := directory{store: sessionRepo.NewMemoryStore(), of: map[string]*models.Session{}}

	for user, slot := range map[string]string{hostID: k.hostSlot, targetID: k.targetSlot} {
		s, err := d.store.New(models.CreateSession{UserID: user, Token: ids.New(), GameID: k.id, TeamID: "red", SlotID: slot})
		if err != nil {
			t.Fatalf("storing %s's session: %v", user, err)
		}

		d.of[user] = s
	}

	return d
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
	sessions := sessionsIn(t, k)

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions)
	if err != nil {
		t.Fatalf("handle(host kicks target) = %v, want no error", err)
	}

	want := []string{sessions.of[targetID].ID}
	if len(result.DisconnectIDs) != 1 || result.DisconnectIDs[0] != want[0] {
		t.Errorf("DisconnectIDs = %v, want %v", result.DisconnectIDs, want)
	}

	if holder := k.holder(t, k.targetSlot); holder != "" {
		t.Errorf("slot %v is still held by %q after a successful kick", k.targetSlot, holder)
	}
}

// TestHandle_TakesTheTargetsSessionOutOfTheGame: a kicked player whose session
// still named the game would keep receiving its broadcasts, and could resume
// into it with their token.
func TestHandle_TakesTheTargetsSessionOutOfTheGame(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(t, k)

	if _, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions); err != nil {
		t.Fatalf("handle(host kicks target) = %v, want no error", err)
	}

	if got := sessions.stored(t, targetID); got.GameID != nil || got.SlotID != nil {
		t.Fatalf("the kicked target's session still names game %v, slot %v", got.GameID, got.SlotID)
	}

	if got := sessions.stored(t, hostID); got.GameID == nil || *got.GameID != k.id {
		t.Fatal("the kick took the host's own session out of the game")
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
	sessions := sessionsIn(t, k)

	k.cancel()

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions)

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
	sessions := sessionsIn(t, k)

	result, err := handle(kickRequest(sessions.of[targetID], k.hostSlot), k.store, sessions)
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
	sessions := sessionsIn(t, k)
	sessions.of[targetID].SlotID = ref(k.hostSlot)

	result, err := handle(kickRequest(sessions.of[targetID], k.hostSlot), k.store, sessions)
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
	sessions := sessionsIn(t, k)

	if err := k.store.RemovePlayer(k.id, k.targetSlot, targetID); err != nil {
		t.Fatalf("freeing the target's slot: %v", err)
	}

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions)
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
	sessions := sessionsIn(t, k)
	elsewhere := ids.New()

	if err := sessions.store.Update(sessions.of[targetID].ID, &models.UpdateSession{GameID: elsewhere}); err != nil {
		t.Fatalf("moving the target's session to another game: %v", err)
	}

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions)
	if err != nil {
		t.Fatalf("handle(target playing elsewhere) = %v, want no error", err)
	}

	if holder := k.holder(t, k.targetSlot); holder != "" {
		t.Errorf("slot %v is still held by %q", k.targetSlot, holder)
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a target playing in another game", result.DisconnectIDs)
	}

	// Their session is in the other game, and must stay there.
	if got := sessions.stored(t, targetID); got.GameID == nil || *got.GameID != elsewhere {
		t.Errorf("the kick moved the target's session out of the game they are playing: %v", got.GameID)
	}
}

// TestHandle_FreesTheSlotOfATargetWithNoSession: a player who logged out still
// holds their slot until someone frees it.
func TestHandle_FreesTheSlotOfATargetWithNoSession(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(t, k)
	if err := sessions.store.Delete(sessions.of[targetID].ID); err != nil {
		t.Fatalf("deleting the target's session: %v", err)
	}

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), k.store, sessions)
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
	sessions := sessionsIn(t, k)

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

// handsOffFirst is the real store, except that the host hands their role to
// the target just before the removal runs — the moment between a kick's first
// host check and its write.
type handsOffFirst struct {
	gameStore
	t *testing.T
	k kickGame
}

func (h handsOffFirst) RemovePlayerIf(id, slotId, userId string, allowed func(*models.Game) error) error {
	if err := h.SetPlayerAsHost(h.k.id, h.k.targetSlot); err != nil {
		h.t.Fatalf("handing the host role to the target: %v", err)
	}

	return h.gameStore.RemovePlayerIf(id, slotId, userId, allowed)
}

// TestHandle_RefusesAHostWhoHandedOffMidKick: the host check is made again
// inside the write, so a caller who stopped hosting after the first check
// cannot still kick.
func TestHandle_RefusesAHostWhoHandedOffMidKick(t *testing.T) {
	k := newGame(t)
	sessions := sessionsIn(t, k)

	result, err := handle(kickRequest(sessions.of[hostID], k.targetSlot), handsOffFirst{k.store, t, k}, sessions)
	if err == nil {
		t.Fatal("a kick by a caller who handed off the host role mid-kick succeeded")
	}

	if holder := k.holder(t, k.targetSlot); holder != targetID {
		t.Errorf("slot %v was freed by a caller no longer the host; it is held by %q", k.targetSlot, holder)
	}

	if got := sessions.stored(t, targetID); got.GameID == nil || *got.GameID != k.id {
		t.Error("the refused kick still took the target's session out of the game")
	}

	if len(result.DisconnectIDs) != 0 {
		t.Errorf("DisconnectIDs = %v, want none for a refused kick", result.DisconnectIDs)
	}
}

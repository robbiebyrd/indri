package lua

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// storeKeyframes renders keyframes from the real store, and records every game
// it was asked to broadcast — with what the store held at that moment, which is
// how a test sees whether a broadcast came before or after the write.
type storeKeyframes struct {
	store *gameRepo.MemoryStore

	mu         sync.Mutex
	broadcasts []string
}

func (k *storeKeyframes) Keyframe(gameID string) ([]byte, error) {
	g, err := k.store.Get(gameID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(g)
}

func (k *storeKeyframes) BroadcastKeyframe(gameID string) error {
	frame, err := k.Keyframe(gameID)
	if err != nil {
		return err
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	k.broadcasts = append(k.broadcasts, string(frame))

	return nil
}

func (k *storeKeyframes) sent() []string {
	k.mu.Lock()
	defer k.mu.Unlock()

	return append([]string(nil), k.broadcasts...)
}

// refreshFixture is an engine over a real store with one game, its keyframes
// rendered from that store.
func refreshFixture(t *testing.T, src string) (*Engine, *storeKeyframes, string) {
	t.Helper()

	store := gameRepo.NewMemoryStore(context.Background(), lock.NewInProcess(), nil)

	g, err := store.New("REFRESH", &models.Script{PublicData: map[string]interface{}{}}, false)
	if err != nil {
		t.Fatalf("creating the game: %v", err)
	}

	e := newTestEngineWithGames(t, store, writeScript(t, "game.lua", src))
	keyframes := &storeKeyframes{store: store}
	e.Keyframes = keyframes

	return e, keyframes, g.ID
}

func inGame(gameID string) actions.Request {
	user := "u1"

	return actions.Request{Session: &models.Session{ID: "s1", UserID: &user, GameID: &gameID}, Payload: map[string]interface{}{}}
}

// indri.refresh sends every player in the game a keyframe — after the write the
// handler made has committed, so the keyframe shows it.
func TestRefresh_BroadcastsTheCommittedGameOnce(t *testing.T) {
	e, keyframes, gameID := refreshFixture(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.refresh()
    indri.refresh()
    state.data = state.data or {}
    state.data.round = 7
    return state
  end)
end)`)

	if _, err := splitScriptError(e.Invoke(context.Background(), "move", inGame(gameID))); err != nil {
		t.Fatalf("move: %v", err)
	}

	sent := keyframes.sent()
	if len(sent) != 1 {
		t.Fatalf("indri.refresh broadcast %d keyframes, want exactly 1 however often it is called", len(sent))
	}

	if !strings.Contains(sent[0], `"round":7`) {
		t.Fatalf("the keyframe was taken before the write committed: %s", sent[0])
	}
}

// A refresh asked for inside a write that failed describes a move that never
// happened, and must not be sent.
func TestRefresh_SendsNothingWhenTheWriteFails(t *testing.T) {
	e, keyframes, gameID := refreshFixture(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.refresh()
    indri.refreshSelf()
    error("not your turn", 0)
  end)
end)`)

	result, err := splitScriptError(e.Invoke(context.Background(), "move", inGame(gameID)))
	if err == nil {
		t.Fatal("a move that raised succeeded")
	}

	if sent := keyframes.sent(); len(sent) != 0 {
		t.Fatalf("a failed move still broadcast %d keyframes", len(sent))
	}

	for _, r := range result.Responses {
		if strings.Contains(string(r), `"code":"REFRESH"`) {
			t.Fatalf("a failed move still answered with a keyframe: %s", r)
		}
	}
}

// indri.refreshSelf answers the caller alone with the committed game.
func TestRefreshSelf_AnswersTheCallerWithTheCommittedGame(t *testing.T) {
	e, keyframes, gameID := refreshFixture(t, `
indri.on("move", function(req)
  indri.refreshSelf()
  indri.mutate(function(state)
    state.data = state.data or {}
    state.data.round = 3
    return state
  end)
end)`)

	result, err := splitScriptError(e.Invoke(context.Background(), "move", inGame(gameID)))
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	if len(result.Responses) != 1 || !strings.Contains(string(result.Responses[0]), `"round":3`) {
		t.Fatalf("refreshSelf answered %q, want one keyframe showing the committed write", result.Responses)
	}

	if sent := keyframes.sent(); len(sent) != 0 {
		t.Fatalf("refreshSelf broadcast %d keyframes; it is for the caller alone", len(sent))
	}
}

func TestRefresh_RefusesWithNoGameOrNoKeyframes(t *testing.T) {
	const src = `indri.on("move", function(req) indri.refresh() end)`

	t.Run("no game", func(t *testing.T) {
		e, _, _ := refreshFixture(t, src)

		if _, err := splitScriptError(e.Invoke(context.Background(), "move", actions.Request{})); err == nil ||
			!strings.Contains(err.Error(), "indri.refresh") {
			t.Fatalf("refresh with no game = %v, want an indri.refresh error", err)
		}
	})

	t.Run("an engine built without keyframes", func(t *testing.T) {
		e, _, gameID := refreshFixture(t, src)
		e.Keyframes = nil

		if _, err := splitScriptError(e.Invoke(context.Background(), "move", inGame(gameID))); err == nil ||
			!strings.Contains(err.Error(), "indri.refresh") {
			t.Fatalf("refresh with no keyframes = %v, want an indri.refresh error", err)
		}
	})
}

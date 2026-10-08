package game

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
)

// tictactoeGame is a fresh game stamped from the worked example's config, whose
// layout is most of what a keyframe of it weighs.
func tictactoeGame(t *testing.T) (*Service, string) {
	t.Helper()

	scripts, err := scriptRepo.NewStore("../../../example/tictactoe/config.json")
	if err != nil {
		t.Fatalf("loading the tictactoe config: %v", err)
	}

	gs := &Service{gameRepo: gameRepo.NewMemoryStore(context.Background(), nil, nil), Script: scripts.Get()}

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	return gs, g.ID
}

// keyframe renders the stored game for a client holding held, and decodes it.
func keyframe(t *testing.T, gs *Service, id string, held string) (map[string]interface{}, int) {
	t.Helper()

	frame, err := gs.GetJSONBytes(id, held)
	if err != nil {
		t.Fatalf("rendering a keyframe: %v", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(*frame, &doc); err != nil {
		t.Fatalf("decoding the keyframe: %v", err)
	}

	return doc, len(*frame)
}

func layoutOf(doc map[string]interface{}) (interface{}, bool) {
	data, _ := doc["data"].(map[string]interface{})
	layout, ok := data["layout"]

	return layout, ok
}

func hashOf(t *testing.T, doc map[string]interface{}) string {
	t.Helper()

	hash, _ := doc["layoutHash"].(string)
	if hash == "" {
		t.Fatalf("the keyframe carries no layoutHash: %v", doc["layoutHash"])
	}

	return hash
}

// A client that holds no layout is sent it, with the hash it is to send back.
func TestKeyframe_CarriesTheLayoutAndItsHash(t *testing.T) {
	gs, id := tictactoeGame(t)

	doc, _ := keyframe(t, gs, id, "")

	hashOf(t, doc)

	if _, ok := layoutOf(doc); !ok {
		t.Fatal("a keyframe for a client holding no layout carries none")
	}
}

// A client that names the layout it holds is sent the hash and not the layout:
// the layout is most of a keyframe and does not change as the game is played.
func TestKeyframe_OmitsTheLayoutTheClientHolds(t *testing.T) {
	gs, id := tictactoeGame(t)

	full, fullSize := keyframe(t, gs, id, "")
	hash := hashOf(t, full)

	lean, leanSize := keyframe(t, gs, id, hash)

	if _, ok := layoutOf(lean); ok {
		t.Error("the keyframe re-sent a layout the client said it holds")
	}

	if got := hashOf(t, lean); got != hash {
		t.Errorf("the lean keyframe names layout %q, want %q", got, hash)
	}

	// Everything else is still there: only the layout is left out.
	delete(full["data"].(map[string]interface{}), "layout")

	fullJSON, _ := json.Marshal(full)
	leanJSON, _ := json.Marshal(lean)

	if string(fullJSON) != string(leanJSON) {
		t.Errorf("leaving out the layout changed the rest of the keyframe:\nfull %s\nlean %s", fullJSON, leanJSON)
	}

	t.Logf("tictactoe keyframe: %d bytes with the layout, %d without", fullSize, leanSize)
}

// A hash that names some other layout — an older one, or nonsense — is not the
// layout the game has, so the layout is sent.
func TestKeyframe_SendsTheLayoutForAHashItDoesNotMatch(t *testing.T) {
	gs, id := tictactoeGame(t)

	for _, held := range []string{"stale", strings.Repeat("0", 64)} {
		doc, _ := keyframe(t, gs, id, held)

		if _, ok := layoutOf(doc); !ok {
			t.Errorf("a client holding %q was not sent the layout", held)
		}
	}
}

// The hash names the layout's content: the same layout always hashes the same,
// and any change to it changes the hash, so a client's copy is never mistaken
// for the current one.
func TestKeyframe_HashFollowsTheLayout(t *testing.T) {
	gs, id := tictactoeGame(t)

	first, _ := keyframe(t, gs, id, "")
	again, _ := keyframe(t, gs, id, "")

	if hashOf(t, first) != hashOf(t, again) {
		t.Fatal("the same layout hashed two different ways")
	}

	if err := gs.gameRepo.UpdateField(id, "data.layout.grid.cols", 6); err != nil {
		t.Fatalf("editing the layout: %v", err)
	}

	edited, _ := keyframe(t, gs, id, hashOf(t, first))

	if hashOf(t, edited) == hashOf(t, first) {
		t.Error("an edited layout kept the old hash")
	}

	if _, ok := layoutOf(edited); !ok {
		t.Error("a client holding the layout from before the edit was not sent the new one")
	}
}

// A game with no layout has nothing to hash, and the keyframe says nothing
// about one.
func TestKeyframe_NoLayoutNoHash(t *testing.T) {
	gs, _ := newService(t)

	g, err := gs.New("ABCD", false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	doc, _ := keyframe(t, gs, g.ID, "")

	if _, ok := doc["layoutHash"]; ok {
		t.Errorf("a game with no layout has layoutHash %v", doc["layoutHash"])
	}
}

// A keyframe is still sanitized, whatever happens to the layout.
func TestKeyframe_IsSanitized(t *testing.T) {
	gs, id := tictactoeGame(t)

	if err := gs.gameRepo.UpdateField(id, "privateData.answer", "hidden"); err != nil {
		t.Fatalf("writing private data: %v", err)
	}

	for _, held := range []string{"", "stale"} {
		frame, err := gs.GetJSONBytes(id, held)
		if err != nil {
			t.Fatalf("rendering a keyframe: %v", err)
		}

		if strings.Contains(string(*frame), "hidden") {
			t.Errorf("a keyframe for a client holding %q leaked private data", held)
		}
	}
}

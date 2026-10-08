package game

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
)

// layoutKey is where a game keeps its layout: game.data.layout.
const layoutKey = "layout"

// Keyframe renders g, sanitized, as a client is sent it.
//
// A game with a layout carries layoutHash, a hash of the layout's content,
// beside it. A client sends that hash back when it next asks for a keyframe,
// and if it still names the game's layout the keyframe leaves the layout out:
// it is most of a keyframe's weight and does not change as the game is played.
// A layout edit reaches clients as a delta, which tells a client its copy is
// stale; its next request then names no hash, or an old one, and is sent the
// layout again.
//
// g is sanitized in place, as Sanitize does.
func (gs *Service) Keyframe(g *models.Game, heldLayoutHash string) ([]byte, error) {
	sanitized, err := json.Marshal(gs.Sanitize(g))
	if err != nil {
		return nil, fmt.Errorf("rendering a keyframe of game %q: %w", g.ID, err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(sanitized, &doc); err != nil {
		return nil, fmt.Errorf("rendering a keyframe of game %q: %w", g.ID, err)
	}

	data, _ := doc["data"].(map[string]interface{})

	layout, ok := data[layoutKey]
	if !ok {
		return sanitized, nil
	}

	hash, err := layoutHash(layout)
	if err != nil {
		return nil, fmt.Errorf("hashing the layout of game %q: %w", g.ID, err)
	}

	doc["layoutHash"] = hash

	if heldLayoutHash == hash {
		delete(data, layoutKey)
	}

	frame, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("rendering a keyframe of game %q: %w", g.ID, err)
	}

	return frame, nil
}

// layoutHash names a layout by its content: the SHA-256 of its JSON. Map keys
// marshal in sorted order, so the same layout always hashes the same.
func layoutHash(layout interface{}) (string, error) {
	encoded, err := json.Marshal(layout)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

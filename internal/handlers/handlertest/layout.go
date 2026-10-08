package handlertest

import (
	"encoding/json"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

// WithLayout is TwoTeams with a layout in game.data, as a real game has, so a
// keyframe has a layout to carry or leave out.
func WithLayout() *models.Script {
	script := TwoTeams()
	script.PublicData = map[string]interface{}{
		"layout": map[string]interface{}{
			"grid":   map[string]interface{}{"cols": 12, "rows": 12},
			"scenes": map[string]interface{}{"board": map[string]interface{}{"widgets": map[string]interface{}{}}},
		},
	}

	return script
}

// LayoutHash is the layoutHash a keyframe names, failing the test if it names
// none.
func LayoutHash(t *testing.T, frame []byte) string {
	t.Helper()

	var doc struct {
		LayoutHash string `json:"layoutHash"`
	}

	if err := json.Unmarshal(frame, &doc); err != nil || doc.LayoutHash == "" {
		t.Fatalf("the keyframe %s names no layoutHash (%v)", frame, err)
	}

	return doc.LayoutHash
}

// HasLayout reports whether a keyframe carries the layout itself.
func HasLayout(t *testing.T, frame []byte) bool {
	t.Helper()

	var doc struct {
		Data map[string]json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(frame, &doc); err != nil {
		t.Fatalf("decoding the keyframe %s: %v", frame, err)
	}

	_, ok := doc.Data["layout"]

	return ok
}

package game

import (
	"encoding/json"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

type debugConn struct {
	transport.Conn
	frames [][]byte
}

func (c *debugConn) Get(key string) (any, bool) { return key == "debug", key == "debug" }
func (c *debugConn) Write(msg []byte) error     { c.frames = append(c.frames, msg); return nil }

func keyframeOf(t *testing.T, frame []byte) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(frame, &m); err != nil {
		t.Fatal(err)
	}

	return m
}

// The schema version is the layout hash: stable across keyframes of the same
// game, so it identifies the schema rather than the game's current state.
func TestKeyframes_SchemaVersionIsTheLayoutHash(t *testing.T) {
	gs := &Service{}
	g := &models.Game{
		ID: "g1", Code: "C1",
		PublicData:  map[string]interface{}{"layout": map[string]interface{}{"grid": 3}, "score": 1.0},
		PrivateData: map[string]interface{}{"secret": "x"},
	}

	c := &debugConn{}
	if err := gs.WriteKeyframe(c, g, "layout-v1", map[string]interface{}{"grid": 3}); err != nil {
		t.Fatal(err)
	}

	g.PublicData["score"] = 2.0
	if err := gs.WriteSlimKeyframe(c, g, "layout-v1"); err != nil {
		t.Fatal(err)
	}

	if len(c.frames) != 3 {
		t.Fatalf("frames = %d, want layout + 2 keyframes", len(c.frames))
	}

	first, second := keyframeOf(t, c.frames[1]), keyframeOf(t, c.frames[2])
	if first["sv"] != "layout-v1" || second["sv"] != "layout-v1" {
		t.Fatalf("sv = %v then %v, want the layout hash both times", first["sv"], second["sv"])
	}

	game := first["game"].(map[string]any)
	if _, leaked := game["privateData"]; leaked {
		t.Error("keyframe leaked privateData")
	}
	if _, hasLayout := game["data"].(map[string]any)["layout"]; hasLayout {
		t.Error("keyframe still carries data.layout (sent separately in the layout frame)")
	}
}

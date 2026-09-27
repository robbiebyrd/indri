package game

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
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

// Each game carries its own layout: the layout frame sends the game's layout,
// and the keyframe's schema version is that layout's version, so it changes
// when the layout is edited and not otherwise.
func TestKeyframes_SendTheGamesOwnLayout(t *testing.T) {
	gs := &Service{}
	layout := map[string]interface{}{"grid": 3.0}
	g := &models.Game{
		ID: "g1", Code: "C1",
		PublicData:  map[string]interface{}{"layout": layout, "score": 1.0},
		PrivateData: map[string]interface{}{"secret": "x"},
	}

	c := &debugConn{}
	if err := gs.WriteKeyframe(c, g); err != nil {
		t.Fatal(err)
	}

	g.PublicData["score"] = 2.0
	if err := gs.WriteSlimKeyframe(c, g); err != nil {
		t.Fatal(err)
	}

	g.PublicData["layout"] = map[string]interface{}{"grid": 4.0}
	if err := gs.WriteSlimKeyframe(c, g); err != nil {
		t.Fatal(err)
	}

	if len(c.frames) != 4 {
		t.Fatalf("frames = %d, want layout + 3 keyframes", len(c.frames))
	}

	frame := keyframeOf(t, c.frames[0])
	version := events.LayoutVersion(layout)
	if frame["v"] != version || !reflect.DeepEqual(frame["data"], map[string]any{"grid": 3.0}) {
		t.Fatalf("layout frame = %v, want the game's layout at version %s", frame, version)
	}

	first, second, edited := keyframeOf(t, c.frames[1]), keyframeOf(t, c.frames[2]), keyframeOf(t, c.frames[3])
	if first["sv"] != version || second["sv"] != version {
		t.Fatalf("sv = %v then %v, want the layout version %s both times", first["sv"], second["sv"], version)
	}
	if edited["sv"] == version {
		t.Fatal("editing the layout didn't change the keyframe's schema version")
	}

	game := first["game"].(map[string]any)
	if _, leaked := game["privateData"]; leaked {
		t.Error("keyframe leaked privateData")
	}
	if _, hasLayout := game["data"].(map[string]any)["layout"]; hasLayout {
		t.Error("keyframe still carries data.layout (sent separately in the layout frame)")
	}
}

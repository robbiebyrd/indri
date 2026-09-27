package layout

// handler_test.go exercises applyLayoutOp (the pure mutation function) and the
// handler's auth guard via a fake transport.Conn. The handler-level auth tests
// that require a real session or game (session-in-different-game, non-host,
// userId spoof) cannot be fully exercised here because SessionService and
// GameService are concrete structs backed by MongoDB; they would require an
// integration test with a live database.
//
// What we can test without MongoDB:
//   - applyLayoutOp: every op, the ErrAbort case for removeWidget, and the
//     guarantee that only PublicData["layout"] changes.
//   - Handler.Handle early auth failure: missing "sessionId" connection key.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/mutation"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

// ── fakeConn ─────────────────────────────────────────────────────────────────

// fakeConn implements transport.Conn for tests that need a connection but do
// not exercise the real transport layer.
type fakeConn struct {
	keys    map[string]interface{}
	written [][]byte
	closed  bool
}

func newFakeConn() *fakeConn { return &fakeConn{keys: make(map[string]interface{})} }

func (f *fakeConn) Get(key string) (interface{}, bool) { v, ok := f.keys[key]; return v, ok }
func (f *fakeConn) Set(key string, value interface{})  { f.keys[key] = value }
func (f *fakeConn) UnSet(key string)                   { delete(f.keys, key) }
func (f *fakeConn) Write(msg []byte) error             { f.written = append(f.written, msg); return nil }
func (f *fakeConn) WriteBinary(msg []byte) error       { f.written = append(f.written, msg); return nil }
func (f *fakeConn) Close() error                       { f.closed = true; return nil }
func (f *fakeConn) IsClosed() bool                     { return f.closed }

// ── helpers ───────────────────────────────────────────────────────────────────

// baseGame returns a minimal *models.Game with Players, Teams, Stage, and
// PrivateData populated so tests can assert those fields are untouched.
func baseGame() *models.Game {
	hostID := "user-host"
	return &models.Game{
		Players: map[string]models.Player{
			hostID: {Host: true},
		},
		Teams: map[string]models.Team{
			"team-1": {},
		},
		Stage: models.Stage{
			CurrentScene: "scene1",
		},
		PublicData: map[string]interface{}{
			"someOtherKey": "shouldNotChange",
		},
		PrivateData: map[string]interface{}{
			"secret": "must stay",
		},
	}
}

// snapshotJSON serialises v to JSON so we can compare before/after a mutation.
func snapshotJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("snapshotJSON: %v", err)
	}
	return string(b)
}

// validAddWidgetOp returns a minimal Op that is accepted by ValidateLayout when
// applied to baseGame (whose layout is empty — so grid must be supplied first).
// We supply a grid inline so the resulting layout is self-consistent.
func validLayoutWithWidget() map[string]interface{} {
	return map[string]interface{}{
		"grid": map[string]interface{}{
			"cols": float64(10),
			"rows": float64(10),
		},
		"scenes": map[string]interface{}{
			"scene1": map[string]interface{}{
				"widgets": map[string]interface{}{
					"w1": map[string]interface{}{
						"type": "text",
						"placement": map[string]interface{}{
							"kind": "grid",
							"col":  float64(0),
							"row":  float64(0),
							"w":    float64(2),
							"h":    float64(2),
						},
					},
				},
			},
		},
	}
}

// gameWithLayout returns a baseGame pre-seeded with a valid layout so that
// addWidget ops pass ValidateLayout without also needing a setGrid op.
func gameWithLayout() *models.Game {
	g := baseGame()
	g.PublicData["layout"] = validLayoutWithWidget()
	return g
}

// ── applyLayoutOp: addWidget ──────────────────────────────────────────────────

func TestApplyLayoutOp_AddWidget_SetsWidget(t *testing.T) {
	g := gameWithLayout()
	op := &Op{
		Op:       "addWidget",
		SceneID:  "scene1",
		WidgetID: "w2",
		Widget: map[string]interface{}{
			"type": "text",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(3),
				"row":  float64(0),
				"w":    float64(2),
				"h":    float64(2),
			},
		},
	}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	if _, ok := widgets["w2"]; !ok {
		t.Error("expected widget w2 to be set in layout")
	}
}

// TestApplyLayoutOp_AddWidget_OnlyMutatesLayout verifies that a successful
// addWidget op touches only PublicData["layout"] and leaves Players, Teams,
// Stage, and PrivateData byte-identical.
func TestApplyLayoutOp_AddWidget_OnlyMutatesLayout(t *testing.T) {
	g := gameWithLayout()

	beforePlayers := snapshotJSON(t, g.Players)
	beforeTeams := snapshotJSON(t, g.Teams)
	beforeStage := snapshotJSON(t, g.Stage)
	beforePrivate := snapshotJSON(t, g.PrivateData)
	beforeOtherKey := snapshotJSON(t, g.PublicData["someOtherKey"])

	op := &Op{
		Op:       "addWidget",
		SceneID:  "scene1",
		WidgetID: "w2",
		Widget: map[string]interface{}{
			"type": "text",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(3),
				"row":  float64(0),
				"w":    float64(2),
				"h":    float64(2),
			},
		},
	}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := snapshotJSON(t, g.Players); got != beforePlayers {
		t.Errorf("Players changed: got %v, want %v", got, beforePlayers)
	}
	if got := snapshotJSON(t, g.Teams); got != beforeTeams {
		t.Errorf("Teams changed: got %v, want %v", got, beforeTeams)
	}
	if got := snapshotJSON(t, g.Stage); got != beforeStage {
		t.Errorf("Stage changed: got %v, want %v", got, beforeStage)
	}
	if got := snapshotJSON(t, g.PrivateData); got != beforePrivate {
		t.Errorf("PrivateData changed: got %v, want %v", got, beforePrivate)
	}
	if got := snapshotJSON(t, g.PublicData["someOtherKey"]); got != beforeOtherKey {
		t.Errorf("PublicData[someOtherKey] changed: got %v, want %v", got, beforeOtherKey)
	}
}

// ── applyLayoutOp: removeWidget ───────────────────────────────────────────────

func TestApplyLayoutOp_RemoveWidget_RemovesExistingWidget(t *testing.T) {
	g := gameWithLayout()
	op := &Op{Op: "removeWidget", SceneID: "scene1", WidgetID: "w1"}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	if _, ok := widgets["w1"]; ok {
		t.Error("expected widget w1 to be removed")
	}
}

// TestApplyLayoutOp_RemoveWidget_MissingWidget_ReturnsErrAbort verifies that
// removing a widget that does not exist returns mutation.ErrAbort (which causes
// Mutate to skip the write, producing no delta and no version bump).
func TestApplyLayoutOp_RemoveWidget_MissingWidget_ReturnsErrAbort(t *testing.T) {
	g := gameWithLayout()
	op := &Op{Op: "removeWidget", SceneID: "scene1", WidgetID: "does-not-exist"}

	err := applyLayoutOp(g, op)
	if !errors.Is(err, mutation.ErrAbort) {
		t.Errorf("expected mutation.ErrAbort, got %v", err)
	}
}

// TestApplyLayoutOp_RemoveWidget_MissingScene_ReturnsErrAbort verifies the
// ErrAbort path when the named scene itself is absent.
func TestApplyLayoutOp_RemoveWidget_MissingScene_ReturnsErrAbort(t *testing.T) {
	g := gameWithLayout()
	op := &Op{Op: "removeWidget", SceneID: "no-such-scene", WidgetID: "w1"}

	err := applyLayoutOp(g, op)
	if !errors.Is(err, mutation.ErrAbort) {
		t.Errorf("expected mutation.ErrAbort for missing scene, got %v", err)
	}
}

// ── applyLayoutOp: setPlacement ───────────────────────────────────────────────

func TestApplyLayoutOp_SetPlacement_UpdatesPlacement(t *testing.T) {
	g := gameWithLayout()
	newPlacement := map[string]interface{}{
		"kind": "grid",
		"col":  float64(5),
		"row":  float64(5),
		"w":    float64(2),
		"h":    float64(2),
	}
	op := &Op{Op: "setPlacement", SceneID: "scene1", WidgetID: "w1", Placement: newPlacement}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	widget := widgets["w1"].(map[string]interface{})
	placement := widget["placement"].(map[string]interface{})
	if placement["col"].(float64) != 5 {
		t.Errorf("expected col=5, got %v", placement["col"])
	}
}

// ── applyLayoutOp: setWidgetConfig ───────────────────────────────────────────

func TestApplyLayoutOp_SetWidgetConfig_MergesConfig(t *testing.T) {
	g := gameWithLayout()
	// Pre-seed widget with existing config.
	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	widgets["w1"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind": "grid",
			"col":  float64(0),
			"row":  float64(0),
			"w":    float64(2),
			"h":    float64(2),
		},
		"config": map[string]interface{}{
			"existing": "value",
		},
	}

	op := &Op{
		Op:       "setWidgetConfig",
		SceneID:  "scene1",
		WidgetID: "w1",
		Config:   map[string]interface{}{"color": "red"},
	}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout2 := g.PublicData["layout"].(map[string]interface{})
	scenes2 := layout2["scenes"].(map[string]interface{})
	scene2 := scenes2["scene1"].(map[string]interface{})
	widgets2 := scene2["widgets"].(map[string]interface{})
	widget := widgets2["w1"].(map[string]interface{})
	config := widget["config"].(map[string]interface{})

	if config["color"] != "red" {
		t.Errorf("expected config.color=red, got %v", config["color"])
	}
	// Pre-existing key must be preserved (merge, not replace).
	if config["existing"] != "value" {
		t.Errorf("expected config.existing=value to survive merge, got %v", config["existing"])
	}
}

// ── applyLayoutOp: setStyle ───────────────────────────────────────────────────

func TestApplyLayoutOp_SetStyle_Board(t *testing.T) {
	g := gameWithLayout()
	style := map[string]interface{}{"background": "#fff"}
	op := &Op{Op: "setStyle", Scope: "board", Style: style}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	if layout["style"] == nil {
		t.Error("expected layout.style to be set")
	}
}

func TestApplyLayoutOp_SetStyle_Scene(t *testing.T) {
	g := gameWithLayout()
	style := map[string]interface{}{"color": "blue"}
	op := &Op{Op: "setStyle", Scope: "scene", SceneID: "scene1", Style: style}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	if scene["style"] == nil {
		t.Error("expected scene.style to be set")
	}
}

func TestApplyLayoutOp_SetStyle_Widget(t *testing.T) {
	g := gameWithLayout()
	style := map[string]interface{}{"opacity": float64(1)}
	op := &Op{Op: "setStyle", Scope: "widget", SceneID: "scene1", WidgetID: "w1", Style: style}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	widget := widgets["w1"].(map[string]interface{})
	if widget["style"] == nil {
		t.Error("expected widget.style to be set")
	}
}

func TestApplyLayoutOp_SetStyle_UnknownScope_ReturnsError(t *testing.T) {
	g := gameWithLayout()
	op := &Op{Op: "setStyle", Scope: "universe", Style: map[string]interface{}{}}

	err := applyLayoutOp(g, op)
	if err == nil {
		t.Error("expected error for unknown scope, got nil")
	}
}

// ── applyLayoutOp: setGrid ────────────────────────────────────────────────────

func TestApplyLayoutOp_SetGrid_ReplacesGrid(t *testing.T) {
	g := gameWithLayout()
	newGrid := map[string]interface{}{
		"cols": float64(20),
		"rows": float64(20),
	}
	op := &Op{Op: "setGrid", Grid: newGrid}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	grid := layout["grid"].(map[string]interface{})
	if grid["cols"].(float64) != 20 {
		t.Errorf("expected cols=20, got %v", grid["cols"])
	}
}

// ── applyLayoutOp: setScript ──────────────────────────────────────────────────

func TestApplyLayoutOp_SetScript_Board(t *testing.T) {
	g := gameWithLayout()
	src := "return true"
	op := &Op{Op: "setScript", Scope: "board", Source: &src}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	if layout["script"] != src {
		t.Errorf("expected layout.script=%q, got %v", src, layout["script"])
	}
}

func TestApplyLayoutOp_SetScript_Scene(t *testing.T) {
	g := gameWithLayout()
	src := "return false"
	op := &Op{Op: "setScript", Scope: "scene", SceneID: "scene1", Source: &src}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	if scene["script"] != src {
		t.Errorf("expected scene.script=%q, got %v", src, scene["script"])
	}
}

func TestApplyLayoutOp_SetScript_Widget(t *testing.T) {
	g := gameWithLayout()
	src := "return nil"
	op := &Op{Op: "setScript", Scope: "widget", SceneID: "scene1", WidgetID: "w1", Source: &src}

	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	layout := g.PublicData["layout"].(map[string]interface{})
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes["scene1"].(map[string]interface{})
	widgets := scene["widgets"].(map[string]interface{})
	widget := widgets["w1"].(map[string]interface{})
	if widget["script"] != src {
		t.Errorf("expected widget.script=%q, got %v", src, widget["script"])
	}
}

// ── Handler auth: no sessionId ────────────────────────────────────────────────

// TestHandle_NoSessionId_RejectsWithError verifies that a connection with no
// "sessionId" key is rejected before any database call is made.
// GetKeyAsString reads from the Conn directly, so a fakeConn is sufficient —
// no MongoDB required.
func TestHandle_NoSessionId_RejectsWithError(t *testing.T) {
	conn := newFakeConn() // no "sessionId" key set
	h := New(&injector.Injector{
		ClientsInjector: &injector.ClientsInjector{Transport: ws.New()},
	})
	msg := map[string]interface{}{
		"code":     "GAME01",
		"op":       "addWidget",
		"sceneId":  "s1",
		"widgetId": "w1",
		"widget":   map[string]interface{}{},
	}
	err := h.Handle(conn, msg)
	if err == nil {
		t.Fatal("expected error for connection with no sessionId key, got nil")
	}
}

// ── ValidateLayout prevents invalid layouts being persisted ───────────────────

// TestApplyLayoutOp_InvalidLayout_ReturnsError verifies that ValidateLayout is
// called after every op and a layout that fails validation is not committed
// (applyLayoutOp returns an error, causing Mutate to roll back).
func TestApplyLayoutOp_InvalidLayout_ReturnsError(t *testing.T) {
	g := baseGame() // no layout yet
	// setGrid with grid that is too small should fail ValidateLayout
	op := &Op{
		Op: "setGrid",
		Grid: map[string]interface{}{
			"cols": float64(1), // below minDim=8
			"rows": float64(1),
		},
	}

	err := applyLayoutOp(g, op)
	if err == nil {
		t.Error("expected error for invalid grid dims, got nil")
	}
}

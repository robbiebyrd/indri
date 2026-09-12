package layout

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// validLayout is a board with one widget, small enough to reason about and
// valid by validateLayout's rules, so every failure in these tests is caused by
// the op under test rather than by the fixture.
func validLayout() map[string]interface{} {
	return map[string]interface{}{
		fieldGrid: map[string]interface{}{fieldCols: 12, fieldRows: 8},
		fieldScenes: map[string]interface{}{
			"board": map[string]interface{}{
				fieldWidgets: map[string]interface{}{
					"title": map[string]interface{}{
						fieldType: "text",
						fieldPlacement: map[string]interface{}{
							fieldKind: kindGrid, "col": 0, "row": 0, "w": 4, "h": 2,
						},
					},
				},
			},
		},
	}
}

func gameWithLayout(layout interface{}) *models.Game {
	g := &models.Game{}
	if layout != nil {
		g.PublicData = map[string]interface{}{layoutKey: layout}
	}

	return g
}

// layoutOf reads back what applyLayoutOp wrote, failing the test if the game
// has no layout at all.
func layoutOf(t *testing.T, g *models.Game) map[string]interface{} {
	t.Helper()

	layout, ok := g.PublicData[layoutKey].(map[string]interface{})
	if !ok {
		t.Fatalf("game has no layout object; data = %#v", g.PublicData)
	}

	return layout
}

// at walks a dotted path into a layout, so a test can name the one value it
// cares about instead of unpacking four levels of map assertions.
func at(t *testing.T, layout map[string]interface{}, path ...string) interface{} {
	t.Helper()

	var value interface{} = layout

	for i, key := range path {
		node, ok := value.(map[string]interface{})
		if !ok {
			t.Fatalf("%v is not an object", path[:i])
		}

		value, ok = node[key]
		if !ok {
			t.Fatalf("%v is not present", path[:i+1])
		}
	}

	return value
}

func TestApplyLayoutOp_SetGridSeedsScenesOnAGameWithNoLayout(t *testing.T) {
	g := gameWithLayout(nil)

	op := &Op{Op: OpSetGrid, Grid: map[string]interface{}{fieldCols: 12, fieldRows: 8}}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(setGrid) = %v, want it to seed a layout", err)
	}

	layout := layoutOf(t, g)

	// The validator requires "scenes", so a setGrid that only wrote the grid
	// would store a document it would itself reject on the next op.
	scenes, ok := layout[fieldScenes].(map[string]interface{})
	if !ok || len(scenes) != 0 {
		t.Errorf("layout scenes = %#v, want an empty object", layout[fieldScenes])
	}

	if err := validateLayout(layout); err != nil {
		t.Errorf("validateLayout(seeded layout) = %v, want the seeded layout to be valid", err)
	}
}

// A scene has no op of its own, so addWidget is the only way one can ever come
// into existence on a board the host started from scratch.
func TestApplyLayoutOp_AddWidgetCreatesAnAbsentScene(t *testing.T) {
	g := gameWithLayout(map[string]interface{}{
		fieldGrid:   map[string]interface{}{fieldCols: 12, fieldRows: 8},
		fieldScenes: map[string]interface{}{},
	})

	op := &Op{
		Op: OpAddWidget, SceneID: "board", WidgetID: "score",
		Widget: map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 0, "row": 0, "w": 2, "h": 1},
		},
	}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(addWidget) = %v, want the scene to be created", err)
	}

	if got := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets, "score", fieldType); got != "text" {
		t.Errorf("widget type = %v, want %q", got, "text")
	}
}

func TestApplyLayoutOp_AddWidgetRejectsAnIdThatIsAlreadyThere(t *testing.T) {
	g := gameWithLayout(validLayout())

	op := &Op{
		Op: OpAddWidget, SceneID: "board", WidgetID: "title",
		Widget: map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 6, "row": 0, "w": 2, "h": 1},
		},
	}

	err := applyLayoutOp(g, op)
	if !errors.Is(err, errExists) {
		t.Fatalf("applyLayoutOp(addWidget, existing id) = %v, want %v", err, errExists)
	}

	// An "add" that silently replaced would lose the original widget.
	if got := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets, "title", fieldPlacement); !reflect.DeepEqual(
		got, validLayout()[fieldScenes].(map[string]interface{})["board"].(map[string]interface{})[fieldWidgets].(map[string]interface{})["title"].(map[string]interface{})[fieldPlacement],
	) {
		t.Errorf("placement = %#v, want the original widget left alone", got)
	}
}

func TestApplyLayoutOp_RejectsAnOverlappingWidgetAndWritesNothing(t *testing.T) {
	g := gameWithLayout(validLayout())
	before := mustJSON(t, g.PublicData)

	// "title" occupies (0,0) 4x2, so this rect lands on top of it.
	op := &Op{
		Op: OpAddWidget, SceneID: "board", WidgetID: "overlapping",
		Widget: map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 1, "row": 1, "w": 2, "h": 2},
		},
	}

	if err := applyLayoutOp(g, op); err == nil {
		t.Fatalf("applyLayoutOp(overlapping addWidget) = nil, want the op rejected")
	}

	// The op is applied to a copy, so a rejected op must leave the game exactly
	// as it was — otherwise a failed validation would still commit on a retry.
	if after := mustJSON(t, g.PublicData); after != before {
		t.Errorf("public data changed on a rejected op:\n before %v\n after  %v", before, after)
	}
}

func TestApplyLayoutOp_RejectsAReservedKey(t *testing.T) {
	g := gameWithLayout(validLayout())

	op := &Op{
		Op: OpSetWidgetConfig, SceneID: "board", WidgetID: "title",
		Config: map[string]interface{}{reservedKey: map[string]interface{}{"answer": 42}},
	}

	// SanitizeDelta strips any path holding this segment, so a layout carrying
	// it stores data no client could ever read back.
	if err := applyLayoutOp(g, op); !errors.Is(err, errReserved) {
		t.Fatalf("applyLayoutOp(config with %q) = %v, want %v", reservedKey, err, errReserved)
	}
}

func TestApplyLayoutOp_RemoveWidgetAbortsWhenThereIsNothingToRemove(t *testing.T) {
	cases := map[string]*Op{
		"missing widget": {Op: OpRemoveWidget, SceneID: "board", WidgetID: "absent"},
		"missing scene":  {Op: OpRemoveWidget, SceneID: "absent", WidgetID: "title"},
	}

	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			g := gameWithLayout(validLayout())
			before := mustJSON(t, g.PublicData)

			// ErrAbort rather than an error: the delete is idempotent for a
			// client retrying after a reconnect, and aborting means no version
			// bump and no delta.
			if err := applyLayoutOp(g, op); !errors.Is(err, mutation.ErrAbort) {
				t.Fatalf("applyLayoutOp(%v) = %v, want %v", name, err, mutation.ErrAbort)
			}

			if after := mustJSON(t, g.PublicData); after != before {
				t.Errorf("public data changed on an aborted op:\n before %v\n after  %v", before, after)
			}
		})
	}
}

func TestApplyLayoutOp_RemoveWidgetDeletesAWidgetThatIsThere(t *testing.T) {
	g := gameWithLayout(validLayout())

	if err := applyLayoutOp(g, &Op{Op: OpRemoveWidget, SceneID: "board", WidgetID: "title"}); err != nil {
		t.Fatalf("applyLayoutOp(removeWidget) = %v, want the widget removed", err)
	}

	widgets, ok := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets).(map[string]interface{})
	if !ok || len(widgets) != 0 {
		t.Errorf("widgets = %#v, want the scene left empty", widgets)
	}
}

func TestApplyLayoutOp_AbortsWhenTheOpChangesNothing(t *testing.T) {
	g := gameWithLayout(validLayout())

	op := &Op{Op: OpSetGrid, Grid: map[string]interface{}{fieldCols: 12, fieldRows: 8}}
	if err := applyLayoutOp(g, op); !errors.Is(err, mutation.ErrAbort) {
		t.Fatalf("applyLayoutOp(setGrid, same grid) = %v, want %v", err, mutation.ErrAbort)
	}
}

// A layout that has been through MongoDB comes back as bson.D, not as a map.
// Without normalisation the validator could not walk it, and every op on a
// stored layout would fail.
func TestApplyLayoutOp_NormalisesALayoutStoredAsBSON(t *testing.T) {
	stored := bson.D{
		{Key: fieldGrid, Value: bson.D{{Key: fieldCols, Value: int32(12)}, {Key: fieldRows, Value: int32(8)}}},
		{Key: fieldScenes, Value: bson.D{
			{Key: "board", Value: bson.D{
				{Key: fieldWidgets, Value: bson.D{
					{Key: "title", Value: bson.D{
						{Key: fieldType, Value: "text"},
						{Key: fieldPlacement, Value: bson.D{
							{Key: fieldKind, Value: kindGrid},
							{Key: "col", Value: int32(0)},
							{Key: "row", Value: int32(0)},
							{Key: "w", Value: int32(4)},
							{Key: "h", Value: int32(2)},
						}},
					}},
				}},
			}},
		}},
	}

	g := gameWithLayout(stored)

	op := &Op{
		Op: OpAddWidget, SceneID: "board", WidgetID: "score",
		Widget: map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 8, "row": 0, "w": 2, "h": 1},
		},
	}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(addWidget on a stored layout) = %v, want it applied", err)
	}

	layout := layoutOf(t, g)

	// The existing widget survives the conversion, and the result is a shape a
	// client can parse rather than the array bson.D marshals to.
	if got := at(t, layout, fieldScenes, "board", fieldWidgets, "title", fieldType); got != "text" {
		t.Errorf("existing widget type = %v, want %q", got, "text")
	}

	if got := at(t, layout, fieldScenes, "board", fieldWidgets, "score", fieldType); got != "text" {
		t.Errorf("added widget type = %v, want %q", got, "text")
	}
}

func TestApplyLayoutOp_RejectsAnOpAddressingSomethingThatIsNotThere(t *testing.T) {
	cases := map[string]*Op{
		"placement on a missing widget": {
			Op: OpSetPlacement, SceneID: "board", WidgetID: "absent",
			Placement: map[string]interface{}{fieldKind: kindGrid, "col": 0, "row": 4, "w": 1, "h": 1},
		},
		"config on a missing widget": {
			Op: OpSetWidgetConfig, SceneID: "board", WidgetID: "absent",
			Config: map[string]interface{}{"text": "hello"},
		},
		"style on a missing scene": {
			Op: OpSetStyle, Scope: ScopeScene, SceneID: "absent",
			Style: map[string]interface{}{"backgroundColor": "#000"},
		},
		"script on a missing widget": {
			Op: OpSetScript, Scope: ScopeWidget, SceneID: "board", WidgetID: "absent",
			Source: ptr("return 1"),
		},
	}

	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			g := gameWithLayout(validLayout())

			// Editing something that is not there is a client bug, not an
			// idempotent no-op: silently doing nothing would hide it.
			if err := applyLayoutOp(g, op); !errors.Is(err, errNotFound) {
				t.Fatalf("applyLayoutOp(%v) = %v, want %v", name, err, errNotFound)
			}
		})
	}
}

func TestApplyLayoutOp_SetPlacementReplacesTheWholePlacement(t *testing.T) {
	g := gameWithLayout(validLayout())

	op := &Op{
		Op: OpSetPlacement, SceneID: "board", WidgetID: "title",
		Placement: map[string]interface{}{fieldKind: kindGrid, "col": 2, "row": 3, "w": 4, "h": 2},
	}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(setPlacement) = %v, want it applied", err)
	}

	got := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets, "title", fieldPlacement)

	want := map[string]interface{}{fieldKind: kindGrid, "col": 2, "row": 3, "w": 4, "h": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("placement = %#v, want %#v", got, want)
	}
}

func TestApplyLayoutOp_SetWidgetConfigMergesRatherThanReplaces(t *testing.T) {
	layout := validLayout()
	widget := layout[fieldScenes].(map[string]interface{})["board"].(map[string]interface{})[fieldWidgets].(map[string]interface{})["title"].(map[string]interface{})
	widget[fieldConfig] = map[string]interface{}{"text": "hello", "align": "left"}

	g := gameWithLayout(layout)

	op := &Op{
		Op: OpSetWidgetConfig, SceneID: "board", WidgetID: "title",
		Config: map[string]interface{}{"text": "goodbye"},
	}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(setWidgetConfig) = %v, want it applied", err)
	}

	got := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets, "title", fieldConfig)

	// A panel sends one field at a time, so a replace would blank every other
	// setting each time the host typed in a different box.
	want := map[string]interface{}{"text": "goodbye", "align": "left"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config = %#v, want %#v", got, want)
	}
}

func TestApplyLayoutOp_SetStyleAtEveryScope(t *testing.T) {
	style := map[string]interface{}{"backgroundColor": "#123456"}

	cases := map[string]struct {
		op   *Op
		path []string
	}{
		"board":  {&Op{Op: OpSetStyle, Scope: ScopeBoard, Style: style}, []string{fieldStyle}},
		"scene":  {&Op{Op: OpSetStyle, Scope: ScopeScene, SceneID: "board", Style: style}, []string{fieldScenes, "board", fieldStyle}},
		"widget": {&Op{Op: OpSetStyle, Scope: ScopeWidget, SceneID: "board", WidgetID: "title", Style: style}, []string{fieldScenes, "board", fieldWidgets, "title", fieldStyle}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := gameWithLayout(validLayout())

			if err := applyLayoutOp(g, tc.op); err != nil {
				t.Fatalf("applyLayoutOp(setStyle %v) = %v, want it applied", name, err)
			}

			if got := at(t, layoutOf(t, g), tc.path...); !reflect.DeepEqual(got, style) {
				t.Errorf("%v style = %#v, want %#v", name, got, style)
			}
		})
	}
}

func TestApplyLayoutOp_SetScriptClearsOnAnEmptySource(t *testing.T) {
	layout := validLayout()
	layout[fieldScript] = "return 1"

	g := gameWithLayout(layout)

	if err := applyLayoutOp(g, &Op{Op: OpSetScript, Scope: ScopeBoard, Source: ptr("")}); err != nil {
		t.Fatalf("applyLayoutOp(setScript, empty) = %v, want the script cleared", err)
	}

	// Deleted rather than set to "": the delta is a removal the client can act
	// on, and the document carries no dead field.
	if _, ok := layoutOf(t, g)[fieldScript]; ok {
		t.Errorf("script is still present, want it removed")
	}
}

func TestApplyLayoutOp_WritesNothingOutsideTheLayout(t *testing.T) {
	g := gameWithLayout(validLayout())
	g.PublicData["scores"] = map[string]interface{}{"red": 3}
	g.PrivateData = map[string]interface{}{"answer": "42"}
	g.Players = map[string]models.Player{"host": {Name: "Host", Host: true}}
	g.Teams = map[string]models.Team{"red": {Name: "Red"}}
	g.Stage = models.Stage{CurrentScene: "intro"}

	before := struct{ players, teams, stage, private, scores string }{
		mustJSON(t, g.Players), mustJSON(t, g.Teams), mustJSON(t, g.Stage),
		mustJSON(t, g.PrivateData), mustJSON(t, g.PublicData["scores"]),
	}

	op := &Op{
		Op: OpAddWidget, SceneID: "board", WidgetID: "score",
		Widget: map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 8, "row": 0, "w": 2, "h": 1},
		},
	}
	if err := applyLayoutOp(g, op); err != nil {
		t.Fatalf("applyLayoutOp(addWidget) = %v, want it applied", err)
	}

	after := struct{ players, teams, stage, private, scores string }{
		mustJSON(t, g.Players), mustJSON(t, g.Teams), mustJSON(t, g.Stage),
		mustJSON(t, g.PrivateData), mustJSON(t, g.PublicData["scores"]),
	}

	if after != before {
		t.Errorf("the op reached outside data.layout:\n before %+v\n after  %+v", before, after)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()

	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling %#v: %v", v, err)
	}

	return string(encoded)
}

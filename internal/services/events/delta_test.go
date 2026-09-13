package events_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/robbiebyrd/indri/internal/services/events"
)

func TestDiff_NestedFieldChange(t *testing.T) {
	before := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": false, "name": "a"},
		},
	}
	after := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": true, "name": "a"},
		},
	}

	updated, removed := events.Diff(before, after)

	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}

	want := map[string]interface{}{"players.p1.host": true}
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("expected %v, got %v", want, updated)
	}
}

func TestDiff_AddedKey(t *testing.T) {
	before := map[string]interface{}{
		"players": map[string]interface{}{"p1": map[string]interface{}{"host": true}},
	}
	newPlayer := map[string]interface{}{"host": false}
	after := map[string]interface{}{
		"players": map[string]interface{}{
			"p1": map[string]interface{}{"host": true},
			"p2": newPlayer,
		},
	}

	updated, removed := events.Diff(before, after)

	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}

	if !reflect.DeepEqual(updated["players.p2"], newPlayer) {
		t.Errorf("expected new player at players.p2, got %v", updated)
	}
}

func TestDiff_RemovedKey(t *testing.T) {
	before := map[string]interface{}{
		"teams": map[string]interface{}{"t1": map[string]interface{}{}, "t2": map[string]interface{}{}},
	}
	after := map[string]interface{}{
		"teams": map[string]interface{}{"t2": map[string]interface{}{}},
	}

	updated, removed := events.Diff(before, after)

	if len(updated) != 0 {
		t.Errorf("expected no updates, got %v", updated)
	}

	if !reflect.DeepEqual(removed, []string{"teams.t1"}) {
		t.Errorf("expected [teams.t1] removed, got %v", removed)
	}
}

func TestDiff_ArrayReplacedWhole(t *testing.T) {
	before := map[string]interface{}{"board": []interface{}{"X", ""}}
	after := map[string]interface{}{"board": []interface{}{"X", "O"}}

	updated, _ := events.Diff(before, after)

	if !reflect.DeepEqual(updated["board"], []interface{}{"X", "O"}) {
		t.Errorf("expected whole board replaced, got %v", updated)
	}
}

func TestDiff_NoChange(t *testing.T) {
	m := map[string]interface{}{"a": 1, "b": map[string]interface{}{"c": 2}}

	updated, removed := events.Diff(m, m)

	if len(updated) != 0 || len(removed) != 0 {
		t.Errorf("expected no delta, got updated=%v removed=%v", updated, removed)
	}
}

func TestSanitizeDelta_DropsPrivatePaths(t *testing.T) {
	updated := map[string]interface{}{
		"players.p1.host":          true,
		"stage.privateData.answer": "42",
		"teams.t1.privateData":     map[string]interface{}{"role": "spy"},
	}
	removed := []string{"players.p2", "stage.scenes.s1.privateData.key"}

	gotUpdated, gotRemoved := events.SanitizeDelta(updated, removed)

	if _, ok := gotUpdated["stage.privateData.answer"]; ok {
		t.Error("private path leaked in updated")
	}
	if _, ok := gotUpdated["teams.t1.privateData"]; ok {
		t.Error("private path leaked in updated")
	}
	if gotUpdated["players.p1.host"] != true {
		t.Error("public path was dropped")
	}
	if !reflect.DeepEqual(gotRemoved, []string{"players.p2"}) {
		t.Errorf("expected only public removal, got %v", gotRemoved)
	}
}

func TestSanitizeDelta_StripsNestedPrivateFromValue(t *testing.T) {
	// A whole-object update (e.g. a newly added player) must not carry its
	// nested privateData out on the wire.
	updated := map[string]interface{}{
		"players.p1": map[string]interface{}{
			"host":        true,
			"privateData": map[string]interface{}{"secret": "role"},
		},
	}

	gotUpdated, _ := events.SanitizeDelta(updated, nil)

	player, ok := gotUpdated["players.p1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected player object, got %T", gotUpdated["players.p1"])
	}
	if _, ok := player["privateData"]; ok {
		t.Error("nested privateData leaked in whole-object update")
	}
	if player["host"] != true {
		t.Error("public field was stripped")
	}
}

// A document key that contains the path separator must not forge a segment.
// Before escaping, a widget id of "foo.privateData" produced a path whose last
// segment was literally "privateData", so SanitizeDelta silently dropped every
// update to that widget and the client applied the ones that survived to the
// wrong node.
func TestDiff_DottedKeyCannotForgeAPathSegment(t *testing.T) {
	tests := []struct {
		name     string
		before   map[string]interface{}
		after    map[string]interface{}
		wantPath string
		wantVal  interface{}
	}{
		{
			name:     "added widget with a forged private id",
			before:   widgets(map[string]interface{}{}),
			after:    widgets(map[string]interface{}{"foo.privateData": "text"}),
			wantPath: `data.widgets.foo\.privateData`,
			wantVal:  "text",
		},
		{
			name:     "changed field under a forged private id",
			before:   widgets(map[string]interface{}{"foo.privateData": map[string]interface{}{"label": "a"}}),
			after:    widgets(map[string]interface{}{"foo.privateData": map[string]interface{}{"label": "b"}}),
			wantPath: `data.widgets.foo\.privateData.label`,
			wantVal:  "b",
		},
		{
			name:     "id ending in a dot before the reserved key",
			before:   widgets(map[string]interface{}{}),
			after:    widgets(map[string]interface{}{"a.b.privateData": 1}),
			wantPath: `data.widgets.a\.b\.privateData`,
			wantVal:  1,
		},
		{
			name:     "id containing a backslash",
			before:   widgets(map[string]interface{}{}),
			after:    widgets(map[string]interface{}{`a\b`: 1}),
			wantPath: `data.widgets.a\\b`,
			wantVal:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, removed := events.Diff(tt.before, tt.after)

			if _, ok := updated[tt.wantPath]; !ok {
				t.Fatalf("expected escaped path %q, got %v", tt.wantPath, updated)
			}

			gotUpdated, _ := events.SanitizeDelta(updated, removed)

			value, ok := gotUpdated[tt.wantPath]
			if !ok {
				t.Fatalf("update to %q was swallowed by SanitizeDelta", tt.wantPath)
			}

			if !reflect.DeepEqual(value, tt.wantVal) {
				t.Errorf("expected %v at %q, got %v", tt.wantVal, tt.wantPath, value)
			}
		})
	}
}

// The removal of such a key must reach the client too: a widget deleted from
// the board has to disappear from it.
func TestDiff_DottedKeyRemovalSurvivesSanitize(t *testing.T) {
	before := widgets(map[string]interface{}{"keep": 1, "foo.privateData": 2})
	after := widgets(map[string]interface{}{"keep": 1})

	updated, removed := events.Diff(before, after)

	_, gotRemoved := events.SanitizeDelta(updated, removed)

	want := []string{`data.widgets.foo\.privateData`}
	if !reflect.DeepEqual(gotRemoved, want) {
		t.Errorf("expected %v removed, got %v", want, gotRemoved)
	}
}

// SanitizeDelta decides on decoded segments, so the wire format itself is the
// contract. Escaping must neither expose a real privateData field nor hide a
// key that merely reads like one.
func TestSanitizeDelta_ComparesDecodedSegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		keep bool
	}{
		{name: "ordinary path", path: "players.p1.host", keep: true},
		{name: "private field", path: "stage.privateData.answer", keep: false},
		{name: "whole private object", path: "teams.t1.privateData", keep: false},
		{name: "private at the root", path: "privateData", keep: false},
		{name: "forged private segment", path: `data.widgets.foo\.privateData`, keep: true},
		{name: "forged private segment with a child", path: `data.widgets.foo\.privateData.label`, keep: true},
		{name: "private key that is only a prefix", path: "data.privateDataish", keep: true},
		{name: "private key that is only a suffix", path: "data.userPrivateData", keep: true},
		// A needlessly escaped key still decodes to privateData, so escaping
		// cannot be used to smuggle a real private field past the filter.
		{name: "escaped spelling of the reserved key", path: `stage.\privateData.answer`, keep: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, removed := events.SanitizeDelta(
				map[string]interface{}{tt.path: "v"},
				[]string{tt.path},
			)

			_, keptUpdate := updated[tt.path]
			if keptUpdate != tt.keep {
				t.Errorf("updated %q: kept = %v, want %v", tt.path, keptUpdate, tt.keep)
			}

			keptRemoval := len(removed) == 1
			if keptRemoval != tt.keep {
				t.Errorf("removed %q: kept = %v, want %v", tt.path, keptRemoval, tt.keep)
			}
		})
	}
}

// widgets wraps a widget map in the document shape a layout lives in, so the
// escaped key is exercised below a prefix rather than at the root.
func widgets(w map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"widgets": w}}
}

func TestDiff_MultiplePaths(t *testing.T) {
	before := map[string]interface{}{"x": 1, "y": 2, "z": 3}
	after := map[string]interface{}{"x": 1, "y": 20, "w": 4}

	updated, removed := events.Diff(before, after)

	sort.Strings(removed)
	if !reflect.DeepEqual(removed, []string{"z"}) {
		t.Errorf("expected [z] removed, got %v", removed)
	}

	want := map[string]interface{}{"y": 20, "w": 4}
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("expected %v, got %v", want, updated)
	}
}

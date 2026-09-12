package layout

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

// TestDecodeOp_Vocabulary pins every op in the vocabulary table against its wire
// form, so a field landing in the wrong place is caught rather than discovered
// later by a handler reading a zero value.
func TestDecodeOp_Vocabulary(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]interface{}
		want *Op
	}{
		{
			name: "addWidget",
			msg: map[string]interface{}{
				"code": "ABCD", "op": "addWidget", "sceneId": "s1", "widgetId": "w1",
				"widget": map[string]interface{}{"type": "text", "config": map[string]interface{}{"text": "hi"}},
			},
			want: &Op{
				Op: OpAddWidget, SceneID: "s1", WidgetID: "w1",
				Widget: map[string]interface{}{"type": "text", "config": map[string]interface{}{"text": "hi"}},
			},
		},
		{
			name: "removeWidget",
			msg:  map[string]interface{}{"code": "ABCD", "op": "removeWidget", "sceneId": "s1", "widgetId": "w1"},
			want: &Op{Op: OpRemoveWidget, SceneID: "s1", WidgetID: "w1"},
		},
		{
			// setPlacement is the only move/resize op; both write this field.
			name: "setPlacement",
			msg: map[string]interface{}{
				"op": "setPlacement", "sceneId": "s1", "widgetId": "w1",
				"placement": map[string]interface{}{"kind": "grid", "col": 2.0, "row": 3.0, "w": 4.0, "h": 5.0},
			},
			want: &Op{
				Op: OpSetPlacement, SceneID: "s1", WidgetID: "w1",
				Placement: map[string]interface{}{"kind": "grid", "col": 2.0, "row": 3.0, "w": 4.0, "h": 5.0},
			},
		},
		{
			name: "setWidgetConfig",
			msg: map[string]interface{}{
				"op": "setWidgetConfig", "sceneId": "s1", "widgetId": "w1",
				"config": map[string]interface{}{"text": "hello"},
			},
			want: &Op{
				Op: OpSetWidgetConfig, SceneID: "s1", WidgetID: "w1",
				Config: map[string]interface{}{"text": "hello"},
			},
		},
		{
			name: "setStyle board scope",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "board", "style": map[string]interface{}{"background": "#fff"},
			},
			want: &Op{Op: OpSetStyle, Scope: ScopeBoard, Style: map[string]interface{}{"background": "#fff"}},
		},
		{
			name: "setStyle scene scope",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "scene", "sceneId": "s1",
				"style": map[string]interface{}{"background": "#000"},
			},
			want: &Op{
				Op: OpSetStyle, Scope: ScopeScene, SceneID: "s1",
				Style: map[string]interface{}{"background": "#000"},
			},
		},
		{
			name: "setStyle widget scope",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "widget", "sceneId": "s1", "widgetId": "w1",
				"style": map[string]interface{}{"color": "red"},
			},
			want: &Op{
				Op: OpSetStyle, Scope: ScopeWidget, SceneID: "s1", WidgetID: "w1",
				Style: map[string]interface{}{"color": "red"},
			},
		},
		{
			name: "setGrid",
			msg: map[string]interface{}{
				"op": "setGrid", "grid": map[string]interface{}{"cols": 24.0, "rows": 16.0},
			},
			want: &Op{Op: OpSetGrid, Grid: map[string]interface{}{"cols": 24.0, "rows": 16.0}},
		},
		{
			name: "setScript board scope",
			msg:  map[string]interface{}{"op": "setScript", "scope": "board", "source": "return 1"},
			want: &Op{Op: OpSetScript, Scope: ScopeBoard, Source: ptr("return 1")},
		},
		{
			name: "setScript scene scope",
			msg:  map[string]interface{}{"op": "setScript", "scope": "scene", "sceneId": "s1", "source": "return 2"},
			want: &Op{Op: OpSetScript, Scope: ScopeScene, SceneID: "s1", Source: ptr("return 2")},
		},
		{
			name: "setScript widget scope",
			msg: map[string]interface{}{
				"op": "setScript", "scope": "widget", "sceneId": "s1", "widgetId": "w1", "source": "return 3",
			},
			want: &Op{Op: OpSetScript, Scope: ScopeWidget, SceneID: "s1", WidgetID: "w1", Source: ptr("return 3")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeOp(tt.msg)
			if err != nil {
				t.Fatalf("decodeOp: unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeOp:\n got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestDecodeOp_EmptySourceIsNotAbsent covers the reason Source is a pointer: an
// empty script clears the script, and must not read back the same as no script
// field at all.
func TestDecodeOp_EmptySourceIsNotAbsent(t *testing.T) {
	got, err := decodeOp(map[string]interface{}{"op": "setScript", "scope": "board", "source": ""})
	if err != nil {
		t.Fatalf("decodeOp: an empty source is a valid value, got error: %v", err)
	}

	if got.Source == nil {
		t.Fatalf("decodeOp: empty source decoded as absent")
	}

	if *got.Source != "" {
		t.Fatalf("decodeOp: source = %q, want empty", *got.Source)
	}

	// The same op without the field is rejected, so "" and absent are never
	// confusable in the first place.
	if _, err := decodeOp(map[string]interface{}{"op": "setScript", "scope": "board"}); err == nil {
		t.Fatalf("decodeOp: an absent source must be rejected, not treated as empty")
	}
}

// TestDecodeOp_Rejects covers every way a payload can be wrong. Each case
// asserts the error class and that the message names the offending field — a
// generic "invalid payload" leaves a client with nothing to fix.
func TestDecodeOp_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		msg     map[string]interface{}
		wantErr error
		names   string // must appear in the message
	}{
		{
			name:    "op missing",
			msg:     map[string]interface{}{"code": "ABCD", "sceneId": "s1"},
			wantErr: errMissingField, names: "op",
		},
		{
			name:    "op not a string",
			msg:     map[string]interface{}{"op": 7.0},
			wantErr: errInvalidType, names: "op",
		},
		{
			name:    "op empty",
			msg:     map[string]interface{}{"op": ""},
			wantErr: errMissingField, names: "op",
		},
		{
			name:    "unknown op",
			msg:     map[string]interface{}{"op": "frobnicate", "sceneId": "s1"},
			wantErr: errUnknownOp, names: "frobnicate",
		},
		{
			// resizeWidget deliberately does not exist: setPlacement covers it.
			name:    "resizeWidget is not an op",
			msg:     map[string]interface{}{"op": "resizeWidget", "sceneId": "s1", "widgetId": "w1"},
			wantErr: errUnknownOp, names: "resizeWidget",
		},

		{
			name:    "addWidget missing sceneId",
			msg:     map[string]interface{}{"op": "addWidget", "widgetId": "w1", "widget": map[string]interface{}{}},
			wantErr: errMissingField, names: "sceneId",
		},
		{
			name:    "addWidget missing widgetId",
			msg:     map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widget": map[string]interface{}{}},
			wantErr: errMissingField, names: "widgetId",
		},
		{
			name:    "addWidget missing widget",
			msg:     map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widgetId": "w1"},
			wantErr: errMissingField, names: "widget",
		},
		{
			name:    "removeWidget missing widgetId",
			msg:     map[string]interface{}{"op": "removeWidget", "sceneId": "s1"},
			wantErr: errMissingField, names: "widgetId",
		},
		{
			name:    "setPlacement missing placement",
			msg:     map[string]interface{}{"op": "setPlacement", "sceneId": "s1", "widgetId": "w1"},
			wantErr: errMissingField, names: "placement",
		},
		{
			name:    "setWidgetConfig missing config",
			msg:     map[string]interface{}{"op": "setWidgetConfig", "sceneId": "s1", "widgetId": "w1"},
			wantErr: errMissingField, names: "config",
		},
		{
			name:    "setStyle missing scope",
			msg:     map[string]interface{}{"op": "setStyle", "style": map[string]interface{}{}},
			wantErr: errMissingField, names: "scope",
		},
		{
			name:    "setStyle missing style",
			msg:     map[string]interface{}{"op": "setStyle", "scope": "board"},
			wantErr: errMissingField, names: "style",
		},
		{
			name:    "setGrid missing grid",
			msg:     map[string]interface{}{"op": "setGrid"},
			wantErr: errMissingField, names: "grid",
		},
		{
			name:    "setScript missing source",
			msg:     map[string]interface{}{"op": "setScript", "scope": "board"},
			wantErr: errMissingField, names: "source",
		},
		{
			name:    "empty sceneId counts as missing",
			msg:     map[string]interface{}{"op": "removeWidget", "sceneId": "", "widgetId": "w1"},
			wantErr: errMissingField, names: "sceneId",
		},

		{
			name: "unknown top-level key",
			msg: map[string]interface{}{
				"op": "setGrid", "grid": map[string]interface{}{}, "gird": map[string]interface{}{},
			},
			wantErr: errUnknownField, names: "gird",
		},
		{
			// A typo'd field must not be silently ignored: the client would see
			// a successful op that changed nothing it asked for.
			name: "misspelled sceneId",
			msg: map[string]interface{}{
				"op": "addWidget", "sceneID": "s1", "widgetId": "w1", "widget": map[string]interface{}{},
			},
			wantErr: errUnknownField, names: "sceneID",
		},
		{
			name: "field belonging to another op",
			msg: map[string]interface{}{
				"op": "setGrid", "grid": map[string]interface{}{}, "sceneId": "s1",
			},
			wantErr: errUnknownField, names: "sceneId",
		},
		{
			name: "scope on an unscoped op",
			msg: map[string]interface{}{
				"op": "addWidget", "sceneId": "s1", "widgetId": "w1",
				"widget": map[string]interface{}{}, "scope": "board",
			},
			wantErr: errUnknownField, names: "scope",
		},

		{
			name: "sceneId as a number",
			msg: map[string]interface{}{
				"op": "addWidget", "sceneId": 1.0, "widgetId": "w1", "widget": map[string]interface{}{},
			},
			wantErr: errInvalidType, names: "sceneId",
		},
		{
			name:    "widget as a string",
			msg:     map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widgetId": "w1", "widget": "text"},
			wantErr: errInvalidType, names: "widget",
		},
		{
			name: "placement as an array",
			msg: map[string]interface{}{
				"op": "setPlacement", "sceneId": "s1", "widgetId": "w1",
				"placement": []interface{}{1.0, 2.0},
			},
			wantErr: errInvalidType, names: "placement",
		},
		{
			name:    "grid as null",
			msg:     map[string]interface{}{"op": "setGrid", "grid": nil},
			wantErr: errInvalidType, names: "grid",
		},
		{
			name:    "source as a number",
			msg:     map[string]interface{}{"op": "setScript", "scope": "board", "source": 3.0},
			wantErr: errInvalidType, names: "source",
		},
		{
			name: "config as a bool",
			msg: map[string]interface{}{
				"op": "setWidgetConfig", "sceneId": "s1", "widgetId": "w1", "config": true,
			},
			wantErr: errInvalidType, names: "config",
		},

		{
			name: "scene scope without sceneId",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "scene", "style": map[string]interface{}{},
			},
			wantErr: errMissingField, names: "sceneId",
		},
		{
			name: "widget scope without widgetId",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "widget", "sceneId": "s1", "style": map[string]interface{}{},
			},
			wantErr: errMissingField, names: "widgetId",
		},
		{
			name: "widget scope without sceneId",
			msg: map[string]interface{}{
				"op": "setScript", "scope": "widget", "widgetId": "w1", "source": "",
			},
			wantErr: errMissingField, names: "sceneId",
		},
		{
			name: "board scope with a sceneId",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "board", "sceneId": "s1", "style": map[string]interface{}{},
			},
			wantErr: errScope, names: "sceneId",
		},
		{
			name: "board scope with a widgetId",
			msg: map[string]interface{}{
				"op": "setScript", "scope": "board", "widgetId": "w1", "source": "x",
			},
			wantErr: errScope, names: "widgetId",
		},
		{
			name: "scene scope with a widgetId",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "scene", "sceneId": "s1", "widgetId": "w1",
				"style": map[string]interface{}{},
			},
			wantErr: errScope, names: "widgetId",
		},
		{
			name: "unknown scope value",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": "global", "style": map[string]interface{}{},
			},
			wantErr: errScope, names: "global",
		},
		{
			name: "scope as a number",
			msg: map[string]interface{}{
				"op": "setStyle", "scope": 1.0, "style": map[string]interface{}{},
			},
			wantErr: errInvalidType, names: "scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeOp(tt.msg)
			if err == nil {
				t.Fatalf("decodeOp: expected an error, got %+v", got)
			}

			if got != nil {
				t.Fatalf("decodeOp: expected a nil op alongside the error, got %+v", got)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("decodeOp: error %v, want one wrapping %v", err, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.names) {
				t.Fatalf("decodeOp: error %q must name %q", err, tt.names)
			}
		})
	}
}

// TestDecodeOp_ErrorNamesTheOp keeps the repo's error convention: every decode
// failure past the discriminator carries the op it was decoding.
func TestDecodeOp_ErrorNamesTheOp(t *testing.T) {
	_, err := decodeOp(map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widgetId": "w1"})
	if err == nil {
		t.Fatalf("decodeOp: expected an error")
	}

	if !strings.HasPrefix(err.Error(), `decoding layout op "addWidget": `) {
		t.Fatalf("decodeOp: error %q must be prefixed with the op being decoded", err)
	}
}

// TestDecodeOp_IgnoresGameCode pins that the game code rides along in the same
// payload without being an unknown field — the handler reads it separately.
func TestDecodeOp_IgnoresGameCode(t *testing.T) {
	got, err := decodeOp(map[string]interface{}{"op": "setGrid", "code": "ABCD", "grid": map[string]interface{}{}})
	if err != nil {
		t.Fatalf("decodeOp: unexpected error: %v", err)
	}

	if got.SceneID != "" || got.WidgetID != "" || got.Scope != "" {
		t.Fatalf("decodeOp: setGrid must not populate addressing fields, got %+v", got)
	}
}

package layout_test

import (
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/layout"
)

func strPtr(s string) *string { return &s }

// TestDecodeOp_ValidOps verifies each op in the vocabulary decodes successfully
// from its minimal valid wire form.
func TestDecodeOp_ValidOps(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]interface{}
		want layout.Op
	}{
		{
			name: "addWidget",
			msg: map[string]interface{}{
				"op":       "addWidget",
				"sceneId":  "scene1",
				"widgetId": "w1",
				"widget":   map[string]interface{}{"type": "text"},
			},
			want: layout.Op{
				Op:       "addWidget",
				SceneID:  "scene1",
				WidgetID: "w1",
				Widget:   map[string]interface{}{"type": "text"},
			},
		},
		{
			name: "removeWidget",
			msg: map[string]interface{}{
				"op":       "removeWidget",
				"sceneId":  "scene1",
				"widgetId": "w1",
			},
			want: layout.Op{
				Op:       "removeWidget",
				SceneID:  "scene1",
				WidgetID: "w1",
			},
		},
		{
			name: "setPlacement",
			msg: map[string]interface{}{
				"op":        "setPlacement",
				"sceneId":   "scene1",
				"widgetId":  "w1",
				"placement": map[string]interface{}{"x": float64(0), "y": float64(0)},
			},
			want: layout.Op{
				Op:        "setPlacement",
				SceneID:   "scene1",
				WidgetID:  "w1",
				Placement: map[string]interface{}{"x": float64(0), "y": float64(0)},
			},
		},
		{
			name: "setWidgetConfig",
			msg: map[string]interface{}{
				"op":       "setWidgetConfig",
				"sceneId":  "scene1",
				"widgetId": "w1",
				"config":   map[string]interface{}{"color": "red"},
			},
			want: layout.Op{
				Op:       "setWidgetConfig",
				SceneID:  "scene1",
				WidgetID: "w1",
				Config:   map[string]interface{}{"color": "red"},
			},
		},
		{
			name: "setStyle_board_scope",
			msg: map[string]interface{}{
				"op":    "setStyle",
				"scope": "board",
				"style": map[string]interface{}{"background": "#fff"},
			},
			want: layout.Op{
				Op:    "setStyle",
				Scope: "board",
				Style: map[string]interface{}{"background": "#fff"},
			},
		},
		{
			name: "setStyle_scene_scope_with_optional_ids",
			msg: map[string]interface{}{
				"op":       "setStyle",
				"scope":    "scene",
				"sceneId":  "scene1",
				"widgetId": "w1",
				"style":    map[string]interface{}{"color": "blue"},
			},
			want: layout.Op{
				Op:       "setStyle",
				Scope:    "scene",
				SceneID:  "scene1",
				WidgetID: "w1",
				Style:    map[string]interface{}{"color": "blue"},
			},
		},
		{
			name: "setGrid",
			msg: map[string]interface{}{
				"op":   "setGrid",
				"grid": map[string]interface{}{"columns": float64(12)},
			},
			want: layout.Op{
				Op:   "setGrid",
				Grid: map[string]interface{}{"columns": float64(12)},
			},
		},
		{
			name: "setScript_board_scope",
			msg: map[string]interface{}{
				"op":     "setScript",
				"scope":  "board",
				"source": "return true",
			},
			want: layout.Op{
				Op:     "setScript",
				Scope:  "board",
				Source: strPtr("return true"),
			},
		},
		{
			name: "setScript_widget_scope_with_optional_ids",
			msg: map[string]interface{}{
				"op":       "setScript",
				"scope":    "widget",
				"sceneId":  "scene1",
				"widgetId": "w1",
				"source":   "return false",
			},
			want: layout.Op{
				Op:       "setScript",
				Scope:    "widget",
				SceneID:  "scene1",
				WidgetID: "w1",
				Source:   strPtr("return false"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := layout.DecodeOp(tt.msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Op != tt.want.Op {
				t.Errorf("Op: got %q, want %q", got.Op, tt.want.Op)
			}
			if got.SceneID != tt.want.SceneID {
				t.Errorf("SceneID: got %q, want %q", got.SceneID, tt.want.SceneID)
			}
			if got.WidgetID != tt.want.WidgetID {
				t.Errorf("WidgetID: got %q, want %q", got.WidgetID, tt.want.WidgetID)
			}
			if got.Scope != tt.want.Scope {
				t.Errorf("Scope: got %q, want %q", got.Scope, tt.want.Scope)
			}
			if tt.want.Source == nil && got.Source != nil {
				t.Errorf("Source: got %q, want nil", *got.Source)
			}
			if tt.want.Source != nil {
				if got.Source == nil {
					t.Errorf("Source: got nil, want %q", *tt.want.Source)
				} else if *got.Source != *tt.want.Source {
					t.Errorf("Source: got %q, want %q", *got.Source, *tt.want.Source)
				}
			}
		})
	}
}

// TestDecodeOp_MissingRequiredField verifies that each missing required field
// produces an error that names the field.
func TestDecodeOp_MissingRequiredField(t *testing.T) {
	tests := []struct {
		name      string
		msg       map[string]interface{}
		wantField string
	}{
		{
			name:      "addWidget_missing_sceneId",
			msg:       map[string]interface{}{"op": "addWidget", "widgetId": "w1", "widget": map[string]interface{}{}},
			wantField: "sceneId",
		},
		{
			name:      "addWidget_missing_widgetId",
			msg:       map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widget": map[string]interface{}{}},
			wantField: "widgetId",
		},
		{
			name:      "addWidget_missing_widget",
			msg:       map[string]interface{}{"op": "addWidget", "sceneId": "s1", "widgetId": "w1"},
			wantField: "widget",
		},
		{
			name:      "removeWidget_missing_sceneId",
			msg:       map[string]interface{}{"op": "removeWidget", "widgetId": "w1"},
			wantField: "sceneId",
		},
		{
			name:      "removeWidget_missing_widgetId",
			msg:       map[string]interface{}{"op": "removeWidget", "sceneId": "s1"},
			wantField: "widgetId",
		},
		{
			name:      "setPlacement_missing_sceneId",
			msg:       map[string]interface{}{"op": "setPlacement", "widgetId": "w1", "placement": map[string]interface{}{}},
			wantField: "sceneId",
		},
		{
			name:      "setPlacement_missing_widgetId",
			msg:       map[string]interface{}{"op": "setPlacement", "sceneId": "s1", "placement": map[string]interface{}{}},
			wantField: "widgetId",
		},
		{
			name:      "setPlacement_missing_placement",
			msg:       map[string]interface{}{"op": "setPlacement", "sceneId": "s1", "widgetId": "w1"},
			wantField: "placement",
		},
		{
			name:      "setWidgetConfig_missing_sceneId",
			msg:       map[string]interface{}{"op": "setWidgetConfig", "widgetId": "w1", "config": map[string]interface{}{}},
			wantField: "sceneId",
		},
		{
			name:      "setWidgetConfig_missing_widgetId",
			msg:       map[string]interface{}{"op": "setWidgetConfig", "sceneId": "s1", "config": map[string]interface{}{}},
			wantField: "widgetId",
		},
		{
			name:      "setWidgetConfig_missing_config",
			msg:       map[string]interface{}{"op": "setWidgetConfig", "sceneId": "s1", "widgetId": "w1"},
			wantField: "config",
		},
		{
			name:      "setStyle_missing_scope",
			msg:       map[string]interface{}{"op": "setStyle", "style": map[string]interface{}{}},
			wantField: "scope",
		},
		{
			name:      "setStyle_missing_style",
			msg:       map[string]interface{}{"op": "setStyle", "scope": "board"},
			wantField: "style",
		},
		{
			name:      "setGrid_missing_grid",
			msg:       map[string]interface{}{"op": "setGrid"},
			wantField: "grid",
		},
		{
			name:      "setScript_missing_scope",
			msg:       map[string]interface{}{"op": "setScript", "source": "x"},
			wantField: "scope",
		},
		{
			name:      "setScript_missing_source",
			msg:       map[string]interface{}{"op": "setScript", "scope": "board"},
			wantField: "source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := layout.DecodeOp(tt.msg)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			var fieldErr *layout.MissingFieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("expected MissingFieldError, got %T: %v", err, err)
			}
			if fieldErr.Field != tt.wantField {
				t.Errorf("MissingFieldError.Field: got %q, want %q", fieldErr.Field, tt.wantField)
			}
		})
	}
}

// TestDecodeOp_MissingOp verifies that an empty or absent op field is rejected.
func TestDecodeOp_MissingOp(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]interface{}
	}{
		{
			name: "absent_op",
			msg:  map[string]interface{}{"sceneId": "s1"},
		},
		{
			name: "empty_op",
			msg:  map[string]interface{}{"op": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := layout.DecodeOp(tt.msg)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// TestDecodeOp_UnknownOp verifies that an unrecognised op value is rejected.
func TestDecodeOp_UnknownOp(t *testing.T) {
	_, err := layout.DecodeOp(map[string]interface{}{
		"op": "doSomethingWeird",
	})
	if err == nil {
		t.Fatal("expected an error for unknown op, got nil")
	}
	var unknownErr *layout.UnknownOpError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownOpError, got %T: %v", err, err)
	}
	if unknownErr.Op != "doSomethingWeird" {
		t.Errorf("UnknownOpError.Op: got %q, want %q", unknownErr.Op, "doSomethingWeird")
	}
}

// TestDecodeOp_UnknownTopLevelKey verifies that keys outside the Op struct's
// vocabulary are rejected.
func TestDecodeOp_UnknownTopLevelKey(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]interface{}
	}{
		{
			name: "extra_key_on_addWidget",
			msg: map[string]interface{}{
				"op":       "addWidget",
				"sceneId":  "s1",
				"widgetId": "w1",
				"widget":   map[string]interface{}{},
				"bogus":    "value",
			},
		},
		{
			name: "extra_key_on_setGrid",
			msg: map[string]interface{}{
				"op":      "setGrid",
				"grid":    map[string]interface{}{},
				"mystery": 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := layout.DecodeOp(tt.msg)
			if err == nil {
				t.Fatal("expected an error for unknown top-level key, got nil")
			}
			var unknownKeyErr *layout.UnknownKeyError
			if !errors.As(err, &unknownKeyErr) {
				t.Fatalf("expected UnknownKeyError, got %T: %v", err, err)
			}
		})
	}
}

// TestDecodeOp_AllowedPassThrough verifies that the "code" key (read by the
// handler before decodeOp is called) does not trigger an unknown-key error.
func TestDecodeOp_AllowedPassThrough(t *testing.T) {
	_, err := layout.DecodeOp(map[string]interface{}{
		"op":       "removeWidget",
		"sceneId":  "s1",
		"widgetId": "w1",
		"code":     "GAME01",
	})
	if err != nil {
		t.Fatalf("unexpected error when code is present: %v", err)
	}
}

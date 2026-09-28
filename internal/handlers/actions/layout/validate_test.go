package layout_test

import (
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/layout"
)

// baseLayout returns a minimal valid layout for use in tests.
// grid: 10x10, one scene with one non-overlapping widget.
func baseLayout() map[string]interface{} {
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

// sceneWidgets is a helper to reach into the layout and return the widgets map
// for the named scene, cast to map[string]interface{}.
func sceneWidgets(layout map[string]interface{}, sceneID string) map[string]interface{} {
	scenes := layout["scenes"].(map[string]interface{})
	scene := scenes[sceneID].(map[string]interface{})
	return scene["widgets"].(map[string]interface{})
}

func TestValidateLayout_ValidBase(t *testing.T) {
	if err := layout.ValidateLayout(baseLayout()); err != nil {
		t.Fatalf("expected valid layout to pass, got: %v", err)
	}
}

// ── Grid dimension tests ──────────────────────────────────────────────────────

func TestValidateLayout_GridDims(t *testing.T) {
	tests := []struct {
		name    string
		cols    float64
		rows    float64
		wantErr bool
	}{
		{"below_min_cols", 7, 10, true},
		{"below_min_rows", 10, 7, true},
		{"at_min_cols", 8, 10, false},
		{"at_min_rows", 10, 8, false},
		{"above_max_cols", 4097, 10, true},
		{"above_max_rows", 10, 4097, true},
		{"at_max_cols", 4096, 10, false},
		{"at_max_rows", 10, 4096, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := baseLayout()
			l["grid"] = map[string]interface{}{
				"cols": tt.cols,
				"rows": tt.rows,
			}
			err := layout.ValidateLayout(l)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateLayout_GridDims_Fractional(t *testing.T) {
	tests := []struct {
		name string
		cols float64
		rows float64
	}{
		{"fractional_cols", 10.5, 10},
		{"fractional_rows", 10, 10.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := baseLayout()
			l["grid"] = map[string]interface{}{
				"cols": tt.cols,
				"rows": tt.rows,
			}
			if err := layout.ValidateLayout(l); err == nil {
				t.Error("expected error for fractional grid dim, got nil")
			}
		})
	}
}

func TestValidateLayout_GridDims_Zero(t *testing.T) {
	l := baseLayout()
	l["grid"] = map[string]interface{}{
		"cols": float64(0),
		"rows": float64(10),
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for zero cols, got nil")
	}
}

// ── Widget placement (w/h) tests ──────────────────────────────────────────────

func TestValidateLayout_WidgetPlacement_WH(t *testing.T) {
	tests := []struct {
		name    string
		w       float64
		h       float64
		wantErr bool
	}{
		{"negative_w", -1, 2, true},
		{"zero_w", 0, 2, true},
		{"negative_h", 2, -1, true},
		{"zero_h", 2, 0, true},
		{"fractional_w", 1.5, 2, true},
		{"fractional_h", 2, 1.5, true},
		{"valid_1x1", 1, 1, false},
		{"valid_2x2", 2, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := baseLayout()
			widgets := sceneWidgets(l, "scene1")
			widgets["w1"] = map[string]interface{}{
				"type": "text",
				"placement": map[string]interface{}{
					"kind": "grid",
					"col":  float64(0),
					"row":  float64(0),
					"w":    tt.w,
					"h":    tt.h,
				},
			}
			err := layout.ValidateLayout(l)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for w=%v h=%v, got nil", tt.w, tt.h)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error for w=%v h=%v, got: %v", tt.w, tt.h, err)
			}
		})
	}
}

// ── Widget out-of-bounds tests ────────────────────────────────────────────────

func TestValidateLayout_WidgetPlacement_OutOfBounds(t *testing.T) {
	tests := []struct {
		name    string
		col     float64
		row     float64
		w       float64
		h       float64
		wantErr bool
	}{
		// grid is 10x10
		{"fits_exactly", 0, 0, 10, 10, false},
		{"col_plus_w_exceeds", 5, 0, 6, 1, true},
		{"row_plus_h_exceeds", 0, 5, 1, 6, true},
		{"col_plus_w_exact", 5, 0, 5, 1, false},
		{"row_plus_h_exact", 0, 5, 1, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := baseLayout()
			widgets := sceneWidgets(l, "scene1")
			widgets["w1"] = map[string]interface{}{
				"type": "text",
				"placement": map[string]interface{}{
					"kind": "grid",
					"col":  tt.col,
					"row":  tt.row,
					"w":    tt.w,
					"h":    tt.h,
				},
			}
			err := layout.ValidateLayout(l)
			if tt.wantErr && err == nil {
				t.Errorf("expected out-of-bounds error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// ── Overlap tests ─────────────────────────────────────────────────────────────

func TestValidateLayout_Overlap_TwoGridWidgets(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	// w1: (0,0) 2x2 — already set by baseLayout
	// w2: (1,1) 2x2 — overlaps with w1
	widgets["w2"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind": "grid",
			"col":  float64(1),
			"row":  float64(1),
			"w":    float64(2),
			"h":    float64(2),
		},
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected overlap error for two overlapping grid widgets, got nil")
	}
}

func TestValidateLayout_Overlap_EdgeTouching_NotCollision(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	// w1: (0,0) 2x2 — right edge at col=2
	// w2: (2,0) 2x2 — left edge at col=2 — touching, not overlapping
	widgets["w2"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind": "grid",
			"col":  float64(2),
			"row":  float64(0),
			"w":    float64(2),
			"h":    float64(2),
		},
	}
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("edge-touching widgets must not be a collision, got: %v", err)
	}
}

func TestValidateLayout_Overlap_TwoAbsoluteWidgets_Allowed(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	// Replace w1 with absolute, add w2 also absolute — overlap allowed
	widgets["w1"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind":   "absolute",
			"left":   "10%",
			"top":    "10%",
			"right":  "50%",
			"bottom": "50%",
		},
	}
	widgets["w2"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind":   "absolute",
			"left":   "20%",
			"top":    "20%",
			"right":  "60%",
			"bottom": "60%",
		},
	}
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("overlapping absolute widgets must be allowed, got: %v", err)
	}
}

func TestValidateLayout_Overlap_AbsoluteAndGrid_NoConflict(t *testing.T) {
	// Absolute widget excluded from both sides of overlap check:
	// a grid widget and an absolute widget in the same "cells" must pass.
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	// w1 is already grid (0,0) 2x2
	// w2 absolute — excluded from overlap set
	widgets["w2"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind":   "absolute",
			"left":   "0%",
			"top":    "0%",
			"right":  "20%",
			"bottom": "20%",
		},
	}
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("absolute widget must be excluded from overlap set, got: %v", err)
	}
}

// ── Absolute placement pixel value tests ─────────────────────────────────────

func TestValidateLayout_AbsolutePlacement_PixelValue_Rejected(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	widgets["w1"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind":   "absolute",
			"left":   float64(10), // pixel value — rejected
			"top":    "10%",
			"right":  "50%",
			"bottom": "50%",
		},
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for pixel value in absolute placement, got nil")
	}
}

func TestValidateLayout_AbsolutePlacement_PercentageOnly_Allowed(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
	widgets["w1"] = map[string]interface{}{
		"type": "text",
		"placement": map[string]interface{}{
			"kind":   "absolute",
			"left":   "10%",
			"top":    "10%",
			"right":  "50%",
			"bottom": "50%",
		},
	}
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("percentage-only absolute placement must be allowed, got: %v", err)
	}
}

// ── Subgrid depth tests ───────────────────────────────────────────────────────

// buildSubgridAtDepth creates a layout where a subgrid is nested depth levels deep.
// depth=1 means a top-level widget of type "subgrid"; depth=2 means a subgrid inside
// that subgrid's config.widgets; and so on. This matches the TS schema where subgrid
// children live at widget.config.widgets, not at the widget root.
func buildSubgridAtDepth(depth int) map[string]interface{} {
	// Build from the inside out.
	// Innermost widget is a subgrid with an empty config.widgets.
	innerWidget := map[string]interface{}{
		"type": "subgrid",
		"placement": map[string]interface{}{
			"kind": "grid",
			"col":  float64(0),
			"row":  float64(0),
			"w":    float64(2),
			"h":    float64(2),
		},
		"config": map[string]interface{}{
			"widgets": map[string]interface{}{},
		},
	}

	// Wrap it in subgrid containers depth-1 times.
	current := innerWidget
	for i := 1; i < depth; i++ {
		current = map[string]interface{}{
			"type": "subgrid",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(0),
				"row":  float64(0),
				"w":    float64(4),
				"h":    float64(4),
			},
			"config": map[string]interface{}{
				"widgets": map[string]interface{}{
					"inner": current,
				},
			},
		}
	}

	return map[string]interface{}{
		"grid": map[string]interface{}{
			"cols": float64(10),
			"rows": float64(10),
		},
		"scenes": map[string]interface{}{
			"scene1": map[string]interface{}{
				"widgets": map[string]interface{}{
					"sg1": current,
				},
			},
		},
	}
}

func TestValidateLayout_SubgridDepth_Four_Allowed(t *testing.T) {
	l := buildSubgridAtDepth(4)
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("subgrid at depth 4 must be allowed, got: %v", err)
	}
}

func TestValidateLayout_SubgridDepth_Five_Rejected(t *testing.T) {
	l := buildSubgridAtDepth(5)
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for subgrid at depth 5, got nil")
	}
}

func TestValidateLayout_SubgridDepth_One_Allowed(t *testing.T) {
	l := buildSubgridAtDepth(1)
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("subgrid at depth 1 must be allowed, got: %v", err)
	}
}

// ── privateData key tests ─────────────────────────────────────────────────────

func TestValidateLayout_PrivateData_TopLevel_Rejected(t *testing.T) {
	l := baseLayout()
	l["privateData"] = map[string]interface{}{"secret": "value"}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for privateData at top level, got nil")
	}
}

func TestValidateLayout_PrivateData_ThreeLevelsDeep_Rejected(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
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
			"nested": map[string]interface{}{
				"privateData": "should not be here",
			},
		},
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for privateData nested three levels in widget config, got nil")
	}
}

func TestValidateLayout_PrivateData_InsideArray_Rejected(t *testing.T) {
	l := baseLayout()
	widgets := sceneWidgets(l, "scene1")
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
			"items": []interface{}{
				map[string]interface{}{"privateData": "secret"},
			},
		},
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for privateData inside an array element, got nil")
	}
}

func TestValidateLayout_PrivateData_InScene_Rejected(t *testing.T) {
	l := baseLayout()
	scenes := l["scenes"].(map[string]interface{})
	scenes["scene1"] = map[string]interface{}{
		"privateData": map[string]interface{}{"secret": "value"},
		"widgets":     map[string]interface{}{},
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for privateData inside a scene, got nil")
	}
}

// ── Widget count tests ────────────────────────────────────────────────────────

func TestValidateLayout_WidgetCount_AtLimit_Allowed(t *testing.T) {
	l := map[string]interface{}{
		"grid": map[string]interface{}{
			"cols": float64(4096),
			"rows": float64(4096),
		},
		"scenes": map[string]interface{}{},
	}
	scenes := l["scenes"].(map[string]interface{})

	// Put 300 widgets across two scenes: 150 each, no overlaps.
	// Use a large enough grid (4096x4096) and spread them in a row.
	scene1 := map[string]interface{}{"widgets": map[string]interface{}{}}
	scene2 := map[string]interface{}{"widgets": map[string]interface{}{}}

	w1 := scene1["widgets"].(map[string]interface{})
	w2 := scene2["widgets"].(map[string]interface{})

	for i := 0; i < 150; i++ {
		id := strings.Repeat("a", i+1) // unique IDs of different lengths
		w1[id] = map[string]interface{}{
			"type": "text",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(i),
				"row":  float64(0),
				"w":    float64(1),
				"h":    float64(1),
			},
		}
		w2[id] = map[string]interface{}{
			"type": "text",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(i),
				"row":  float64(0),
				"w":    float64(1),
				"h":    float64(1),
			},
		}
	}

	scenes["scene1"] = scene1
	scenes["scene2"] = scene2

	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("300 widgets (at limit) must be allowed, got: %v", err)
	}
}

func TestValidateLayout_WidgetCount_OverLimit_Rejected(t *testing.T) {
	l := map[string]interface{}{
		"grid": map[string]interface{}{
			"cols": float64(4096),
			"rows": float64(4096),
		},
		"scenes": map[string]interface{}{},
	}
	scenes := l["scenes"].(map[string]interface{})

	scene1 := map[string]interface{}{"widgets": map[string]interface{}{}}
	w1 := scene1["widgets"].(map[string]interface{})

	for i := 0; i < 301; i++ {
		id := strings.Repeat("a", i+1)
		w1[id] = map[string]interface{}{
			"type": "text",
			"placement": map[string]interface{}{
				"kind": "grid",
				"col":  float64(i),
				"row":  float64(0),
				"w":    float64(1),
				"h":    float64(1),
			},
		}
	}

	scenes["scene1"] = scene1

	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for 301 widgets (over limit), got nil")
	}
}

// ── Serialized size tests ─────────────────────────────────────────────────────

func TestValidateLayout_SerializedSize_OverLimit_Rejected(t *testing.T) {
	// Construct a layout that is valid in all other ways but exceeds 256 KB when
	// serialized. We do this by adding a large string to the grid's style field.
	l := baseLayout()
	// 256 KB = 262144 bytes; add a 270000-char string which ensures we exceed it.
	l["style"] = map[string]interface{}{
		"comment": strings.Repeat("x", 270000),
	}
	if err := layout.ValidateLayout(l); err == nil {
		t.Error("expected error for layout exceeding 256 KB, got nil")
	}
}

func TestValidateLayout_SerializedSize_AtLimit_Allowed(t *testing.T) {
	// A normal small layout is well under 256 KB.
	l := baseLayout()
	if err := layout.ValidateLayout(l); err != nil {
		t.Errorf("small layout must be under the size limit, got: %v", err)
	}
}

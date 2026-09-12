package layout

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

// Fixture builders. Layouts are deep and repetitive, so every test below states
// only the part it is about and inherits a known-good frame from these.

func gridPlacement(col, row, w, h interface{}) map[string]interface{} {
	return map[string]interface{}{"kind": "grid", "col": col, "row": row, "w": w, "h": h}
}

func absolutePlacement(left, top, width, height interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind": "absolute", "left": left, "top": top, "width": width, "height": height,
	}
}

func widgetAt(placement map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "text", "placement": placement}
}

// subGridAt wraps widgets in a sub-grid widget with its own 8x8 coordinate
// space, which is the smallest grid minDim allows.
func subGridAt(placement, widgets map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":      subGridType,
		"placement": placement,
		"config": map[string]interface{}{
			"grid":    map[string]interface{}{"cols": 8, "rows": 8},
			"widgets": widgets,
		},
	}
}

// layoutWith puts widgets in a single scene on a 24x16 board.
func layoutWith(widgets map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"grid": map[string]interface{}{"cols": 24, "rows": 16},
		"scenes": map[string]interface{}{
			"board": map[string]interface{}{"widgets": widgets},
		},
	}
}

// nestedSubGrids builds a layout whose deepest widget lives at the given
// nesting level: level 1 is the scene's own widgets, so `levels` wrappers minus
// one sub-grids are stacked above the leaf.
func nestedSubGrids(levels int) map[string]interface{} {
	widgets := map[string]interface{}{"leaf": widgetAt(gridPlacement(0, 0, 1, 1))}

	for i := levels; i > 1; i-- {
		widgets = map[string]interface{}{"sub": subGridAt(gridPlacement(0, 0, 4, 4), widgets)}
	}

	return layoutWith(widgets)
}

// manyWidgets returns n absolutely placed widgets, which are exempt from
// collision and therefore isolate the count cap from the overlap rule.
func manyWidgets(n int) map[string]interface{} {
	widgets := make(map[string]interface{}, n)

	for i := range n {
		widgets[fmt.Sprintf("w%d", i)] = widgetAt(absolutePlacement("0%", "0%", "10%", "10%"))
	}

	return widgets
}

// TestOverlaps_MatchesClientCollides pins the AABB contract this file
// duplicates from collides() in client/layout/grid/collision.ts. Strict "<" on
// every edge, and no identity exemption — self-exemption belongs to the caller.
func TestOverlaps_MatchesClientCollides(t *testing.T) {
	tests := []struct {
		name string
		a, b rect
		want bool
	}{
		{name: "identical rects overlap", a: rect{0, 0, 2, 2}, b: rect{0, 0, 2, 2}, want: true},
		{name: "partial overlap", a: rect{0, 0, 3, 3}, b: rect{2, 2, 3, 3}, want: true},
		{name: "contained", a: rect{0, 0, 8, 8}, b: rect{2, 2, 1, 1}, want: true},
		{name: "flush horizontally is not a collision", a: rect{0, 0, 2, 2}, b: rect{2, 0, 2, 2}},
		{name: "flush vertically is not a collision", a: rect{0, 0, 2, 2}, b: rect{0, 2, 2, 2}},
		{name: "diagonally flush is not a collision", a: rect{0, 0, 2, 2}, b: rect{2, 2, 2, 2}},
		{name: "disjoint", a: rect{0, 0, 1, 1}, b: rect{5, 5, 1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overlaps(tt.a, tt.b); got != tt.want {
				t.Fatalf("overlaps(%+v, %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}

			// Pure geometry is symmetric; an asymmetric result would make the
			// pairwise scan depend on map iteration order.
			if got := overlaps(tt.b, tt.a); got != tt.want {
				t.Fatalf("overlaps(%+v, %+v) = %v, want %v (not symmetric)", tt.b, tt.a, got, tt.want)
			}
		})
	}
}

// TestValidateLayout_Accepts covers the layouts that must survive validation.
// Several of these look like violations and are not — the exemptions are the
// point, so each one is pinned rather than left to be "fixed" later.
func TestValidateLayout_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		layout map[string]interface{}
	}{
		{
			// The everyday case: two scenes, both placement kinds, a nested
			// sub-grid, styles, scripts and opaque widget configs.
			name: "non-trivial layout",
			layout: map[string]interface{}{
				"grid":   map[string]interface{}{"cols": 24, "rows": 16},
				"style":  map[string]interface{}{"backgroundColor": "#101010"},
				"script": "return 1",
				"scenes": map[string]interface{}{
					"board": map[string]interface{}{
						"style":  map[string]interface{}{"opacity": 0.5},
						"script": "",
						"widgets": map[string]interface{}{
							"title": map[string]interface{}{
								"type":      "text",
								"placement": gridPlacement(0, 0, 6, 2),
								"style":     map[string]interface{}{"backgroundColor": "red"},
								"config":    map[string]interface{}{"text": "hello"},
								"script":    "return 2",
							},
							"badge": widgetAt(absolutePlacement("10%", "20.5%", "30%", "-5%")),
							"panel": subGridAt(gridPlacement(0, 4, 8, 8), map[string]interface{}{
								"inner": widgetAt(gridPlacement(1, 1, 2, 2)),
							}),
						},
					},
					"lobby": map[string]interface{}{
						"widgets": map[string]interface{}{
							"note": widgetAt(gridPlacement(20, 12, 4, 4)),
						},
					},
				},
			},
		},
		{
			name:   "empty scene has no widgets to check",
			layout: layoutWith(map[string]interface{}{}),
		},
		{
			name: "no scenes at all",
			layout: map[string]interface{}{
				"grid":   map[string]interface{}{"cols": 8, "rows": 8},
				"scenes": map[string]interface{}{},
			},
		},
		{
			name: "grid at the minimum dimension",
			layout: map[string]interface{}{
				"grid":   map[string]interface{}{"cols": minDim, "rows": minDim},
				"scenes": map[string]interface{}{},
			},
		},
		{
			name: "grid at the maximum dimension",
			layout: map[string]interface{}{
				"grid":   map[string]interface{}{"cols": maxDim, "rows": maxDim},
				"scenes": map[string]interface{}{},
			},
		},
		{
			// Flush edges are not a collision: a rect ending at col 2 sits
			// against one starting at col 2.
			name: "edge-touching rects do not collide",
			layout: layoutWith(map[string]interface{}{
				"a": widgetAt(gridPlacement(0, 0, 2, 2)),
				"b": widgetAt(gridPlacement(2, 0, 2, 2)),
				"c": widgetAt(gridPlacement(0, 2, 2, 2)),
			}),
		},
		{
			// Absolute placements are exempt from collision as BOTH subject and
			// obstacle, which is expressed by never entering the overlap set.
			name: "two absolute widgets may overlap exactly",
			layout: layoutWith(map[string]interface{}{
				"a": widgetAt(absolutePlacement("0%", "0%", "50%", "50%")),
				"b": widgetAt(absolutePlacement("0%", "0%", "50%", "50%")),
			}),
		},
		{
			name: "an absolute widget is not an obstacle for a grid widget",
			layout: layoutWith(map[string]interface{}{
				"under": widgetAt(gridPlacement(0, 0, 4, 4)),
				"over":  widgetAt(absolutePlacement("0%", "0%", "100%", "100%")),
			}),
		},
		{
			name:   "optional z on an absolute placement",
			layout: layoutWith(map[string]interface{}{"a": widgetAt(withZ(absolutePlacement("0%", "0%", "1%", "1%"), 3))}),
		},
		{
			// Collision is scoped per grid level: the child sits at (4,0) in
			// the sub-grid's own space, which is a different coordinate space
			// from the parent's (4,0) where "sibling" lives.
			name: "a sub-grid child does not collide with the parent level",
			layout: layoutWith(map[string]interface{}{
				"panel":   subGridAt(gridPlacement(0, 0, 4, 4), map[string]interface{}{"inner": widgetAt(gridPlacement(4, 0, 2, 2))}),
				"sibling": widgetAt(gridPlacement(4, 0, 2, 2)),
			}),
		},
		{
			name:   "sub-grid nesting at the maximum depth",
			layout: nestedSubGrids(maxDepth),
		},
		{
			name:   "exactly the maximum widget count",
			layout: layoutWith(manyWidgets(maxWidgets)),
		},
		{
			// The ADR: Indri is a framework and does not know what a "text"
			// widget is, so config contents are opaque. Anything JSON-shaped
			// goes, including keys that look structural.
			name: "widget config is opaque",
			layout: layoutWith(map[string]interface{}{
				"a": map[string]interface{}{
					"type":      "chart",
					"placement": gridPlacement(0, 0, 1, 1),
					"config": map[string]interface{}{
						"grid":    "this is not a grid",
						"col":     -99,
						"widgets": []interface{}{1.0, "two", nil, true},
						"nested":  map[string]interface{}{"deep": map[string]interface{}{"deeper": 1.0}},
					},
				},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateLayout(tt.layout); err != nil {
				t.Fatalf("validateLayout: unexpected error: %v", err)
			}
		})
	}
}

func withZ(placement map[string]interface{}, z interface{}) map[string]interface{} {
	placement["z"] = z

	return placement
}

// TestValidateLayout_Rejects is the security half of this file: every case is
// something the client's validator only warns about, or repairs, or never sees.
// Each asserts the error class and that the message names the offending path —
// a bare "invalid layout" leaves an author with nothing to fix.
func TestValidateLayout_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		layout  map[string]interface{}
		wantErr error
		names   string // must appear in the message
	}{
		{
			name:    "nil layout",
			layout:  nil,
			wantErr: errMissingField, names: "layout",
		},
		{
			name:    "missing grid",
			layout:  map[string]interface{}{"scenes": map[string]interface{}{}},
			wantErr: errMissingField, names: "grid",
		},
		{
			name:    "missing scenes",
			layout:  map[string]interface{}{"grid": map[string]interface{}{"cols": 8, "rows": 8}},
			wantErr: errMissingField, names: "scenes",
		},

		{
			name: "grid cols below the minimum",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 7, "rows": 16}, "scenes": map[string]interface{}{},
			},
			wantErr: errDimension, names: "grid.cols",
		},
		{
			name: "grid rows above the maximum",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 16, "rows": 4097}, "scenes": map[string]interface{}{},
			},
			wantErr: errDimension, names: "grid.rows",
		},
		{
			name: "grid dimension of zero",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 0, "rows": 16}, "scenes": map[string]interface{}{},
			},
			wantErr: errDimension, names: "grid.cols",
		},
		{
			name: "non-integer grid dimension",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 24.5, "rows": 16}, "scenes": map[string]interface{}{},
			},
			wantErr: errInvalidType, names: "grid.cols",
		},
		{
			name: "grid dimension as a string",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": "24", "rows": 16}, "scenes": map[string]interface{}{},
			},
			wantErr: errInvalidType, names: "grid.cols",
		},
		{
			name: "sub-grid dimension below the minimum",
			layout: layoutWith(map[string]interface{}{
				"panel": map[string]interface{}{
					"type":      subGridType,
					"placement": gridPlacement(0, 0, 4, 4),
					"config": map[string]interface{}{
						"grid":    map[string]interface{}{"cols": 4, "rows": 8},
						"widgets": map[string]interface{}{},
					},
				},
			}),
			wantErr: errDimension, names: "panel.config.grid.cols",
		},

		{
			name:    "negative col",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(-1, 0, 2, 2))}),
			wantErr: errRect, names: "a.placement",
		},
		{
			name:    "negative row",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0, -3, 2, 2))}),
			wantErr: errRect, names: "a.placement",
		},
		{
			// A zero-area widget is invisible but still eats input.
			name:    "zero width",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0, 0, 0, 2))}),
			wantErr: errRect, names: "a.placement",
		},
		{
			name:    "negative height",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0, 0, 2, -2))}),
			wantErr: errRect, names: "a.placement",
		},
		{
			// Fractional cells would make the percentage boxes disagree with
			// what an editor snaps to.
			name:    "fractional width",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0, 0, 2.5, 2))}),
			wantErr: errInvalidType, names: "a.placement.w",
		},
		{
			name:    "fractional col",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0.5, 0, 2, 2))}),
			wantErr: errInvalidType, names: "a.placement.col",
		},
		{
			name:    "rect exceeds the grid on the column axis",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(20, 0, 8, 2))}),
			wantErr: errBounds, names: "a.placement",
		},
		{
			name:    "rect exceeds the grid on the row axis",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(0, 15, 2, 2))}),
			wantErr: errBounds, names: "a.placement",
		},
		{
			name: "sub-grid child exceeds its own grid, not the parent's",
			layout: layoutWith(map[string]interface{}{
				"panel": subGridAt(gridPlacement(0, 0, 4, 4), map[string]interface{}{
					// Fits the 24x16 board, does not fit the 8x8 sub-grid.
					"inner": widgetAt(gridPlacement(0, 0, 12, 2)),
				}),
			}),
			wantErr: errBounds, names: "panel.config.widgets.inner.placement",
		},
		{
			name:    "rect coordinate too large to be a coordinate",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(1e18, 0, 2, 2))}),
			wantErr: errInvalidType, names: "a.placement.col",
		},
		{
			name:    "missing rect component",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(map[string]interface{}{"kind": "grid", "col": 0, "row": 0, "w": 2})}),
			wantErr: errMissingField, names: "a.placement.h",
		},

		{
			name: "two overlapping grid widgets",
			layout: layoutWith(map[string]interface{}{
				"a": widgetAt(gridPlacement(0, 0, 4, 4)),
				"b": widgetAt(gridPlacement(3, 3, 4, 4)),
			}),
			wantErr: errOverlap, names: "scenes.board.widgets.a",
		},
		{
			name: "overlapping siblings inside a sub-grid",
			layout: layoutWith(map[string]interface{}{
				"panel": subGridAt(gridPlacement(0, 0, 4, 4), map[string]interface{}{
					"a": widgetAt(gridPlacement(0, 0, 4, 4)),
					"b": widgetAt(gridPlacement(2, 2, 4, 4)),
				}),
			}),
			wantErr: errOverlap, names: "panel.config.widgets.a",
		},

		{
			// A bare number is the wrong FORMAT, not a sloppy value: there is
			// no way to read "40" as a percentage rather than 40 pixels.
			name:    "pixel value as a number in an absolute placement",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(absolutePlacement(40, "0%", "10%", "10%"))}),
			wantErr: errInvalidType, names: "a.placement.left",
		},
		{
			name:    "pixel value as a string in an absolute placement",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(absolutePlacement("40px", "0%", "10%", "10%"))}),
			wantErr: errPercent, names: "a.placement.left",
		},
		{
			name:    "unitless string in an absolute placement",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(absolutePlacement("0%", "40", "10%", "10%"))}),
			wantErr: errPercent, names: "a.placement.top",
		},
		{
			name:    "missing absolute component",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(map[string]interface{}{"kind": "absolute", "left": "0%", "top": "0%", "width": "1%"})}),
			wantErr: errMissingField, names: "a.placement.height",
		},
		{
			name:    "fractional z on an absolute placement",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(withZ(absolutePlacement("0%", "0%", "1%", "1%"), 1.5))}),
			wantErr: errInvalidType, names: "a.placement.z",
		},

		{
			// The path proves the rejection happens at the level maxDepth
			// allows and not one level earlier.
			name:    "sub-grid nesting past the maximum depth",
			layout:  nestedSubGrids(maxDepth + 1),
			wantErr: errDepth, names: "sub.config.widgets.sub.config.widgets.sub.config.widgets.sub.config",
		},
		{
			name: "sub-grid config missing its widgets",
			layout: layoutWith(map[string]interface{}{
				"panel": map[string]interface{}{
					"type":      subGridType,
					"placement": gridPlacement(0, 0, 4, 4),
					"config":    map[string]interface{}{"grid": map[string]interface{}{"cols": 8, "rows": 8}},
				},
			}),
			wantErr: errMissingField, names: "panel.config.widgets",
		},

		{
			name: "reserved key at the top level",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 8, "rows": 8}, "scenes": map[string]interface{}{},
				"privateData": map[string]interface{}{"x": 1},
			},
			wantErr: errReserved, names: "privateData",
		},
		{
			// Three levels below the widget, inside an opaque config the server
			// otherwise never reads — the one thing it always reads it for.
			name: "reserved key nested three levels deep in a config",
			layout: layoutWith(map[string]interface{}{
				"a": map[string]interface{}{
					"type":      "text",
					"placement": gridPlacement(0, 0, 1, 1),
					"config": map[string]interface{}{
						"one": map[string]interface{}{
							"two": map[string]interface{}{
								"privateData": "secret",
							},
						},
					},
				},
			}),
			wantErr: errReserved, names: "scenes.board.widgets.a.config.one.two.privateData",
		},
		{
			name: "reserved key inside an array",
			layout: layoutWith(map[string]interface{}{
				"a": map[string]interface{}{
					"type":      "text",
					"placement": gridPlacement(0, 0, 1, 1),
					"config": map[string]interface{}{
						"items": []interface{}{"fine", map[string]interface{}{"privateData": 1}},
					},
				},
			}),
			wantErr: errReserved, names: "config.items.1.privateData",
		},

		{
			name:    "one widget over the count cap",
			layout:  layoutWith(manyWidgets(maxWidgets + 1)),
			wantErr: errTooManyWidgets, names: "300",
		},
		{
			// The cap bounds what every client must deep-clone on every
			// subsequent delta, so it counts the whole document, sub-grids
			// included, not one level of it.
			name: "the count cap spans scenes and sub-grids",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 24, "rows": 16},
				"scenes": map[string]interface{}{
					"one": map[string]interface{}{"widgets": manyWidgets(maxWidgets - 1)},
					"two": map[string]interface{}{
						"widgets": map[string]interface{}{
							"panel": subGridAt(gridPlacement(0, 0, 4, 4), manyWidgets(2)),
						},
					},
				},
			},
			wantErr: errTooManyWidgets, names: "300",
		},
		{
			name:    "serialised layout over the byte cap",
			layout:  hugeLayout(),
			wantErr: errTooLarge, names: "exceeds the maximum",
		},

		{
			name:    "widget missing its type",
			layout:  layoutWith(map[string]interface{}{"a": map[string]interface{}{"placement": gridPlacement(0, 0, 1, 1)}}),
			wantErr: errMissingField, names: "scenes.board.widgets.a.type",
		},
		{
			name:    "widget with an empty type",
			layout:  layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": "", "placement": gridPlacement(0, 0, 1, 1)}}),
			wantErr: errMissingField, names: "scenes.board.widgets.a.type",
		},
		{
			name:    "widget missing its placement",
			layout:  layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": "text"}}),
			wantErr: errMissingField, names: "scenes.board.widgets.a.placement",
		},
		{
			name:    "placement missing its kind",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(map[string]interface{}{"col": 0, "row": 0, "w": 1, "h": 1})}),
			wantErr: errMissingField, names: "a.placement.kind",
		},
		{
			name:    "unknown placement kind",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(map[string]interface{}{"kind": "flow"})}),
			wantErr: errPlacementKind, names: "flow",
		},
		{
			name:    "widget is not an object",
			layout:  layoutWith(map[string]interface{}{"a": "text"}),
			wantErr: errInvalidType, names: "scenes.board.widgets.a",
		},
		{
			name:    "scene missing its widgets",
			layout:  map[string]interface{}{"grid": map[string]interface{}{"cols": 8, "rows": 8}, "scenes": map[string]interface{}{"board": map[string]interface{}{}}},
			wantErr: errMissingField, names: "scenes.board.widgets",
		},
		{
			name:    "empty widget id",
			layout:  layoutWith(map[string]interface{}{"": widgetAt(gridPlacement(0, 0, 1, 1))}),
			wantErr: errMissingField, names: "scenes.board.widgets",
		},
		{
			name:    "script that is not a string",
			layout:  layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": "text", "placement": gridPlacement(0, 0, 1, 1), "script": 7}}),
			wantErr: errInvalidType, names: "scenes.board.widgets.a.script",
		},

		{
			// A key the server accepted but the client's strict parse rejects
			// would blank the board for everyone, so the authority rejects it.
			name: "misspelled top-level key",
			layout: map[string]interface{}{
				"grid": map[string]interface{}{"cols": 8, "rows": 8}, "scenes": map[string]interface{}{},
				"scnes": map[string]interface{}{},
			},
			wantErr: errUnknownField, names: "scnes",
		},
		{
			name:    "unknown key on a widget",
			layout:  layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": "text", "placement": gridPlacement(0, 0, 1, 1), "colour": "red"}}),
			wantErr: errUnknownField, names: "scenes.board.widgets.a.colour",
		},
		{
			name:    "unknown key on a grid placement",
			layout:  layoutWith(map[string]interface{}{"a": widgetAt(withZ(gridPlacement(0, 0, 1, 1), 2))}),
			wantErr: errUnknownField, names: "a.placement.z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLayout(tt.layout)
			if err == nil {
				t.Fatalf("validateLayout: expected an error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateLayout: error %v, want one wrapping %v", err, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.names) {
				t.Fatalf("validateLayout: error %q must name %q", err, tt.names)
			}

			if !strings.HasPrefix(err.Error(), "validating layout: ") {
				t.Fatalf("validateLayout: error %q must carry the lowercase context", err)
			}
		})
	}
}

// hugeLayout is one widget whose opaque config pushes the serialised document
// past the byte cap. Widget count alone would not catch this: the cap exists
// because every client deep-clones the layout on every subsequent delta.
func hugeLayout() map[string]interface{} {
	return layoutWith(map[string]interface{}{
		"a": map[string]interface{}{
			"type":      "text",
			"placement": gridPlacement(0, 0, 1, 1),
			"config":    map[string]interface{}{"text": strings.Repeat("x", maxBytes)},
		},
	})
}

// TestValidateLayout_WireShape runs a layout through the JSON round trip it
// actually takes off the socket, where every number arrives as a float64. A
// validator that only accepted Go int literals would pass its own tests and
// reject every real message.
func TestValidateLayout_WireShape(t *testing.T) {
	encoded, err := json.Marshal(layoutWith(map[string]interface{}{
		"a": widgetAt(gridPlacement(0, 0, 4, 4)),
		"b": widgetAt(gridPlacement(4, 0, 4, 4)),
	}))
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshalling the fixture: %v", err)
	}

	if err := validateLayout(decoded); err != nil {
		t.Fatalf("validateLayout: a JSON round trip must validate, got %v", err)
	}

	// And the same shape still fails where it should, so the float64 path is
	// not quietly skipping the integer rule by treating every number as whole.
	encoded, err = json.Marshal(layoutWith(map[string]interface{}{
		"a": widgetAt(gridPlacement(0, 0, 2.5, 1)),
	}))
	if err != nil {
		t.Fatalf("marshalling the fractional fixture: %v", err)
	}

	var bad map[string]interface{}
	if err := json.Unmarshal(encoded, &bad); err != nil {
		t.Fatalf("unmarshalling the fractional fixture: %v", err)
	}

	if err := validateLayout(bad); !errors.Is(err, errInvalidType) {
		t.Fatalf("validateLayout: a fractional span from the wire must be rejected, got %v", err)
	}
}

// TestValidateLayout_NoPanicOnMalformedInput pins that nothing here panics on
// untrusted wire data. Every one of these is a returned error, not a crash.
func TestValidateLayout_NoPanicOnMalformedInput(t *testing.T) {
	cases := []map[string]interface{}{
		{"grid": nil, "scenes": nil},
		{"grid": []interface{}{1.0}, "scenes": map[string]interface{}{}},
		{"grid": map[string]interface{}{"cols": nil, "rows": nil}, "scenes": map[string]interface{}{}},
		{"grid": map[string]interface{}{"cols": true, "rows": 8}, "scenes": map[string]interface{}{}},
		{"grid": map[string]interface{}{"cols": 8, "rows": 8}, "scenes": map[string]interface{}{"board": nil}},
		{"grid": map[string]interface{}{"cols": 8, "rows": 8}, "scenes": map[string]interface{}{"board": map[string]interface{}{"widgets": []interface{}{}}}},
		layoutWith(map[string]interface{}{"a": nil}),
		layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": "text", "placement": nil}}),
		layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": 1.0, "placement": gridPlacement(0, 0, 1, 1)}}),
		layoutWith(map[string]interface{}{"a": map[string]interface{}{"type": subGridType, "placement": gridPlacement(0, 0, 1, 1)}}),
		layoutWith(map[string]interface{}{"a": map[string]interface{}{
			"type": subGridType, "placement": gridPlacement(0, 0, 1, 1), "config": "nope",
		}}),
		layoutWith(map[string]interface{}{"a": widgetAt(map[string]interface{}{"kind": 1.0})}),
		layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(math.NaN(), 0, 1, 1))}),
		layoutWith(map[string]interface{}{"a": widgetAt(gridPlacement(math.Inf(1), 0, 1, 1))}),
	}

	for i, layout := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if err := validateLayout(layout); err == nil {
				t.Fatalf("validateLayout: expected an error for %v", layout)
			}
		})
	}
}

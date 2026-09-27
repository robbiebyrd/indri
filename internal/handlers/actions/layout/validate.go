package layout

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	minDim     = 8
	maxDim     = 4096
	maxDepth   = 4
	maxWidgets = 300
	maxBytes   = 256 << 10
)

type rect struct{ col, row, w, h int }

// overlaps is the same AABB test as the client's collides(). Edge-touching is
// not a collision — strict < on all four comparisons, matching collision.ts exactly.
func overlaps(a, b rect) bool {
	return a.col < b.col+b.w && b.col < a.col+a.w &&
		a.row < b.row+b.h && b.row < a.row+a.h
}

// ValidateLayout checks a layout document for structural correctness after an op
// has been applied. It never validates widget config contents — the server is
// framework-level and does not know widget semantics.
func ValidateLayout(layout map[string]interface{}) error {
	raw, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("validating layout: marshalling: %w", err)
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("validating layout: serialized size %d exceeds limit of %d bytes", len(raw), maxBytes)
	}

	if err := checkPrivateData(layout); err != nil {
		return err
	}

	cols, rows, err := extractGrid(layout)
	if err != nil {
		return err
	}

	scenes, _ := layout["scenes"].(map[string]interface{})
	total := countWidgets(scenes)
	if total > maxWidgets {
		return fmt.Errorf("validating layout: %d widgets exceeds limit of %d", total, maxWidgets)
	}

	for sceneID, sv := range scenes {
		scene, _ := sv.(map[string]interface{})
		widgets, _ := scene["widgets"].(map[string]interface{})
		if err := validateSceneWidgets(sceneID, widgets, cols, rows, 1); err != nil {
			return err
		}
	}

	return nil
}

func extractGrid(layout map[string]interface{}) (cols, rows int, err error) {
	grid, _ := layout["grid"].(map[string]interface{})
	cv, _ := grid["cols"].(float64)
	rv, _ := grid["rows"].(float64)
	if math.Trunc(cv) != cv || cv < minDim || cv > maxDim {
		return 0, 0, fmt.Errorf("validating layout: grid cols %v must be an integer in [%d, %d]", cv, minDim, maxDim)
	}
	if math.Trunc(rv) != rv || rv < minDim || rv > maxDim {
		return 0, 0, fmt.Errorf("validating layout: grid rows %v must be an integer in [%d, %d]", rv, minDim, maxDim)
	}
	return int(cv), int(rv), nil
}

func validateSceneWidgets(sceneID string, widgets map[string]interface{}, cols, rows, depth int) error {
	var gridPlaced []struct {
		id string
		r  rect
	}

	for widgetID, wv := range widgets {
		widget, _ := wv.(map[string]interface{})
		placement, _ := widget["placement"].(map[string]interface{})
		kind, _ := placement["kind"].(string)

		switch kind {
		case "grid":
			r, err := extractGridRect(placement)
			if err != nil {
				return fmt.Errorf("validating layout: scene %q widget %q: %w", sceneID, widgetID, err)
			}
			if r.col+r.w > cols || r.row+r.h > rows {
				return fmt.Errorf("validating layout: scene %q widget %q placement (%d,%d %dx%d) exceeds %dx%d grid",
					sceneID, widgetID, r.col, r.row, r.w, r.h, cols, rows)
			}
			gridPlaced = append(gridPlaced, struct {
				id string
				r  rect
			}{widgetID, r})

		case "absolute":
			if err := checkAbsolutePlacement(sceneID, widgetID, placement); err != nil {
				return err
			}
		}

		// Recurse into subgrid config.widgets
		widgetType, _ := widget["type"].(string)
		if widgetType == "subgrid" {
			if depth > maxDepth {
				return fmt.Errorf("validating layout: scene %q widget %q subgrid nesting depth %d exceeds limit of %d",
					sceneID, widgetID, depth, maxDepth)
			}
			config, _ := widget["config"].(map[string]interface{})
			nested, _ := config["widgets"].(map[string]interface{})
			if err := validateSceneWidgets(sceneID+"/"+widgetID, nested, cols, rows, depth+1); err != nil {
				return err
			}
		}
	}

	// Pairwise overlap check over grid-placed widgets.
	for i := 0; i < len(gridPlaced); i++ {
		for j := i + 1; j < len(gridPlaced); j++ {
			if overlaps(gridPlaced[i].r, gridPlaced[j].r) {
				return fmt.Errorf("validating layout: scene %q widgets %q and %q overlap",
					sceneID, gridPlaced[i].id, gridPlaced[j].id)
			}
		}
	}

	return nil
}

func extractGridRect(placement map[string]interface{}) (rect, error) {
	col, colOK := asInt(placement["col"])
	row, rowOK := asInt(placement["row"])
	w, wOK := asInt(placement["w"])
	h, hOK := asInt(placement["h"])

	if !colOK {
		return rect{}, fmt.Errorf("placement col must be a non-negative integer")
	}
	if !rowOK {
		return rect{}, fmt.Errorf("placement row must be a non-negative integer")
	}
	if !wOK || w < 1 {
		return rect{}, fmt.Errorf("placement w must be a positive integer")
	}
	if !hOK || h < 1 {
		return rect{}, fmt.Errorf("placement h must be a positive integer")
	}
	return rect{col, row, w, h}, nil
}

// asInt converts a JSON number (float64) to int, rejecting fractional or negative values.
func asInt(v interface{}) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	if math.Trunc(f) != f || f < 0 {
		return 0, false
	}
	return int(f), true
}

func checkAbsolutePlacement(sceneID, widgetID string, placement map[string]interface{}) error {
	for k, v := range placement {
		if k == "kind" {
			continue
		}
		if _, isNum := v.(float64); isNum {
			return fmt.Errorf("validating layout: scene %q widget %q absolute placement key %q must be a percentage string, not a pixel number",
				sceneID, widgetID, k)
		}
	}
	return nil
}

func checkPrivateData(v interface{}) error {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			if k == "privateData" {
				return fmt.Errorf("validating layout: \"privateData\" is a reserved key that the server strips during sanitization")
			}
			if err := checkPrivateData(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, elem := range val {
			if err := checkPrivateData(elem); err != nil {
				return err
			}
		}
	}
	return nil
}

// countWidgets counts all widget objects in the layout, including those nested
// inside subgrid config.widgets at any depth, so the cap applies to the full
// processing budget of validateSceneWidgets (O(n²) overlap check).
func countWidgets(scenes map[string]interface{}) int {
	total := 0
	for _, sv := range scenes {
		scene, _ := sv.(map[string]interface{})
		widgets, _ := scene["widgets"].(map[string]interface{})
		total += countWidgetMap(widgets)
	}
	return total
}

func countWidgetMap(widgets map[string]interface{}) int {
	count := len(widgets)
	for _, wv := range widgets {
		widget, _ := wv.(map[string]interface{})
		widgetType, _ := widget["type"].(string)
		if widgetType == "subgrid" {
			config, _ := widget["config"].(map[string]interface{})
			nested, _ := config["widgets"].(map[string]interface{})
			count += countWidgetMap(nested)
		}
	}
	return count
}

package layout

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
)

// THIS FILE IS ONE HALF OF A DELIBERATELY DUPLICATED PAIR.
//
// The other half is the TypeScript validator under `client/layout/`. They
// cannot share code across languages, so the rules are written twice on
// purpose and the constants and the AABB comparison below are kept textually
// identical to their TypeScript originals. Every rule names the file it
// mirrors. Change one side and you MUST change the other.
//
// The two halves are not interchangeable, though. The client's `parseLayout`
// is UX feedback: it clamps and warns so a sloppy board still renders. This
// one is the security boundary: the client is not, so a rule that only the
// client enforces is not enforced at all. It rejects rather than repairs,
// because a server that silently rewrites an author's board is worse than one
// that tells them what is wrong.
//
// What is NOT validated here is as deliberate as what is. Widget `config`
// contents are opaque (ADR in plans/layout-authoring-editor.md): Indri is a
// framework and does not know what a "text" widget is, so encoding widget
// semantics here would duplicate the client registry and go stale. `style` is
// opaque for the same reason — its vocabulary is React-Native-shaped and lives
// in client/layout/schema/style.ts. Placement, bounds, overlap, depth,
// reserved keys and size are structural, and those are enforced.

// Structural limits.
//
//   - minDim, maxDim mirror MIN_DIM and MAX_DIM in client/layout/grid/coords.ts.
//   - maxDepth mirrors MAX_SUBGRID_DEPTH in client/layout/schema/widget.ts.
//   - maxWidgets and maxBytes are server-side caps with no client counterpart:
//     without them one host can push a payload that every client must
//     deep-clone on every subsequent delta.
const (
	minDim, maxDim = 8, 4096
	maxDepth       = 4
	maxWidgets     = 300
	maxBytes       = 256 << 10
)

// reservedKey may not appear anywhere in a layout at any depth: the server's
// events.SanitizeDelta strips any path containing that segment, so accepting it
// would store data that can never be read back. Mirrors RESERVED_KEY in
// client/layout/schema/layout.ts.
const reservedKey = "privateData"

// subGridType is the widget type that carries a nested coordinate space.
// Mirrors SUBGRID_TYPE in client/layout/schema/widget.ts.
const subGridType = "subgrid"

// Placement discriminator values. Mirrors the Placement union in
// client/layout/schema/placement.ts.
const (
	kindGrid     = "grid"
	kindAbsolute = "absolute"
)

// Wire field names inside a layout, as they arrive in game.data.layout.
const (
	fieldScenes  = "scenes"
	fieldWidgets = "widgets"
	fieldScript  = "script"
	fieldType    = "type"
	fieldKind    = "kind"
	fieldOverlap = "overlap"
	fieldZ       = "z"
	fieldCols    = "cols"
	fieldRows    = "rows"
)

// Validation failures. Every one is wrapped by validateLayout with the same
// lowercase context, and every one names the dotted path of the offending
// value, e.g. `scenes.board.widgets.title.placement`.
var (
	errReserved       = errors.New("reserved key")
	errDimension      = errors.New("invalid grid dimension")
	errRect           = errors.New("invalid rect")
	errBounds         = errors.New("rect outside grid bounds")
	errOverlap        = errors.New("overlapping widgets")
	errDepth          = errors.New("sub-grid nesting too deep")
	errPlacementKind  = errors.New("unknown placement kind")
	errPercent        = errors.New("invalid percentage")
	errTooManyWidgets = errors.New("too many widgets")
	errTooLarge       = errors.New("layout too large")
)

// percentPattern mirrors the Percent brand in client/layout/schema/placement.ts.
// Absolute placement is percentages and nothing else: a bare number is the
// wrong FORMAT rather than a sloppy value, and there is nothing sensible to
// recover from it, so it is rejected outright.
var percentPattern = regexp.MustCompile(`^-?\d+(\.\d+)?%$`)

// gridSize is one coordinate space. Mirrors GridSize in
// client/layout/grid/coords.ts.
type gridSize struct{ cols, rows int }

// rect is a widget's cell rectangle in its own level's coordinate space.
// Mirrors GridRect in client/layout/grid/coords.ts. Only in-bounds rects are
// ever built, so every field is between 0 and maxDim and the arithmetic in
// overlaps cannot overflow.
type rect struct{ col, row, w, h int }

// placed pairs a rect with the id that owns it, so an overlap names both
// sides. Mirrors PlacedWidget in client/layout/grid/collision.ts.
type placed struct {
	id   string
	rect rect
}

// overlaps is the same four-comparison axis-aligned test as collides() in
// client/layout/grid/collision.ts. Strict "<" on every edge, so edge-touching
// is NOT a collision: a rect ending at col 3 sits flush against one starting
// at col 3.
func overlaps(a, b rect) bool {
	return a.col < b.col+b.w && b.col < a.col+a.w &&
		a.row < b.row+b.h && b.row < a.row+a.h
}

// validateLayout walks the WHOLE layout AFTER the op has been applied in
// memory, so an op can never leave the document in a state the renderer must
// defend against. Validating the op in isolation could not work: only the
// resulting document tells you whether an overlap now exists.
//
// The input is untrusted wire data shaped as map[string]interface{}, so every
// failure is a returned error and nothing here panics.
func validateLayout(layout map[string]interface{}) error {
	if layout == nil {
		return fmt.Errorf("validating layout: %w %q", errMissingField, "layout")
	}

	if err := checkSize(layout); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	// The reserved-key scan runs over the raw tree before any structural
	// check, because the key is forbidden in the opaque parts of the document
	// — a widget's config and style — as much as in the parts with a shape.
	if path := findReservedKey(layout, ""); path != "" {
		return fmt.Errorf("validating layout: %w %q: the server strips any path containing"+
			" a %q segment, so the value could never be read back", errReserved, path, reservedKey)
	}

	if err := checkKeys("", layout, fieldGrid, fieldStyle, fieldScript, fieldScenes); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	if err := optionalString(layout, fieldScript, fieldScript); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	grid, err := readGridSize(layout, fieldGrid, fieldGrid)
	if err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	scenes, err := readObject(layout, fieldScenes, fieldScenes)
	if err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	// Widgets are counted across the whole layout, nested sub-grids included:
	// the cap exists to bound what every client must clone, and a client
	// clones the document rather than one scene of it.
	count := 0

	for _, id := range sortedKeys(scenes) {
		path := join(fieldScenes, id)

		scene, err := readObject(scenes, id, path)
		if err != nil {
			return fmt.Errorf("validating layout: %w", err)
		}

		if err := validateScene(scene, grid, path, &count); err != nil {
			return fmt.Errorf("validating layout: %w", err)
		}
	}

	return nil
}

// checkSize caps the serialised layout. Marshalling also rejects anything that
// is not representable as JSON, which is the same thing as saying no client
// could ever receive it.
func checkSize(layout map[string]interface{}) error {
	encoded, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("%w: layout is not serialisable: %w", errInvalidType, err)
	}

	if len(encoded) > maxBytes {
		return fmt.Errorf("%w: %d bytes exceeds the maximum of %d", errTooLarge, len(encoded), maxBytes)
	}

	return nil
}

// validateScene checks one scene against the board's grid. Mirrors
// SceneLayoutSchema in client/layout/schema/layout.ts.
func validateScene(scene map[string]interface{}, grid gridSize, path string, count *int) error {
	if err := checkKeys(path, scene, fieldStyle, fieldScript, fieldWidgets); err != nil {
		return err
	}

	if err := optionalString(scene, fieldScript, join(path, fieldScript)); err != nil {
		return err
	}

	widgets, err := readObject(scene, fieldWidgets, join(path, fieldWidgets))
	if err != nil {
		return err
	}

	return validateWidgets(widgets, grid, join(path, fieldWidgets), 1, count)
}

// validateWidgets checks one grid level, then recurses into its sub-grids.
//
// Collision is scoped per level: a sub-grid child collides only with its own
// siblings, inside its parent's coordinate space. That is why the overlap set
// is rebuilt for every call and never carried across the recursion — see the
// module comment in client/layout/grid/collision.ts.
func validateWidgets(
	widgets map[string]interface{},
	grid gridSize,
	path string,
	depth int,
	count *int,
) error {
	var siblings []placed

	for _, id := range sortedKeys(widgets) {
		if id == "" {
			return fmt.Errorf("%w: widget id in %q", errMissingField, path)
		}

		*count++
		if *count > maxWidgets {
			return fmt.Errorf("%w: more than %d widgets", errTooManyWidgets, maxWidgets)
		}

		widgetPath := join(path, id)

		widget, err := readObject(widgets, id, widgetPath)
		if err != nil {
			return err
		}

		kind, r, err := validateWidget(widget, grid, widgetPath)
		if err != nil {
			return err
		}

		// A widget that allows overlap never enters the set, as subject OR
		// obstacle — absolute placement always, a grid widget when it opts in.
		// That exemption belongs to the caller rather than to overlaps() — see
		// client/layout/grid/collision.ts, which explains why a check inside
		// the collision test would be a second, divergent source of truth.
		exempt := false
		if placement, ok := widget[fieldPlacement].(map[string]interface{}); ok {
			exempt = allowsOverlap(placement)
		}

		if kind == kindGrid && !exempt {
			siblings = append(siblings, placed{id: id, rect: r})
		}

		if err := validateSubGrid(widget, widgetPath, depth, count); err != nil {
			return err
		}
	}

	return checkOverlaps(siblings, path)
}

// validateWidget checks one widget and returns its placement kind, plus its
// rect when that kind is "grid". Mirrors WidgetSchema in
// client/layout/schema/widget.ts — `config` and `style` stay opaque.
func validateWidget(widget map[string]interface{}, grid gridSize, path string) (string, rect, error) {
	if err := checkKeys(path, widget, fieldType, fieldPlacement, fieldStyle, fieldConfig, fieldScript); err != nil {
		return "", rect{}, err
	}

	typePath := join(path, fieldType)

	raw, ok := widget[fieldType]
	if !ok {
		return "", rect{}, fmt.Errorf("%w %q", errMissingField, typePath)
	}

	if _, err := nonEmptyString(typePath, raw); err != nil {
		return "", rect{}, err
	}

	if err := optionalString(widget, fieldScript, join(path, fieldScript)); err != nil {
		return "", rect{}, err
	}

	placementPath := join(path, fieldPlacement)

	placement, err := readObject(widget, fieldPlacement, placementPath)
	if err != nil {
		return "", rect{}, err
	}

	kind, err := readString(placement, fieldKind, join(placementPath, fieldKind))
	if err != nil {
		return "", rect{}, err
	}

	switch kind {
	case kindGrid:
		r, err := readRect(placement, grid, placementPath)

		return kindGrid, r, err
	case kindAbsolute:
		return kindAbsolute, rect{}, checkAbsolute(placement, placementPath)
	default:
		return "", rect{}, fmt.Errorf("%w %q in %q", errPlacementKind, kind, placementPath)
	}
}

// allowsOverlap reports whether a placement is exempt from collision, as both
// subject and obstacle. Mirrors allowsOverlap in
// client/layout/schema/placement.ts: absolute placement is exempt by
// definition, a grid widget only when it says so.
func allowsOverlap(placement map[string]interface{}) bool {
	if kind, _ := placement[fieldKind].(string); kind == kindAbsolute {
		return true
	}

	exempt, _ := placement[fieldOverlap].(bool)

	return exempt
}

// validateSubGrid recurses into a sub-grid widget's nested coordinate space.
// Anything that is not a sub-grid has an opaque config and is left alone.
func validateSubGrid(widget map[string]interface{}, path string, depth int, count *int) error {
	if widget[fieldType] != subGridType {
		return nil
	}

	configPath := join(path, fieldConfig)

	// A sub-grid renders a grid of widgets, any of which may itself be a
	// sub-grid, so an unbounded payload is a stack-overflow surface.
	if depth >= maxDepth {
		return fmt.Errorf("%w: %q exceeds the maximum depth of %d", errDepth, configPath, maxDepth)
	}

	config, err := readObject(widget, fieldConfig, configPath)
	if err != nil {
		return err
	}

	// SubGridConfigSchema in client/layout/schema/widget.ts: a nested grid and
	// its widgets, and nothing else. This is the one config the server does
	// read, because nesting depth and per-level collision cannot be checked
	// without it.
	if err := checkKeys(configPath, config, fieldGrid, fieldWidgets); err != nil {
		return err
	}

	nested, err := readGridSize(config, fieldGrid, join(configPath, fieldGrid))
	if err != nil {
		return err
	}

	widgetsPath := join(configPath, fieldWidgets)

	children, err := readObject(config, fieldWidgets, widgetsPath)
	if err != nil {
		return err
	}

	return validateWidgets(children, nested, widgetsPath, depth+1, count)
}

// checkOverlaps compares every grid-placed pair at one level. O(n²), bounded by
// maxWidgets and run once per op inside the mutation's critical section.
func checkOverlaps(siblings []placed, path string) error {
	for i := range siblings {
		for j := i + 1; j < len(siblings); j++ {
			if overlaps(siblings[i].rect, siblings[j].rect) {
				return fmt.Errorf("%w: %q overlaps %q", errOverlap,
					join(path, siblings[i].id), join(path, siblings[j].id))
			}
		}
	}

	return nil
}

// readRect reads a grid placement. Mirrors isValidRect and isWithinBounds in
// client/layout/grid/coords.ts, but rejects instead of clamping: the client
// clamps because it has already decided to keep the widget, while the server
// is deciding whether to store it at all.
func readRect(placement map[string]interface{}, grid gridSize, path string) (rect, error) {
	if err := checkKeys(path, placement, fieldKind, "col", "row", "w", "h", fieldOverlap, fieldZ); err != nil {
		return rect{}, err
	}

	col, err := readInt(placement, "col", join(path, "col"))
	if err != nil {
		return rect{}, err
	}

	row, err := readInt(placement, "row", join(path, "row"))
	if err != nil {
		return rect{}, err
	}

	w, err := readInt(placement, "w", join(path, "w"))
	if err != nil {
		return rect{}, err
	}

	h, err := readInt(placement, "h", join(path, "h"))
	if err != nil {
		return rect{}, err
	}

	// A zero or negative span is not a shape at all: a zero-area widget is
	// invisible but still eats input.
	if col < 0 || row < 0 || w <= 0 || h <= 0 {
		return rect{}, fmt.Errorf("%w %q: (col %d, row %d, %dx%d) has a negative origin or a non-positive span",
			errRect, path, col, row, w, h)
	}

	if col+w > int64(grid.cols) || row+h > int64(grid.rows) {
		return rect{}, fmt.Errorf("%w %q: (col %d, row %d, %dx%d) does not fit a %dx%d grid",
			errBounds, path, col, row, w, h, grid.cols, grid.rows)
	}

	// Safe: the bounds check above proves every component is within the grid,
	// which is itself capped at maxDim.
	return rect{col: int(col), row: int(row), w: int(w), h: int(h)}, nil
}

// checkAbsolute reads an absolute placement. Every offset is a percentage
// string; the optional z is a whole number. Mirrors the absolute branch of
// Placement in client/layout/schema/placement.ts.
func checkAbsolute(placement map[string]interface{}, path string) error {
	if err := checkKeys(path, placement, fieldKind, "left", "top", "width", "height", fieldOverlap, fieldZ); err != nil {
		return err
	}

	for _, name := range []string{"left", "top", "width", "height"} {
		fieldPath := join(path, name)

		value, err := readString(placement, name, fieldPath)
		if err != nil {
			return err
		}

		if !percentPattern.MatchString(value) {
			return fmt.Errorf("%w %q: %q must be a percentage string such as \"25%%\";"+
				" pixel values are not allowed", errPercent, fieldPath, value)
		}
	}

	if _, ok := placement["z"]; ok {
		if _, err := readInt(placement, "z", join(path, "z")); err != nil {
			return err
		}
	}

	return nil
}

// readGridSize reads a coordinate space. Mirrors GridSizeSchema in
// client/layout/schema/placement.ts and isDimension in
// client/layout/grid/coords.ts.
func readGridSize(parent map[string]interface{}, key, path string) (gridSize, error) {
	raw, err := readObject(parent, key, path)
	if err != nil {
		return gridSize{}, err
	}

	if err := checkKeys(path, raw, fieldCols, fieldRows); err != nil {
		return gridSize{}, err
	}

	cols, err := readDimension(raw, fieldCols, join(path, fieldCols))
	if err != nil {
		return gridSize{}, err
	}

	rows, err := readDimension(raw, fieldRows, join(path, fieldRows))
	if err != nil {
		return gridSize{}, err
	}

	return gridSize{cols: cols, rows: rows}, nil
}

func readDimension(parent map[string]interface{}, key, path string) (int, error) {
	n, err := readInt(parent, key, path)
	if err != nil {
		return 0, err
	}

	if n < minDim || n > maxDim {
		return 0, fmt.Errorf("%w %q: %d is outside %d..%d", errDimension, path, n, minDim, maxDim)
	}

	return int(n), nil
}

// readObject reads a required nested object, reporting an absent key as
// missing rather than as a type error so a client can tell the two apart.
func readObject(parent map[string]interface{}, key, path string) (map[string]interface{}, error) {
	raw, ok := parent[key]
	if !ok {
		return nil, fmt.Errorf("%w %q", errMissingField, path)
	}

	return object(path, raw)
}

func readString(parent map[string]interface{}, key, path string) (string, error) {
	raw, ok := parent[key]
	if !ok {
		return "", fmt.Errorf("%w %q", errMissingField, path)
	}

	return str(path, raw)
}

// optionalString type-checks a field that may be absent. Used for the `script`
// fields, whose content is opaque — only their shape is the server's business.
func optionalString(parent map[string]interface{}, key, path string) error {
	raw, ok := parent[key]
	if !ok {
		return nil
	}

	_, err := str(path, raw)

	return err
}

// readInt reads a whole number. Mirrors Number.isInteger in
// client/layout/grid/coords.ts: a fractional coordinate would make the
// percentage boxes disagree with what an editor snaps to.
//
// It returns int64 so that no caller has to reason about overflow before its
// own range check has run.
func readInt(parent map[string]interface{}, key, path string) (int64, error) {
	raw, ok := parent[key]
	if !ok {
		return 0, fmt.Errorf("%w %q", errMissingField, path)
	}

	f, ok := number(raw)
	if !ok {
		return 0, fmt.Errorf("%w: %q must be a number", errInvalidType, path)
	}

	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, fmt.Errorf("%w: %q must be a whole number, got %v", errInvalidType, path, f)
	}

	// Outside this range the float cannot round-trip through an integer at
	// all, so there is no value to report and nothing a client could have
	// meant by it.
	if f < math.MinInt32 || f > math.MaxInt32 {
		return 0, fmt.Errorf("%w: %q is out of range, got %v", errInvalidType, path, f)
	}

	return int64(f), nil
}

// number accepts every numeric shape a layout can arrive in: JSON decoding
// produces float64, a BSON round-trip produces int32 or int64, and a Go
// fixture produces int. A bool is deliberately not a number.
func number(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()

		return f, err == nil
	default:
		return 0, false
	}
}

// checkKeys rejects any key the shape does not declare, mirroring the
// `.strict()` on every schema in client/layout/schema/. A misspelled key the
// server accepted would be rejected by the client's strict parse, blanking the
// board for everyone — so the authority rejects it first.
func checkKeys(path string, value map[string]interface{}, allowed ...string) error {
	for _, key := range sortedKeys(value) {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("%w %q", errUnknownField, join(path, key))
		}
	}

	return nil
}

// findReservedKey is a depth-first scan returning the dotted path of the
// reserved key, or "" when it is absent. Mirrors findReservedKey in
// client/layout/schema/layout.ts.
func findReservedKey(value interface{}, path string) string {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range sortedKeys(v) {
			childPath := join(path, key)
			if key == reservedKey {
				return childPath
			}

			if hit := findReservedKey(v[key], childPath); hit != "" {
				return hit
			}
		}
	case []interface{}:
		for i, item := range v {
			if hit := findReservedKey(item, join(path, fmt.Sprint(i))); hit != "" {
				return hit
			}
		}
	}

	return ""
}

// sortedKeys makes every walk deterministic, so the same bad layout always
// names the same offending path rather than whichever one map iteration
// happened to reach first.
func sortedKeys(m map[string]interface{}) []string {
	return slices.Sorted(maps.Keys(m))
}

func join(path, key string) string {
	if path == "" {
		return key
	}

	return path + "." + key
}

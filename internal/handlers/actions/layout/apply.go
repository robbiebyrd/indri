package layout

import (
	"errors"
	"fmt"
	"maps"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// layoutKey is the one key in a game's public data this action may write.
const layoutKey = "layout"

// Addressing failures. They name a dotted path inside the layout, the same way
// the validator does, so a client gets one vocabulary for "where" regardless of
// which half rejected it.
var (
	errNotFound = errors.New("does not exist")
	errExists   = errors.New("already exists")
)

// applyLayoutOp applies one decoded op to a game's layout, in memory, inside
// Mutate.
//
// The write is confined to g.PublicData["layout"]. The op is applied to a
// detached copy and only a copy that passes validateLayout is assigned back, so
// a rejected op leaves the game exactly as it found it. Nothing else in the
// document is reachable from here: not PrivateData, not Players, not Teams, not
// Stage.
func applyLayoutOp(g *models.Game, op *Op) error {
	// Two independent copies: one to edit, one to compare against. Normalising
	// is what makes the comparison mean anything — a layout read back from
	// MongoDB arrives as bson.D, which would never compare equal to a map.
	current := normalizeLayout(g.PublicData[layoutKey])

	next := normalizeLayout(g.PublicData[layoutKey])
	if next == nil {
		next = map[string]interface{}{}
	}

	// validateLayout requires "scenes", so the first op on a game with no
	// layout — setGrid, normally — must seed it rather than write a document
	// the validator would then reject.
	if _, ok := next[fieldScenes]; !ok {
		next[fieldScenes] = map[string]interface{}{}
	}

	if err := applyOp(next, op); err != nil {
		return fmt.Errorf("applying layout op %q: %w", op.Op, err)
	}

	// A genuine no-op: skip the write so no version bump and no empty delta
	// occur. This is what makes removeWidget idempotent for a client retrying
	// after a reconnect.
	if reflect.DeepEqual(current, next) {
		return mutation.ErrAbort
	}

	if err := validateLayout(next); err != nil {
		return fmt.Errorf("applying layout op %q: %w", op.Op, err)
	}

	if g.PublicData == nil {
		g.PublicData = map[string]interface{}{}
	}

	g.PublicData[layoutKey] = next

	return nil
}

func applyOp(layout map[string]interface{}, op *Op) error {
	switch op.Op {
	case OpAddWidget:
		return addWidget(layout, op)
	case OpRemoveWidget:
		return removeWidget(layout, op)
	case OpSetPlacement:
		return setPlacement(layout, op)
	case OpSetWidgetConfig:
		return setWidgetConfig(layout, op)
	case OpSetStyle:
		return setStyle(layout, op)
	case OpSetGrid:
		layout[fieldGrid] = op.Grid

		return nil
	case OpSetScript:
		return setScript(layout, op)
	default:
		// Unreachable: decodeOp rejects any op that is not in opSpecs.
		return fmt.Errorf("%w %q", errUnknownOp, op.Op)
	}
}

// addWidget inserts a widget, creating the scene if it is not there yet.
//
// Creating the scene is deliberate: the op vocabulary has no addScene, so a
// scene comes into existence with its first widget. An existing widget id is an
// error rather than a replace — "add" that silently clobbers is how an editor
// loses a widget nobody meant to touch; remove it first, or use setPlacement,
// setWidgetConfig and setStyle to edit it in place.
func addWidget(layout map[string]interface{}, op *Op) error {
	widgets, err := widgetsOf(layout, op, true)
	if err != nil {
		return err
	}

	if _, ok := widgets[op.WidgetID]; ok {
		return fmt.Errorf("%q %w", widgetPath(op), errExists)
	}

	widgets[op.WidgetID] = op.Widget

	return nil
}

// removeWidget deletes a widget if it is there. A missing scene or a missing
// widget is not an error: the op is idempotent, and because nothing changed
// applyLayoutOp then aborts the mutation rather than writing.
func removeWidget(layout map[string]interface{}, op *Op) error {
	widgets, err := widgetsOf(layout, op, false)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}

		return err
	}

	delete(widgets, op.WidgetID)

	return nil
}

// setPlacement replaces the whole placement, because a move and a resize write
// the same field and a partial placement is not a placement.
func setPlacement(layout map[string]interface{}, op *Op) error {
	widget, err := widgetOf(layout, op)
	if err != nil {
		return err
	}

	widget[fieldPlacement] = op.Placement

	return nil
}

// setWidgetConfig merges into the widget's config rather than replacing it, so
// an editor panel can send one field at a time. Removing a config key is not
// expressible; remove and re-add the widget.
func setWidgetConfig(layout map[string]interface{}, op *Op) error {
	widget, err := widgetOf(layout, op)
	if err != nil {
		return err
	}

	// An empty merge changes nothing, so do not conjure an empty config object
	// and turn a no-op into a write.
	if len(op.Config) == 0 {
		return nil
	}

	config, err := childObject(widget, fieldConfig, join(widgetPath(op), fieldConfig), true)
	if err != nil {
		return err
	}

	maps.Copy(config, op.Config)

	return nil
}

func setStyle(layout map[string]interface{}, op *Op) error {
	target, err := scopeTarget(layout, op)
	if err != nil {
		return err
	}

	target[fieldStyle] = op.Style

	return nil
}

// setScript writes the script at the op's scope. An empty source clears it: the
// key is deleted rather than set to "", so the document carries no dead field
// and the delta is a removal the client can act on.
func setScript(layout map[string]interface{}, op *Op) error {
	target, err := scopeTarget(layout, op)
	if err != nil {
		return err
	}

	if *op.Source == "" {
		delete(target, fieldScript)

		return nil
	}

	target[fieldScript] = *op.Source

	return nil
}

// scopeTarget resolves the board, a scene or a widget. decodeOp has already
// checked that the scope carries exactly the ids it needs, so the addressing
// below can only fail because the target is absent.
func scopeTarget(layout map[string]interface{}, op *Op) (map[string]interface{}, error) {
	switch op.Scope {
	case ScopeBoard:
		return layout, nil
	case ScopeScene:
		return sceneOf(layout, op, false)
	case ScopeWidget:
		return widgetOf(layout, op)
	default:
		// Unreachable: decodeOp rejects any scope outside the three above.
		return nil, fmt.Errorf("%w %q", errScope, op.Scope)
	}
}

func sceneOf(layout map[string]interface{}, op *Op, create bool) (map[string]interface{}, error) {
	scenes, err := childObject(layout, fieldScenes, fieldScenes, create)
	if err != nil {
		return nil, err
	}

	return childObject(scenes, op.SceneID, scenePath(op), create)
}

func widgetsOf(layout map[string]interface{}, op *Op, create bool) (map[string]interface{}, error) {
	scene, err := sceneOf(layout, op, create)
	if err != nil {
		return nil, err
	}

	return childObject(scene, fieldWidgets, join(scenePath(op), fieldWidgets), create)
}

func widgetOf(layout map[string]interface{}, op *Op) (map[string]interface{}, error) {
	widgets, err := widgetsOf(layout, op, false)
	if err != nil {
		return nil, err
	}

	return childObject(widgets, op.WidgetID, widgetPath(op), false)
}

func scenePath(op *Op) string {
	return join(fieldScenes, op.SceneID)
}

func widgetPath(op *Op) string {
	return join(join(scenePath(op), fieldWidgets), op.WidgetID)
}

// childObject reads a nested object, creating an empty one when create is set.
// A key that is already there but holds something other than an object is a
// type error rather than something to overwrite: the layout would be
// unparseable for every client, and the op almost certainly addressed the wrong
// thing.
func childObject(parent map[string]interface{}, key, path string, create bool) (map[string]interface{}, error) {
	raw, ok := parent[key]
	if !ok {
		if !create {
			return nil, fmt.Errorf("%q %w", path, errNotFound)
		}

		created := map[string]interface{}{}
		parent[key] = created

		return created, nil
	}

	return object(path, raw)
}

// normalizeLayout deep-copies a stored layout into plain Go maps and slices,
// returning nil when there is no layout (or when what is stored is not an
// object at all — validateLayout will reject the replacement anyway, and there
// is nothing in a non-object to preserve).
//
// It exists for two reasons. A layout read back from MongoDB arrives as bson.D,
// an ordered slice of key/value pairs that validateLayout cannot walk and no
// client could parse, so it has to be converted before it can be edited. And
// copying rather than editing in place is what lets applyLayoutOp validate the
// result before deciding whether the game is allowed to see it.
func normalizeLayout(value interface{}) map[string]interface{} {
	normalized, ok := normalizeValue(value).(map[string]interface{})
	if !ok {
		return nil
	}

	return normalized
}

func normalizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case bson.D:
		out := make(map[string]interface{}, len(v))
		for _, element := range v {
			out[element.Key] = normalizeValue(element.Value)
		}

		return out
	case bson.M:
		// Not produced by the current driver options, but it is what the
		// document decoder emits once DefaultDocumentM is set, and falling
		// through to the default would hand back the stored map uncopied.
		return normalizeValue(map[string]interface{}(v))
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeValue(item)
		}

		return out
	case bson.A:
		return normalizeValue([]interface{}(v))
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeValue(item)
		}

		return out
	default:
		return v
	}
}

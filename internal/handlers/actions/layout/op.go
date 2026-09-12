// Package layout decodes and applies host-authored edits to a game's layout.
package layout

import (
	"errors"
	"fmt"
	"slices"
)

// Op names. One action carries all of them behind an "op" discriminator, so the
// protocol surface and the Storer interface stay small and a single Mutate can
// produce granular per-field deltas.
const (
	OpAddWidget       = "addWidget"
	OpRemoveWidget    = "removeWidget"
	OpSetPlacement    = "setPlacement"
	OpSetWidgetConfig = "setWidgetConfig"
	OpSetStyle        = "setStyle"
	OpSetGrid         = "setGrid"
	OpSetScript       = "setScript"
)

// Scopes for the ops that can target the board, one scene, or one widget.
const (
	ScopeBoard  = "board"
	ScopeScene  = "scene"
	ScopeWidget = "widget"
)

// Wire field names, as they arrive in actions.Request.Payload.
const (
	fieldOp        = "op"
	fieldCode      = "code"
	fieldSceneID   = "sceneId"
	fieldWidgetID  = "widgetId"
	fieldScope     = "scope"
	fieldWidget    = "widget"
	fieldPlacement = "placement"
	fieldConfig    = "config"
	fieldStyle     = "style"
	fieldGrid      = "grid"
	fieldSource    = "source"
)

// Decoding failures. Every one is wrapped with the op name by decodeOp, and
// every one names the offending field so a client can fix the message.
var (
	errUnknownOp    = errors.New("unknown op")
	errMissingField = errors.New("missing required field")
	errUnknownField = errors.New("unknown field")
	errInvalidType  = errors.New("invalid type")
	errScope        = errors.New("invalid scope")
)

// Op is one decoded layout edit. Only the fields its op declares are populated;
// the rest stay zero. Decoding checks shape only — bounds, overlap, depth and
// reserved keys are validated against the resulting document, not the op.
type Op struct {
	Op        string                 `json:"op"`
	SceneID   string                 `json:"sceneId,omitempty"`
	WidgetID  string                 `json:"widgetId,omitempty"`
	Scope     string                 `json:"scope,omitempty"`
	Widget    map[string]interface{} `json:"widget,omitempty"`
	Placement map[string]interface{} `json:"placement,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Style     map[string]interface{} `json:"style,omitempty"`
	Grid      map[string]interface{} `json:"grid,omitempty"`
	// Source is a pointer because an empty script is meaningful and distinct
	// from an absent one.
	Source *string `json:"source,omitempty"`
}

// opSpec is the per-op field contract. Anything outside required+optional is an
// unknown field, so a typo cannot be silently ignored.
type opSpec struct {
	required []string
	optional []string
}

func (s opSpec) allows(name string) bool {
	return slices.Contains(s.required, name) || slices.Contains(s.optional, name)
}

// scoped reports whether the op's target is chosen by a "scope" field, in which
// case sceneId and widgetId are required or forbidden by that scope.
func (s opSpec) scoped() bool {
	return slices.Contains(s.required, fieldScope)
}

// opSpecs is the op vocabulary. setPlacement covers both move and resize: they
// write the same field, so a separate resizeWidget op would be two names for
// one write.
var opSpecs = map[string]opSpec{
	OpAddWidget:       {required: []string{fieldSceneID, fieldWidgetID, fieldWidget}},
	OpRemoveWidget:    {required: []string{fieldSceneID, fieldWidgetID}},
	OpSetPlacement:    {required: []string{fieldSceneID, fieldWidgetID, fieldPlacement}},
	OpSetWidgetConfig: {required: []string{fieldSceneID, fieldWidgetID, fieldConfig}},
	OpSetStyle:        {required: []string{fieldScope, fieldStyle}, optional: []string{fieldSceneID, fieldWidgetID}},
	OpSetGrid:         {required: []string{fieldGrid}},
	OpSetScript:       {required: []string{fieldScope, fieldSource}, optional: []string{fieldSceneID, fieldWidgetID}},
}

// decodeOp turns a raw action payload into an Op. The payload comes straight
// off the wire, so every field is checked for type as well as presence.
func decodeOp(msg map[string]interface{}) (*Op, error) {
	raw, ok := msg[fieldOp]
	if !ok {
		return nil, fmt.Errorf("decoding layout op: %w %q", errMissingField, fieldOp)
	}

	name, err := nonEmptyString(fieldOp, raw)
	if err != nil {
		return nil, fmt.Errorf("decoding layout op: %w", err)
	}

	spec, ok := opSpecs[name]
	if !ok {
		return nil, fmt.Errorf("decoding layout op %q: %w", name, errUnknownOp)
	}

	op := &Op{Op: name}
	if err := op.decodeFields(msg, spec); err != nil {
		return nil, fmt.Errorf("decoding layout op %q: %w", name, err)
	}

	return op, nil
}

func (o *Op) decodeFields(msg map[string]interface{}, spec opSpec) error {
	for key := range msg {
		// "op" is the discriminator and "code" addresses the game; both belong
		// to the message rather than to any one op.
		if key == fieldOp || key == fieldCode {
			continue
		}

		if !spec.allows(key) {
			return fmt.Errorf("%w %q", errUnknownField, key)
		}
	}

	for _, name := range spec.required {
		raw, ok := msg[name]
		if !ok {
			return fmt.Errorf("%w %q", errMissingField, name)
		}

		if err := o.set(name, raw); err != nil {
			return err
		}
	}

	for _, name := range spec.optional {
		raw, ok := msg[name]
		if !ok {
			continue
		}

		if err := o.set(name, raw); err != nil {
			return err
		}
	}

	if spec.scoped() {
		return o.checkScope()
	}

	return nil
}

func (o *Op) set(name string, raw interface{}) error {
	var err error

	switch name {
	case fieldSceneID:
		o.SceneID, err = nonEmptyString(name, raw)
	case fieldWidgetID:
		o.WidgetID, err = nonEmptyString(name, raw)
	case fieldScope:
		o.Scope, err = nonEmptyString(name, raw)
	case fieldWidget:
		o.Widget, err = object(name, raw)
	case fieldPlacement:
		o.Placement, err = object(name, raw)
	case fieldConfig:
		o.Config, err = object(name, raw)
	case fieldStyle:
		o.Style, err = object(name, raw)
	case fieldGrid:
		o.Grid, err = object(name, raw)
	case fieldSource:
		// An empty source is a real value: it clears the script.
		var source string
		if source, err = str(name, raw); err == nil {
			o.Source = &source
		}
	default:
		// Unreachable: decodeFields only sets fields the spec declares, and
		// every declared field has a case above.
		return fmt.Errorf("%w %q", errUnknownField, name)
	}

	return err
}

// checkScope enforces which of sceneId and widgetId a scope needs. A scope that
// carries an address it does not use is rejected rather than ignored, because
// ignoring it would silently edit a different target than the client meant.
func (o *Op) checkScope() error {
	switch o.Scope {
	case ScopeBoard:
		if o.SceneID != "" {
			return fmt.Errorf("%w %q: %q must not be set", errScope, o.Scope, fieldSceneID)
		}

		if o.WidgetID != "" {
			return fmt.Errorf("%w %q: %q must not be set", errScope, o.Scope, fieldWidgetID)
		}
	case ScopeScene:
		if o.SceneID == "" {
			return fmt.Errorf("%w %q", errMissingField, fieldSceneID)
		}

		if o.WidgetID != "" {
			return fmt.Errorf("%w %q: %q must not be set", errScope, o.Scope, fieldWidgetID)
		}
	case ScopeWidget:
		if o.SceneID == "" {
			return fmt.Errorf("%w %q", errMissingField, fieldSceneID)
		}

		if o.WidgetID == "" {
			return fmt.Errorf("%w %q", errMissingField, fieldWidgetID)
		}
	default:
		return fmt.Errorf("%w %q", errScope, o.Scope)
	}

	return nil
}

func str(name string, raw interface{}) (string, error) {
	v, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string", errInvalidType, name)
	}

	return v, nil
}

func nonEmptyString(name string, raw interface{}) (string, error) {
	v, err := str(name, raw)
	if err != nil {
		return "", err
	}

	if v == "" {
		return "", fmt.Errorf("%w %q", errMissingField, name)
	}

	return v, nil
}

func object(name string, raw interface{}) (map[string]interface{}, error) {
	v, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: %q must be an object", errInvalidType, name)
	}

	return v, nil
}

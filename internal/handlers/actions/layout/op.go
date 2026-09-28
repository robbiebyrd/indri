package layout

import (
	"encoding/json"
	"fmt"
)

// Op is the decoded form of a single layout mutation instruction.
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
	Source    *string                `json:"source,omitempty"`
}

// MissingFieldError names the required field that was absent in the wire message.
type MissingFieldError struct {
	Field  string
	OpName string
}

func (e *MissingFieldError) Error() string {
	return fmt.Sprintf("field %q required for op %q", e.Field, e.OpName)
}

// UnknownOpError names the op value that was not in the vocabulary.
type UnknownOpError struct {
	Op string
}

func (e *UnknownOpError) Error() string {
	return fmt.Sprintf("unknown layout op %q", e.Op)
}

// UnknownKeyError names a top-level key that is not part of the Op vocabulary.
type UnknownKeyError struct {
	Key string
}

func (e *UnknownKeyError) Error() string {
	return fmt.Sprintf("unknown layout op key %q", e.Key)
}

// allowedKeys is the complete set of top-level keys the Op struct recognises,
// plus "code" which is consumed by the handler before DecodeOp is called.
var allowedKeys = map[string]bool{
	"op":        true,
	"sceneId":   true,
	"widgetId":  true,
	"scope":     true,
	"widget":    true,
	"placement": true,
	"config":    true,
	"style":     true,
	"grid":      true,
	"source":    true,
	"code":      true,
}

// opRequired maps each op name to the fields that must be non-zero for that op.
var opRequired = map[string][]string{
	"addWidget":       {"sceneId", "widgetId", "widget"},
	"removeWidget":    {"sceneId", "widgetId"},
	"setPlacement":    {"sceneId", "widgetId", "placement"},
	"setWidgetConfig": {"sceneId", "widgetId", "config"},
	"setStyle":        {"scope", "style"},
	"setGrid":         {"grid"},
	"setScript":       {"scope", "source"},
}

// DecodeOp decodes a layout sub-operation from the already-decoded wire message.
//
// The caller (handler) has already stripped the WebSocket "action" key but
// leaves the "op" key and all other fields intact.
func DecodeOp(msg map[string]interface{}) (*Op, error) {
	// Reject unknown top-level keys before doing anything else so callers get
	// a clean error rather than silently dropped data.
	for k := range msg {
		if !allowedKeys[k] {
			return nil, fmt.Errorf("decoding layout op: %w", &UnknownKeyError{Key: k})
		}
	}

	opName, _ := msg["op"].(string)
	if opName == "" {
		return nil, fmt.Errorf("decoding layout op: op field is required and must be a non-empty string")
	}

	required, known := opRequired[opName]
	if !known {
		return nil, fmt.Errorf("decoding layout op %q: %w", opName, &UnknownOpError{Op: opName})
	}

	// Re-marshal and unmarshal into the struct so JSON tag handling is consistent
	// and we don't hand-decode every field.
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("decoding layout op %q: %w", opName, err)
	}

	var op Op
	if err = json.Unmarshal(raw, &op); err != nil {
		return nil, fmt.Errorf("decoding layout op %q: %w", opName, err)
	}

	// Validate required fields after unmarshalling so we can use the typed struct.
	for _, field := range required {
		if missing := isMissing(&op, field); missing {
			return nil, fmt.Errorf("decoding layout op %q: %w", opName, &MissingFieldError{Field: field, OpName: opName})
		}
	}

	return &op, nil
}

// isMissing reports whether the named field on op is zero/nil.
func isMissing(op *Op, field string) bool {
	switch field {
	case "sceneId":
		return op.SceneID == ""
	case "widgetId":
		return op.WidgetID == ""
	case "scope":
		return op.Scope == ""
	case "widget":
		return op.Widget == nil
	case "placement":
		return op.Placement == nil
	case "config":
		return op.Config == nil
	case "style":
		return op.Style == nil
	case "grid":
		return op.Grid == nil
	case "source":
		return op.Source == nil
	default:
		// Programming error: opRequired references a field name with no case here.
		// Add the corresponding case when adding a new required field.
		panic("isMissing: unhandled field name " + field)
	}
}

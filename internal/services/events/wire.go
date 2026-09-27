package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// LayoutFrame carries a game's layout: its rendering config (grid, widgets,
// Lua scripts), kept out of keyframes and deltas. It is sent before a
// connection's first keyframe and again whenever the layout is edited. V is
// the layout's version (see LayoutVersion).
type LayoutFrame struct {
	O    OpCode                 `json:"o"`
	V    string                 `json:"v"`
	Data map[string]interface{} `json:"data"`
}

// NewLayoutFrame is the layout frame for layout.
func NewLayoutFrame(layout map[string]interface{}) LayoutFrame {
	return LayoutFrame{O: OpLayout, V: LayoutVersion(layout), Data: layout}
}

// LayoutVersion identifies a layout by its content, so it changes exactly when
// the layout does. JSON encodes map keys in sorted order, so equal layouts
// always have the same version.
func LayoutVersion(layout map[string]interface{}) string {
	raw, _ := json.Marshal(layout)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:8])
}

// KeyframeWrapper carries a keyframe: the client view of the game (see
// ClientView) with a schema version. The "sv" field is the discriminator the
// client uses to identify this message type.
type KeyframeWrapper struct {
	SV   string                 `json:"sv"`
	Game map[string]interface{} `json:"game"`
}

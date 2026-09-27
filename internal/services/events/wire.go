package events

import "github.com/robbiebyrd/indri/internal/models"

// LayoutFrame is sent once per connection before the first keyframe. It carries
// the static rendering config (Lua scripts, widget definitions) that never
// changes during a game session.
type LayoutFrame struct {
	O    OpCode                 `json:"o"    msgpack:"o"`
	V    string                 `json:"v"    msgpack:"v"`
	Data map[string]interface{} `json:"data" msgpack:"data"`
}

// KeyframeWrapper wraps a slim game keyframe (data.layout stripped) with a
// schema version hash. The "sv" field is the discriminator the client uses to
// identify this message type.
type KeyframeWrapper struct {
	SV   string       `json:"sv"   msgpack:"sv"`
	Game *models.Game `json:"game" msgpack:"game"`
}

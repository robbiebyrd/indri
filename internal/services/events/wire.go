package events

// LayoutFrame is sent once per connection before the first keyframe. It carries
// the static rendering config (Lua scripts, widget definitions) that never
// changes during a game session.
type LayoutFrame struct {
	O    OpCode                 `json:"o"`
	V    string                 `json:"v"`
	Data map[string]interface{} `json:"data"`
}

// KeyframeWrapper carries a keyframe: the client view of the game (see
// ClientView) with a schema version. The "sv" field is the discriminator the
// client uses to identify this message type.
type KeyframeWrapper struct {
	SV   string                 `json:"sv"`
	Game map[string]interface{} `json:"game"`
}

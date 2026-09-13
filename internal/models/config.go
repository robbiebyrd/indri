package models

// ScriptSchemaVersion is the config.json format this build understands.
//
// It exists so that a later change to the format is detectable rather than
// silently misread: a config declaring a version this server does not know is
// refused at boot. Raise it whenever a change to Script would make an older
// server read a newer file wrongly.
const ScriptSchemaVersion = 1

type Script struct {
	// SchemaVersion is the config.json format the file was written for. Zero
	// means the file predates versioning and is read as ScriptSchemaVersion.
	SchemaVersion int `bson:"schemaVersion,omitempty" json:"schemaVersion,omitempty"`

	Config Config          `bson:"config,omitempty" json:"config,omitempty"`
	Teams  map[string]Team `bson:"teams"            json:"teams,omitempty"`
	Stage  Stage           `bson:"stage"            json:"stage,omitempty"`

	// Scripts lists the Lua game scripts to load, in load order.
	Scripts []ScriptFile `bson:"scripts,omitempty" json:"scripts,omitempty"`

	PublicData  map[string]interface{} `bson:"data"        json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData" json:"privateData,omitempty"`
}

// ScriptFile is one Lua game script and the host capabilities it is allowed to
// reach.
//
// Grants are per script rather than per server because a capability that is
// absent is unreachable: a script that was not granted one never sees it on its
// host table at all, so what a script can do is a glance at this list rather
// than an audit of every host function's body. An unrecognised grant name fails
// boot — see internal/services/lua/capability.go.
type ScriptFile struct {
	// Path is written relative to the config file that declares it, and the
	// script store rewrites it to that base as it loads: a server started from
	// another working directory must find the same files.
	Path string `bson:"path" json:"path"`

	Grants []string `bson:"grants,omitempty" json:"grants,omitempty"`
}

// Config sets configuration for the way teams are handled.
//
//	PVP: If enabled, there are no teams, and all players are competing against each other.
//	MaxTeams: The total number of allowed teams.
//	MaxPlayersPerTeam: The total number of players allowed on each team.
type Config struct {
	PVP               bool `bson:"pvp"               json:"pvp"`
	MaxTeams          int  `bson:"maxTeams"          json:"maxTeams"`
	MaxPlayersPerTeam int  `bson:"maxPlayersPerTeam" json:"maxPlayersPerTeam"`
	ProfanityFilter   bool `bson:"profanityFilter"   json:"profanityFilter"`
	CreateTeams       bool `bson:"createTeams"       json:"createTeams"`
}

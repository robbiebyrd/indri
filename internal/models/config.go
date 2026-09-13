package models

type Script struct {
	Config Config          `bson:"config,omitempty" json:"config,omitempty"`
	Teams  map[string]Team `bson:"teams"            json:"teams,omitempty"`
	Stage  Stage           `bson:"stage"            json:"stage,omitempty"`

	// Scripts lists the Lua game scripts to load, in load order. Paths are
	// written relative to the config file that declares them, and the script
	// store rewrites them to that base as it loads: a server started from
	// another working directory must find the same files.
	Scripts []string `bson:"scripts,omitempty" json:"scripts,omitempty"`

	PublicData  map[string]interface{} `bson:"data"        json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData" json:"privateData,omitempty"`
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

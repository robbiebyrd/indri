package models

type Script struct {
	Config       Config                 `bson:"config,omitempty" json:"config,omitempty"`
	DefaultTeams map[string]Team        `bson:"defaultTeams"     json:"defaultTeams,omitempty"`
	DefaultStage Stage                  `bson:"defaultStage"     json:"defaultStage,omitempty"`
	PublicData   map[string]interface{} `bson:"data"             json:"data,omitempty"`
	PrivateData  map[string]interface{} `bson:"privateData"      json:"privateData,omitempty"`
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
}

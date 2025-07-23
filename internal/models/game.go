package models

import "github.com/chenmingyong0423/go-mongox/v2"

type Game struct {
	mongox.Model `bson:",inline"`

	Code        string                 `bson:"code"            json:"code"`
	Teams       map[string]Team        `bson:"teams,omitempty" json:"teams,omitempty"`
	Players     map[string]Player      `bson:"players"         json:"players"`
	Stage       Stage                  `bson:"stage"           json:"stage,omitempty"`
	PublicData  map[string]interface{} `bson:"data"            json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"     json:"privateData,omitempty"`
	PlayerData  map[string]interface{} `bson:"playerData"      json:"playerData,omitempty"`
}

type CreateGame struct {
	Code        string                  `json:"code"`
	Teams       *map[string]Team        `json:"teams,omitempty"`
	Stage       *Stage                  `json:"stage,omitempty"`
	PublicData  *map[string]interface{} `json:"data,omitempty"`
	PrivateData *map[string]interface{} `json:"privateData,omitempty"`
	PlayerData  *map[string]interface{} `json:"playerData,omitempty"`
}

type UpdateGame struct {
	Players *map[string]Player `json:"players,omitempty"`
	Teams   *map[string]Team   `json:"teams,omitempty"`
	Stage   *Stage             `json:"stage,omitempty"`
}

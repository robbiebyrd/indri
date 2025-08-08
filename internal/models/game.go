package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Game struct {
	ID bson.ObjectID `bson:"_id,omitempty" json:"id" mongox:"autoID"`

	CreatedAt   time.Time              `bson:"created_at"           json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updated_at"           json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deleted_at,omitempty" json:"-"`
	Code        string                 `bson:"code"                 json:"code"`
	Teams       map[string]Team        `bson:"teams,omitempty"      json:"teams,omitempty"`
	Players     map[string]Player      `bson:"players"              json:"players"`
	Stage       Stage                  `bson:"stage"                json:"stage,omitempty"`
	PublicData  map[string]interface{} `bson:"data"                 json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"          json:"privateData,omitempty"`
	PlayerData  map[string]interface{} `bson:"playerData"           json:"playerData,omitempty"`
	Private     bool                   `bson:"private"              json:"private,omitempty"`
}

type CreateGame struct {
	CreatedAt   time.Time              `bson:"created_at"           json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updated_at"           json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deleted_at,omitempty" json:"-"`
	Code        string                 `bson:"code"                 json:"code"`
	Teams       *map[string]Team       `bson:"teams,omitempty"      json:"teams,omitempty"`
	Players     *map[string]Player     `bson:"players"              json:"players"`
	Stage       *Stage                 `bson:"stage"                json:"stage,omitempty"`
	PublicData  map[string]interface{} `bson:"data"                 json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"          json:"privateData,omitempty"`
	PlayerData  map[string]interface{} `bson:"playerData"           json:"playerData,omitempty"`
	Private     bool                   `bson:"private"              json:"private,omitempty"`
}

type UpdateGame struct {
	Teams       *map[string]Team       `bson:"teams,omitempty" json:"teams,omitempty"`
	Players     *map[string]Player     `bson:"players"         json:"players"`
	Stage       *Stage                 `bson:"stage"           json:"stage,omitempty"`
	UpdatedAt   time.Time              `bson:"updated_at"      json:"updatedAt"`
	PublicData  map[string]interface{} `bson:"data"            json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"     json:"privateData,omitempty"`
	PlayerData  map[string]interface{} `bson:"playerData"      json:"playerData,omitempty"`
	Private     bool                   `bson:"private"         json:"private,omitempty"`
}

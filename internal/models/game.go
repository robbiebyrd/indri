package models

import (
	"time"
)

type Game struct {
	// ID is a backend-neutral string, minted by internal/repo/ids, because a
	// game lives in MongoDB, SQLite or PostgreSQL depending on configuration
	// and only MongoDB has ObjectIDs. The SQL schemas declare `id TEXT`.
	ID string `bson:"_id,omitempty" json:"id"`

	// Version is bumped on every mutation and used for optimistic-concurrency
	// checks so concurrent writers cannot silently lose each other's changes.
	Version int64 `bson:"version" json:"-"`

	CreatedAt   time.Time              `bson:"createdAt"           json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updatedAt"           json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deletedAt,omitempty" json:"-"`
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
	// ID is minted by the caller (internal/repo/ids) rather than by the
	// database, so every backend stores the same identifier for the same game.
	ID          string                 `bson:"_id,omitempty"        json:"id"`
	Version     int64                  `bson:"version"              json:"-"`
	CreatedAt   time.Time              `bson:"createdAt"            json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updatedAt"            json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deletedAt,omitempty"  json:"-"`
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
	Teams       *map[string]Team       `bson:"teams,omitempty"       json:"teams,omitempty"`
	Players     *map[string]Player     `bson:"players,omitempty"     json:"players"`
	Stage       *Stage                 `bson:"stage,omitempty"       json:"stage,omitempty"`
	UpdatedAt   time.Time              `bson:"updatedAt"             json:"updatedAt"`
	PublicData  map[string]interface{} `bson:"data,omitempty"        json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData,omitempty" json:"privateData,omitempty"`
	PlayerData  map[string]interface{} `bson:"playerData,omitempty"  json:"playerData,omitempty"`
	Private     bool                   `bson:"private"               json:"private,omitempty"`
}

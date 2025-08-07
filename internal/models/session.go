package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Session struct {
	ID bson.ObjectID `bson:"_id,omitempty"        json:"id"               mongox:"autoID"`

	GameID *string `bson:"gameId,omitempty"     json:"gameId,omitempty"`
	UserID *string `bson:"userId,omitempty"     json:"userId,omitempty"`
	TeamID *string `bson:"teamId,omitempty"     json:"teamId,omitempty"`

	CreatedAt time.Time `bson:"created_at"           json:"createdAt"`
	UpdatedAt time.Time `bson:"updated_at"           json:"updatedAt"`
	DeletedAt time.Time `bson:"deleted_at,omitempty" json:"-"`
}

type CreateSession struct {
	GameID string `bson:"gameId"     json:"gameId"`
	UserID string `bson:"userId"     json:"userId"`
	TeamID string `bson:"teamId"     json:"teamId"`
	CreatedAt time.Time `bson:"created_at" json:"createdAt"`
}

type UpdateSession struct {
	ID bson.ObjectID `bson:"_id,omitempty"        json:"id"               mongox:"autoID"`

	GameID string `bson:"gameId"     json:"gameId"`
	UserID string `bson:"userId"     json:"userId"`
	TeamID string `bson:"teamId"     json:"teamId"`

	UpdatedAt time.Time `bson:"updated_at" json:"updatedAt"`
}

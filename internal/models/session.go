package models

import (
	"time"
)

type Session struct {
	ID string `bson:"_id,omitempty" json:"id"`

	// Token is the unguessable bearer token used to resume this session.
	// It is never serialized to clients as part of session state.
	Token string `bson:"token,omitempty" json:"-"`

	GameID *string `bson:"gameId,omitempty" json:"gameId,omitempty"`
	UserID *string `bson:"userId,omitempty" json:"userId,omitempty"`
	TeamID *string `bson:"teamId,omitempty" json:"teamId,omitempty"`
	SlotID *string `bson:"slotId,omitempty" json:"slotId,omitempty"`

	CreatedAt time.Time `bson:"createdAt"           json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"           json:"updatedAt"`
	DeletedAt time.Time `bson:"deletedAt,omitempty" json:"-"`
}

type CreateSession struct {
	Token     string    `bson:"token"     json:"-"`
	GameID    string    `bson:"gameId"     json:"gameId"`
	UserID    string    `bson:"userId"     json:"userId"`
	TeamID    string    `bson:"teamId"     json:"teamId"`
	SlotID    string    `bson:"slotId"     json:"slotId"`
	CreatedAt time.Time `bson:"createdAt"  json:"createdAt"`
}

type UpdateSession struct {
	GameID    string    `bson:"gameId"    json:"gameId"`
	UserID    string    `bson:"userId"    json:"userId"`
	TeamID    string    `bson:"teamId"    json:"teamId"`
	SlotID    string    `bson:"slotId"    json:"slotId"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

package models

import (
	"time"
)

type Session struct {
	// ID is a backend-neutral string minted by internal/repo/ids; see
	// models.Game.ID for why it is not a Mongo ObjectID.
	//
	// This is the value the transport stores under the connection key
	// "sessionId" and the one broadcasts resolve recipients by. It is never
	// sent to a client — the wire field also called sessionId carries Token.
	ID string `bson:"_id,omitempty" json:"id"`

	// Token is the unguessable bearer token used to resume this session.
	// It is never serialized to clients as part of session state.
	Token string `bson:"token,omitempty" json:"-"`

	GameID *string `bson:"gameId,omitempty" json:"gameId,omitempty"`
	UserID *string `bson:"userId,omitempty" json:"userId,omitempty"`
	TeamID *string `bson:"teamId,omitempty" json:"teamId,omitempty"`

	// SlotID is the key of this session's player in Game.Players. Holding a
	// session that names a slot is not holding the slot: the slot's own
	// Player.UserID is the authority on who holds it.
	SlotID *string `bson:"slotId,omitempty" json:"slotId,omitempty"`

	CreatedAt time.Time `bson:"createdAt"           json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"           json:"updatedAt"`
	DeletedAt time.Time `bson:"deletedAt,omitempty" json:"-"`
}

type CreateSession struct {
	// ID is minted by the caller (internal/repo/ids); see models.Game.ID.
	ID        string    `bson:"_id,omitempty" json:"id"`
	Token     string    `bson:"token"     json:"-"`
	GameID    string    `bson:"gameId"     json:"gameId"`
	UserID    string    `bson:"userId"     json:"userId"`
	TeamID    string    `bson:"teamId"     json:"teamId"`
	SlotID    string    `bson:"slotId"     json:"slotId"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

type UpdateSession struct {
	GameID string `bson:"gameId" json:"gameId"`
	UserID string `bson:"userId" json:"userId"`
	TeamID string `bson:"teamId" json:"teamId"`
	SlotID string `bson:"slotId" json:"slotId"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

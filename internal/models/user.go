package models

import (
	"time"
)

type User struct {
	// ID is a backend-neutral string minted by internal/repo/ids; see
	// models.Game.ID for why it is not a Mongo ObjectID.
	ID string `bson:"_id,omitempty" json:"id"`

	CreatedAt   time.Time              `bson:"createdAt"            json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updatedAt"            json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deletedAt,omitempty"  json:"-"`
	Email       string                 `bson:"email"                json:"email"`
	Name        string                 `bson:"name"                 json:"name"`
	DisplayName *string                `bson:"displayName"          json:"displayName"`
	Password    *string                `bson:"password"             json:"-"`
	Score       *int                   `bson:"score"                json:"score"`
	PublicData  map[string]interface{} `bson:"data"                 json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"          json:"privateData,omitempty"`
}

// CreateUser is what a registration asks for. It carries no id: register
// decodes it straight from the client's payload, and an id the client chose
// could name — and replace — another user. The store mints it.
type CreateUser struct {
	CreatedAt   time.Time `bson:"createdAt"   json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
	Email       string    `bson:"email"       json:"email"`
	Name        string    `bson:"name"        json:"name"`
	DisplayName *string   `bson:"displayName" json:"displayName"`
	Password    *string   `bson:"password"    json:"password,omitempty"`
}

type UpdateUser struct {
	ID          string    `bson:"_id,omitempty"         json:"id"`
	UpdatedAt   time.Time `bson:"updatedAt"             json:"updatedAt"`
	Email       string    `bson:"email,omitempty"       json:"email"`
	Name        string    `bson:"name,omitempty"        json:"name"`
	DisplayName *string   `bson:"displayName,omitempty" json:"displayName"`
	Password    *string   `bson:"password,omitempty"    json:"-"`
}

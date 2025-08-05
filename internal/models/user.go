package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID bson.ObjectID `bson:"_id,omitempty" json:"id" mongox:"autoID"`

	CreatedAt   time.Time              `bson:"created_at"           json:"createdAt"`
	UpdatedAt   time.Time              `bson:"updated_at"           json:"updatedAt"`
	DeletedAt   time.Time              `bson:"deleted_at,omitempty" json:"-"`
	Email       string                 `bson:"email"                json:"email"`
	Name        string                 `bson:"name"                 json:"name"`
	DisplayName *string                `bson:"displayName"          json:"displayName"`
	Password    *string                `bson:"password"             json:"-"`
	Score       *int                   `bson:"score"                json:"score"`
	PublicData  map[string]interface{} `bson:"data"                 json:"data,omitempty"`
	PrivateData map[string]interface{} `bson:"privateData"          json:"privateData,omitempty"`
}

type CreateUser struct {
	CreatedAt   time.Time `bson:"created_at"  json:"createdAt"`
	Email       string    `bson:"email"       json:"email"`
	Name        string    `bson:"name"        json:"name"`
	DisplayName *string   `bson:"displayName" json:"displayName"`
	Password    *string   `bson:"password"    json:"-"`
}

type UpdateUser struct {
	ID          string    `bson:"_id,omitempty" json:"id"`
	UpdatedAt   time.Time `bson:"updated_at"    json:"updatedAt"`
	Email       string    `bson:"email"         json:"email"`
	Name        string    `bson:"name"          json:"name"`
	DisplayName *string   `bson:"displayName"   json:"displayName"`
	Password    *string   `bson:"password"      json:"-"`
}

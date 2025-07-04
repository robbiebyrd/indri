package models

import (
	"github.com/chenmingyong0423/go-mongox/v2"
)

type User struct {
	mongox.Model `bson:",inline"`

	Email       string  `bson:"email"       json:"email"`
	Name        string  `bson:"name"        json:"name"`
	DisplayName *string `bson:"displayName" json:"displayName"`
	Password    *string `bson:"password"    json:"-"`
	Score       *int    `bson:"score"       json:"score"`

	CommonDataFields
}

type CreateUser struct {
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	DisplayName *string `json:"displayName,omitempty"`
	Password    string  `json:"password"`
}

type UpdateUser struct {
	ID          string  `json:"id"`
	Email       *string `json:"email,omitempty"`
	Name        *string `json:"name,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	Password    *string `json:"password,omitempty"`
}

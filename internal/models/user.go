package models

import (
	"github.com/chenmingyong0423/go-mongox/v2"
)

type User struct {
	mongox.Model `bson:",inline"`

	Email       *string `bson:"email"       json:"email"`
	Name        string  `bson:"name"        json:"name"`
	DisplayName string  `bson:"displayName" json:"displayName"`
	Password    *string `bson:"password"    json:"-"`
	Score       *int    `bson:"score"       json:"score"`
}

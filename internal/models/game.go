package models

import "github.com/chenmingyong0423/go-mongox/v2"

type Game struct {
	mongox.Model `bson:",inline"`

	Code  string          `bson:"code"            json:"code"`
	Teams map[string]Team `bson:"teams,omitempty" json:"teams,omitempty"`
	Stage Stage           `bson:"stage"           json:"stage,omitempty"`
}

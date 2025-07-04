package models

import "github.com/chenmingyong0423/go-mongox/v2"

type Game struct {
	mongox.Model `bson:",inline"`

	Code  string          `bson:"code"            json:"code"`
	Teams map[string]Team `bson:"teams,omitempty" json:"teams,omitempty"`
	Stage Stage           `bson:"stage"           json:"stage,omitempty"`
}

type CreateGame struct {
	Code  string          `json:"code"`
	Teams map[string]Team `json:"teams,omitempty"`
	Stage Stage           `json:"stage,omitempty"`
}

type UpdateGame struct {
	Teams *map[string]Team `json:"teams,omitempty"`
	Stage *Stage           `json:"stage,omitempty"`
}

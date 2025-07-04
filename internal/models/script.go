package models

import "github.com/chenmingyong0423/go-mongox/v2"

type Script struct {
	mongox.Model `bson:",inline"`

	Name      string `bson:"name"`
	CreatedBy string `bson:"createdBy"`
	Shared    bool   `bson:"shared"`

	Stage *Stage `bson:"stages,omitempty" json:"stages,omitempty"`

	CommonDataFields
}

type CreateScript struct {
	Stage Stage `json:"stage"`
}

type UpdateScript struct {
	Stage *Stage `json:"stage,omitempty"`
}

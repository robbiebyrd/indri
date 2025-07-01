package models

import "github.com/chenmingyong0423/go-mongox/v2"

type Script struct {
	mongox.Model `bson:",inline"`

	Stage         *Stage `bson:"stages,omitempty" json:"stages,omitempty"`

	CommonDataFields `bson:",inline" json:"commonDataFields"`
}

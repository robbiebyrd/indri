package models

type Team struct {
	Name    string            `bson:"name"    json:"name"`
	Players map[string]Player `bson:"players" json:"players"`

	CommonDataFields
}

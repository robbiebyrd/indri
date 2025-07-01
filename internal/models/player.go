package models

type Player struct {
	Name         string `bson:"name"         json:"name"`
	Score        int    `bson:"score"        json:"score"`
	Disconnected bool   `bson:"disconnected" json:"disconnected"`
	Host         bool   `bson:"host"         json:"host"`

	CommonDataFields `bson:",inline"`
}

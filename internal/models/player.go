package models

type Player struct {
	Name         string `bson:"name"         json:"name"`
	Score        int    `bson:"score"        json:"score"`
	Disconnected bool   `bson:"disconnected" json:"disconnected"`
	Host         bool   `bson:"host"         json:"host"`
	Controller   bool   `bson:"controller"   json:"controller"`

	PublicData  *map[string]interface{}            `bson:"data"        json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privateData" json:"privateData,omitempty"`
}

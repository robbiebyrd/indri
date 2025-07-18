package models

type Scene struct {
	PublicData  *map[string]interface{}            `bson:"data"        json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privateData" json:"privateData,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerData"  json:"playerData,omitempty"`
}

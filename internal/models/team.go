package models

type Team struct {
	Name      string   `bson:"name"      json:"name"`
	PlayerIDs []string `bson:"playerIds" json:"playerIds"`

	PublicData  map[string]interface{}            `bson:"data"        json:"data,omitempty"`
	PrivateData map[string]interface{}            `bson:"privateData" json:"privateData,omitempty"`
	PlayerData  map[string]map[string]interface{} `bson:"playerData"  json:"playerData,omitempty"`
}

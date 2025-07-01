package models

type genericDataType map[string]interface{}

type CommonDataFields struct {
	PublicData  *genericDataType            `bson:"data"        json:"data,omitempty"`
	PrivateData *genericDataType            `bson:"privateData" json:"privateData,omitempty"`
	PlayerData  *map[string]genericDataType `bson:"playerData"  json:"playerData,omitempty"`
}

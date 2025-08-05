package models

type Scene struct {
	PublicData  *map[string]interface{}            `bson:"publicdata"  json:"publicdata,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privatedata" json:"privatedata,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerdata"  json:"playerdata,omitempty"`
}

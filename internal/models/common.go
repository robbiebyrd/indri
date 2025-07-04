package models

type CommonDataFields struct {
	PublicData  *map[string]interface{}            `bson:"data"        json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privateData" json:"privateData,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerData"  json:"playerData,omitempty"`
}

type DataStoreType string

const (
	DataStorePublic  DataStoreType = "data"
	DataStorePrivate DataStoreType = "privateData"
	DataStorePlayer  DataStoreType = "playerData"
)

func (d DataStoreType) String() string {
	return string(d)
}

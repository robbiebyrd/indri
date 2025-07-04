package models

type Stage struct {
	ScriptID     string           `bson:"scriptId"         json:"-"`
	CurrentScene string           `bson:"currentScene"     json:"currentScene"`
	SceneOrder   []string         `bson:"sceneOrder"       json:"sceneOrder"`
	Scenes       map[string]Scene `bson:"scenes,omitempty" json:"scenes,omitempty"`

	PublicData  *map[string]interface{}            `bson:"data"        json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privateData" json:"privateData,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerData"  json:"playerData,omitempty"`
}

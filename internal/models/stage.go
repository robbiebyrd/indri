package models

type Stage struct {
	Script       Script           `bson:"script"           json:"-"`
	CurrentScene string           `bson:"currentScene"     json:"currentScene"`
	SceneOrder   []string         `bson:"sceneOrder"       json:"sceneOrder"`
	Scenes       map[string]Scene `bson:"scenes,omitempty" json:"scenes,omitempty"`

	CommonDataFields `bson:",inline" json:"commonDataFields"`
}

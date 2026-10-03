package models

// Player is one pre-declared slot in a game. Game.Players is keyed by slot id,
// not user id: UserID names the slot's current holder and is empty while the
// slot is free.
type Player struct {
	UserID      string                  `bson:"userId"      json:"userId"`
	Name        string                  `bson:"name"        json:"name"`
	Score       int                     `bson:"score"       json:"score"`
	Connected   bool                    `bson:"connected"   json:"connected"`
	Host        bool                    `bson:"host"        json:"host"`
	Controller  bool                    `bson:"controller"  json:"controller"`
	PublicData  *map[string]interface{} `bson:"data"        json:"data,omitempty"`
	PrivateData *map[string]interface{} `bson:"privateData" json:"privateData,omitempty"`
}

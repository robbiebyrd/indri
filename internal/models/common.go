package models

// DataStoreType names one of the three data stores a game, stage, scene, team
// or player carries.
//
// The value is the field name itself, and it is deliberately one name rather
// than two: a caller composes it into a dotted path that is used both as a
// MongoDB update path (resolved by bson tag) and as the path of the published
// delta (read by json tag), so it is only usable while a store's bson and json
// names agree. They now agree on every store of every type in the game
// document; Scene.PrivateData was the last that did not, and models.Scene
// records what flipping it cost.
type DataStoreType string

const (
	DataStorePublic  DataStoreType = "data"
	DataStorePrivate DataStoreType = "privateData"
	DataStorePlayer  DataStoreType = "playerData"
)

func (d DataStoreType) String() string {
	return string(d)
}

package models

// DataStoreType names one of the three data stores a game, stage, scene, team
// or player carries.
//
// The value is the field name itself, and it is deliberately one name rather
// than two: a caller composes it into a dotted path that is used both as a
// MongoDB update path (resolved by bson tag) and as the path of the published
// delta (read by json tag), so it is only usable while a store's bson and json
// names agree. They agree everywhere except Scene.PrivateData, which is tagged
// bson "private_data" — see models.Scene for the defect, the direction chosen,
// and the persisted data that blocks it.
type DataStoreType string

const (
	DataStorePublic  DataStoreType = "data"
	DataStorePrivate DataStoreType = "privateData"
	DataStorePlayer  DataStoreType = "playerData"
)

func (d DataStoreType) String() string {
	return string(d)
}

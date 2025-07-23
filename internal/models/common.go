package models

type DataStoreType string

const (
	DataStorePublic  DataStoreType = "data"
	DataStorePrivate DataStoreType = "privateData"
	DataStorePlayer  DataStoreType = "playerData"
)

func (d DataStoreType) String() string {
	return string(d)
}

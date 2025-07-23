package changestream

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"time"
)

type ChangeEventIn struct {
	OperationType     string                   `json:"operationType"`
	WallTime          time.Time                `json:"wallTime"`
	FullDocument      map[string]interface{}   `json:"fullDocument"`
	Ns                Namespace                `json:"ns"`
	DocumentKey       map[string]bson.ObjectID `json:"documentKey"`
	UpdateDescription UpdateDescription        `json:"updateDescription"`
}

type ClusterTime struct {
	T int `json:"T"`
	I int `json:"I"`
}

type Namespace struct {
	Db   string `json:"db"`
	Coll string `json:"coll"`
}

type DocumentKey struct {
	ID bson.ObjectID `json:"_id"`
}

type UpdateDescription struct {
	UpdatedFields   *map[string]interface{}   `json:"updatedFields,omitempty"`
	RemovedFields   *[]string                 `json:"removedFields,omitempty"`
	TruncatedArrays *[]map[string]interface{} `json:"truncatedArrays,omitempty"`
}

type ChangeEventOut struct {
	ID            bson.ObjectID           `json:"id"`
	OperationType OperationType           `json:"op"`
	Timestamp     time.Time               `json:"ts"`
	Collection    *string                 `json:"type,omitempty"`
	UpdatedFields *map[string]interface{} `json:"updated,omitempty"`
	RemovedFields *[]string               `json:"removed,omitempty"`
}

type OperationType string

const (
	StatusInsert  OperationType = "insert"
	StatusUpdate  OperationType = "update"
	StatusReplace OperationType = "replace"
	StatusDelete  OperationType = "delete"
)

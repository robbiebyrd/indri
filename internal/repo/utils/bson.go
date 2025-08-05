package utils

import "go.mongodb.org/mongo-driver/v2/bson"

func CreateBSONDoc(doc interface{}) (bson.D, error) {
	// Convert the incoming UpdateUser model to BSON bytes
	bsonBytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, err
	}

	// Unmarshall the bytes to a BSON Document type
	var bsonDoc bson.D

	err = bson.Unmarshal(bsonBytes, &bsonDoc)
	if err != nil {
		return nil, err
	}

	return bsonDoc, nil
}

package changestream

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func getCollection(db *mongo.Database, collectionName string) *mongo.Collection {
	return db.Collection(collectionName)
}

func getDB(client mongo.Client, dbName string) *mongo.Database {
	return client.Database(dbName)
}

func getClient(mongoURI string) (*mongo.Client, error) {
	return mongo.Connect(options.Client().ApplyURI(mongoURI))
}

func getChangeStream(ctx context.Context, collection *mongo.Collection, filter *bson.D) (*mongo.ChangeStream, error) {
	pipe := getFilterPipeline(filter)

	changeStreamOptions := options.ChangeStream().SetFullDocument(options.UpdateLookup)

	cs, err := collection.Watch(ctx, pipe, changeStreamOptions)
	return cs, err
}

func getChangeStreamGlobal(ctx context.Context, db *mongo.Database) (*mongo.ChangeStream, error) {
	changeStreamOptions := options.ChangeStream().SetFullDocument(options.Default)
	cs, err := db.Watch(ctx, mongo.Pipeline{}, changeStreamOptions)
	return cs, err
}

func getFilterPipeline(filter *bson.D) mongo.Pipeline {
	if filter == nil {
		return mongo.Pipeline{}
	}

	return mongo.Pipeline{*filter}
}

func stringToOpCode(str string) (OperationType, error) {
	switch str {
	case string(StatusInsert):
		return StatusInsert, nil
	case string(StatusDelete):
		return StatusDelete, nil
	case string(StatusUpdate):
		return StatusUpdate, nil
	case string(StatusReplace):
		return StatusReplace, nil
	}
	return "", fmt.Errorf("could not parse update type from changestream, got %v", str)
}

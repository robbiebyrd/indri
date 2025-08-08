package changestream

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
)

type MongoChangeMonitor struct {
	client       *mongo.Client
	database     *mongo.Database
	changeStream *mongo.ChangeStream
	filter       *bson.D
}

func New(ctx context.Context, mongoClient *mongoClient.Client, collectionName *string, filterDoc *bson.D) (*MongoChangeMonitor, error) {
	log.Printf("New change monitor starting on collection %v", collectionName)

	var changeStream *mongo.ChangeStream

	var err error

	if collectionName == nil {
		changeStream, err = getChangeStreamGlobal(ctx, mongoClient.Database.Database())
	} else {
		collection := getCollection(mongoClient.Database.Database(), *collectionName)
		changeStream, err = getChangeStream(ctx, collection, filterDoc)
	}

	if err != nil {
		return nil, err
	}

	return &MongoChangeMonitor{
		client:       mongoClient.MongoClient,
		database:     mongoClient.Database.Database(),
		changeStream: changeStream,
		filter:       filterDoc,
	}, nil
}

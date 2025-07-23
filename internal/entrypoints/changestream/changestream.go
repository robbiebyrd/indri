package changestream

import (
	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"log"

	"context"
)

type MongoChangeMonitor struct {
	client       *mongo.Client
	database     *mongo.Database
	changeStream *mongo.ChangeStream
	filter       *bson.D
}

func New(ctx context.Context, collectionName *string, filterDoc *bson.D) (*MongoChangeMonitor, error) {
	log.Printf("New change monitor starting on collection %v", collectionName)

	client, err := mongoClient.New()
	if err != nil {
		return nil, err
	}

	var changeStream *mongo.ChangeStream

	if collectionName == nil {
		changeStream, err = getChangeStreamGlobal(ctx, client.Database.Database())
	} else {
		collection := getCollection(client.Database.Database(), *collectionName)
		changeStream, err = getChangeStream(ctx, collection, filterDoc)
	}
	if err != nil {
		return nil, err
	}

	return &MongoChangeMonitor{
		client:       client.MongoClient,
		database:     client.Database.Database(),
		changeStream: changeStream,
		filter:       filterDoc,
	}, nil
}

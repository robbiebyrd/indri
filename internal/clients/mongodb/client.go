package mongodb

import (
	"context"
	"fmt"
	"log"

	"github.com/chenmingyong0423/go-mongox/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

var mongodbClient *Client

type Client struct {
	Database    *mongox.Database
	ORM         *mongox.Client
	MongoClient *mongo.Client
}

func New(ctx context.Context) (*Client, error) {
	if mongodbClient != nil {
		return mongodbClient, nil
	}

	vars := envVars.GetEnv()

	log.Printf("Connecting to MongoDB at %s\n", vars.MongoURI)

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(vars.MongoURI))
	if err != nil {
		log.Fatal(fmt.Errorf("could not configure connection to MongoDB, exiting: %v", err))
	}

	err = mongoClient.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatal(fmt.Errorf("could not ping MongoDB because it appears offline, exiting: %v", err))
	}

	client := mongox.NewClient(mongoClient, &mongox.Config{})
	database := client.NewDatabase(vars.MongoDatabase)

	log.Println("successfully connected to MongoDB")

	mongodbClient := &Client{
		Database:    database,
		ORM:         client,
		MongoClient: mongoClient,
	}

	return mongodbClient, nil
}

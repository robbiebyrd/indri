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

	log.Printf("Connecting to MongoDB database %q\n", vars.MongoDatabase)

	// DefaultDocumentM makes a nested document inside an interface{} field
	// decode as bson.M (a map) rather than the driver's default bson.D (an
	// ordered []bson.E). Every `data`/`privateData`/`playerData` blob is
	// map[string]interface{}, and all the application code that walks one —
	// the layout ops, game handlers — assumes a map. Without this, a document
	// read back from Mongo is a different shape from the one that was written,
	// so map indexing silently misses and type switches fall through.
	//
	// This does NOT affect arrays: they still decode as bson.A. It also does
	// not affect the many bson.D literals in internal/repo, which construct
	// queries rather than decode results.
	clientOpts := options.Client().
		ApplyURI(vars.MongoURI).
		SetBSONOptions(&options.BSONOptions{DefaultDocumentM: true})

	mongoClient, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("could not configure connection to MongoDB: %w", err)
	}

	err = mongoClient.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, fmt.Errorf("could not ping MongoDB because it appears offline: %w", err)
	}

	client := mongox.NewClient(mongoClient, &mongox.Config{})
	database := client.NewDatabase(vars.MongoDatabase)

	log.Println("successfully connected to MongoDB")

	mongodbClient = &Client{
		Database:    database,
		ORM:         client,
		MongoClient: mongoClient,
	}

	return mongodbClient, nil
}

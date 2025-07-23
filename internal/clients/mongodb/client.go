package mongodb

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/robbiebyrd/indri/internal/repo/env"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"log"
	"strconv"
)

/*
Client encapsulates the MongoDB client, ORM, and database connection.
It provides a structured way to interact with MongoDB resources within the application.
*/
type Client struct {
	Database    *mongox.Database
	ORM         *mongox.Client
	MongoClient *mongo.Client
}

/*
New returns an instance of the MongoDB client, establishing a connection if one does not already exist.
It configures the client using environment variables and ensures the connection is valid before returning.

Returns:

	*Client: A pointer to the singleton Client instance.
	error: An error if the connection or configuration fails, otherwise nil.
*/
func New() (*Client, error) {
	envVars := env.GetEnv()

	url := "mongodb://" + envVars.MongoHost + ":" + strconv.Itoa(envVars.MongoPort)
	log.Printf("Connecting to MongoDB at %s\n", url)

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(url).SetAuth(options.Credential{
		Username:   envVars.MongoUsername,
		Password:   envVars.MongoPassword,
		AuthSource: envVars.MongoAuthDatabase,
	}))
	if err != nil {
		log.Fatal(fmt.Errorf("could not configure connection to MongoDB, exiting: %v", err))
	}

	err = mongoClient.Ping(context.Background(), readpref.Primary())
	if err != nil {
		log.Fatal(fmt.Errorf("could not ping MongoDB because it appears offline, exiting: %v", err))
	}

	client := mongox.NewClient(mongoClient, &mongox.Config{})
	database := client.NewDatabase(envVars.MongoDatabase)

	log.Println("successfully connected to MongoDB")

	return &Client{
		Database:    database,
		ORM:         client,
		MongoClient: mongoClient,
	}, nil
}

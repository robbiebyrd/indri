package mongodb

import (
	"context"
	"fmt"
	"github.com/chenmingyong0423/go-mongox/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"indri/internal/repo/env"
	"log"
	"strconv"
)

type Client struct {
	Database    *mongox.Database
	ORM         *mongox.Client
	MongoClient *mongo.Client
}

var globalClient *Client

func New() (*Client, error) {
	envVars := env.GetEnv()

	if globalClient != nil {
		return globalClient, nil
	}

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

	globalClient = &Client{
		Database:    database,
		ORM:         client,
		MongoClient: mongoClient,
	}

	return globalClient, nil
}

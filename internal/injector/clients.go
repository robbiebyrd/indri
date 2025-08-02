package injector

import (
	"context"
	"github.com/olahol/melody"
	mClient "github.com/robbiebyrd/indri/internal/clients/melody"
	mongoClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/entrypoints/changestream"
)

var globalClientsInjector *ClientsInjector

func GetClients(ctx context.Context, mongodbClient *mongoClient.Client, melodyClient *melody.Melody, globalMonitor *changestream.MongoChangeMonitor) (*ClientsInjector, error) {
	if globalClientsInjector != nil {
		return globalClientsInjector, nil
	}

	if mongodbClient == nil {
		newMongodbClient, err := mongoClient.New(ctx)
		if err != nil {
			return nil, err
		}

		mongodbClient = newMongodbClient
	}

	if melodyClient == nil {
		melodyClient = mClient.New()
	}

	if globalMonitor == nil {
		newGlobalMonitor, err := changestream.New(ctx, mongodbClient, nil, nil)
		if err != nil {
			return nil, err
		}

		globalMonitor = newGlobalMonitor
	}

	return &ClientsInjector{
		MongoDBClient: mongodbClient,
		MelodyClient:  melodyClient,
		GlobalMonitor: globalMonitor,
		Context:       ctx,
	}, nil
}

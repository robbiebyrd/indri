package injector

import (
	"github.com/olahol/melody"
	mClient "github.com/robbiebyrd/indri/internal/clients/melody"
	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/entrypoints/changestream"
)

type Injector interface {
	Inject()
}

type Data struct {
	MongoDBClient *mongodb.Client
	MelodyClient  *melody.Melody
	GlobalMonitor *changestream.MongoChangeMonitor
}

var globalInjector *Data

func New(mongodbClient *mongodb.Client, melodyClient *melody.Melody, globalMonitor *changestream.MongoChangeMonitor) (*Data, error) {

	if globalInjector != nil {
		return globalInjector, nil
	}

	if mongodbClient == nil {
		newMongodbClient, err := mongodb.New()
		if err != nil {
			return nil, err
		}

		mongodbClient = newMongodbClient
	}

	if melodyClient == nil {
		melodyClient = mClient.New()
	}

	if globalMonitor == nil {
		newGlobalMonitor, err := changestream.New(nil, nil, nil)
		if err != nil {
			return nil, err
		}
		globalMonitor = newGlobalMonitor
	}
	return &Data{
		MongoDBClient: mongodbClient,
		MelodyClient:  melodyClient,
	}, nil
}

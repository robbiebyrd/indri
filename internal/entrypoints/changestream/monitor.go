package changestream

import (
	"context"
	"fmt"
	"log"
)

func (cm *MongoChangeMonitor) Monitor(ctx context.Context, channel chan<- ChangeEventOut) {
	log.Printf("Started watching changes on db %v", cm.database.Name())

	defer func() {
		if err := cm.changeStream.Close(ctx); err != nil {
			log.Printf("error closing change stream: %v", err)
		}
	}()

	for cm.changeStream.Next(ctx) {
		event, err := cm.processEvent()
		if err != nil {
			log.Printf("error processing event: %v", err)
			continue
		}
		channel <- *event
	}

	if err := cm.changeStream.Err(); err != nil {
		log.Printf("change stream error occured while monitoring: %v", err)
	}
}

func (cm *MongoChangeMonitor) processEvent() (*ChangeEventOut, error) {
	var docIn *ChangeEventIn

	if err := cm.changeStream.Decode(&docIn); err != nil {
		log.Printf("error decoding change stream: %v", err)
		return nil, err
	}

	id, ok := docIn.DocumentKey["_id"]
	if !ok {
		return nil, fmt.Errorf("event missing _id field: %v", docIn)
	}

	opType, err := stringToOpCode(docIn.OperationType)
	if err != nil {
		return nil, err
	}

	if opType != OpUpdate {
		return nil, fmt.Errorf("unexpected operation type: %v", opType)
	}

	return &ChangeEventOut{
		ID:            id,
		Collection:    &docIn.Ns.Coll,
		OperationType: opType,
		Timestamp:     docIn.WallTime,
		UpdatedFields: docIn.UpdateDescription.UpdatedFields,
		RemovedFields: docIn.UpdateDescription.RemovedFields,
	}, nil
}

package utils

import (
	"encoding/json"
	"errors"
	"github.com/olahol/melody"
	"indri/internal/models"
	"log"
)

type MessageHandlerInterface interface {
	HandleMessage(s *melody.Session, decodedMsg map[string]interface{}) *models.Game
}

func DecodeMessageWithAction(msg []byte) (*string, *map[string]interface{}, error) {
	var decodedMsg map[string]interface{}

	err := json.Unmarshal(msg, &decodedMsg)
	if err != nil {
		log.Println(err)
	}

	action, ok := decodedMsg["action"].(string)
	if !ok || action == "" {
		log.Println(err)
		return nil, nil, errors.New("could not parse action")
	}

	delete(decodedMsg, "action")

	return &action, &decodedMsg, nil
}

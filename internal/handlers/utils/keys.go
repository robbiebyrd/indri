package utils

import (
	"errors"
)

func ParseGameCodeAndTeamID(decodedMsg map[string]interface{}) (*string, *string) {
	gameCode, _ := decodedMsg["code"].(string)
	teamId, _ := decodedMsg["teamId"].(string)

	return &gameCode, &teamId
}

func RequireGameCodeAndTeamID(decodedMsg map[string]interface{}) (*string, *string, error) {
	gameCode, ok := decodedMsg["code"].(string)
	if !ok {
		return nil, nil, errors.New("could not parse gameCode from request")
	}

	teamId, ok := decodedMsg["teamId"].(string)
	if !ok {
		return nil, nil, errors.New("could not parse teamId from request")
	}

	return &gameCode, &teamId, nil
}

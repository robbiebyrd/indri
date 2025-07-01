package utils

import (
	"errors"
)

func ParseGameIDAndTeamID(decodedMsg map[string]interface{}) (*string, *string) {
	gameId, _ := decodedMsg["gameId"].(string)
	teamId, _ := decodedMsg["teamId"].(string)

	return &gameId, &teamId
}

func RequireGameIDAndTeamID(decodedMsg map[string]interface{}) (*string, *string, error) {
	gameId, ok := decodedMsg["gameId"].(string)
	if !ok {
		return nil, nil, errors.New("could not parse gameId from request")
	}

	teamId, ok := decodedMsg["teamId"].(string)
	if !ok {
		return nil, nil, errors.New("could not parse teamId from request")
	}

	return &gameId, &teamId, nil
}

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
	gameCode, teamId := ParseGameCodeAndTeamID(decodedMsg)

	if gameCode != nil && len(*gameCode) > 0 {
		return nil, nil, errors.New("could not parse gameCode from request")
	}

	if teamId != nil && len(*teamId) > 0 {
		return nil, nil, errors.New("could not parse gameCode from request")
	}

	return gameCode, teamId, nil
}

func RequireGameCode(decodedMsg map[string]interface{}) (*string, error) {
	gameCode, ok := decodedMsg["code"].(string)
	if !ok {
		return nil, errors.New("could not parse gameCode from request")
	}

	return &gameCode, nil
}

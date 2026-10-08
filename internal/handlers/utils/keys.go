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

	if gameCode == nil || len(*gameCode) == 0 {
		return nil, nil, errors.New("could not parse gameCode from request")
	} else if teamId == nil || len(*teamId) == 0 {
		return nil, nil, errors.New("could not parse teamId from request")
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

// HeldLayoutHash is the layout the caller says it already holds, by the hash a
// keyframe gave it, or "" when it names none. join, create, refresh and
// reconnect take it, so the keyframe they answer with can leave that layout
// out; see GameService.Keyframe.
func HeldLayoutHash(decodedMsg map[string]interface{}) string {
	hash, _ := decodedMsg["layoutHash"].(string)

	return hash
}

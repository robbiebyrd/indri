package session

import "fmt"

func ValidateStandardKeys(gameId *string, teamId *string, userId *string) (string, string, string, error) {
	if gameId == nil {
		return "", "", "", fmt.Errorf("gameId is required")
	}

	if teamId == nil || *teamId == "" {
		return "", "", "", fmt.Errorf("teamId is required")
	}

	if userId == nil || *userId == "" {
		return "", "", "", fmt.Errorf("userId is required")
	}

	return *gameId, *teamId, *userId, nil

}

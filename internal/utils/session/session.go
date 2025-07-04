package session

import "fmt"

func ValidateStandardKeys(gameCode *string, teamId *string, userId *string) (string, string, string, error) {
	if gameCode == nil {
		return "", "", "", fmt.Errorf("gameCode is required")
	}

	if teamId == nil || *teamId == "" {
		return "", "", "", fmt.Errorf("teamId is required")
	}

	if userId == nil || *userId == "" {
		return "", "", "", fmt.Errorf("userId is required")
	}

	return *gameCode, *teamId, *userId, nil

}

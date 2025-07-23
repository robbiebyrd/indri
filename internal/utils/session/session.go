package session

import "fmt"

func ValidateStandardKeys(gameCode string, teamId string, userId string) error {
	if userId == "" {
		return fmt.Errorf("userId is required")
	}

	return ValidateGameAndUser(gameCode, userId)
}

func ValidateGameAndUser(gameCode string, userId string) error {
	if gameCode == "" {
		return fmt.Errorf("gameCode is required")
	}

	if userId == "" {
		return fmt.Errorf("userId is required")
	}

	return nil
}

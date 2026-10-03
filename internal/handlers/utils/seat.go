package utils

import (
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
)

// Seater is the part of the game service that seats a player and renders the
// game they were seated in.
type Seater interface {
	ConnectPlayer(id string, teamId string, userId string, displayName string) (string, error)
	GetJSONBytes(id string) (*[]byte, error)
}

// UserGetter resolves the caller's user, for their display name.
type UserGetter interface {
	Get(id string) (*models.User, error)
}

// SessionUpdater records the caller's game, team and slot on their session.
type SessionUpdater interface {
	Update(sessionId string, session *models.UpdateSession) error
}

// SeatCaller seats the caller on teamId in game gameId, records that seat on
// their session, and returns the game's keyframe taken after the seat was
// written. join and create both end this way.
//
// A caller who could not be seated keeps the session they had: a session
// naming a game whose slots it holds none of would receive that game's
// broadcasts and pass every "in this game" check.
func SeatCaller(
	games Seater,
	users UserGetter,
	sessions SessionUpdater,
	session *models.Session,
	gameId string,
	teamId string,
) (*[]byte, error) {
	if session.UserID == nil || *session.UserID == "" {
		return nil, fmt.Errorf("calling session has no user id")
	}

	userId := *session.UserID

	user, err := users.Get(userId)
	if err != nil {
		return nil, err
	}

	displayName := user.Name
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}

	slotId, err := games.ConnectPlayer(gameId, teamId, userId, displayName)
	if err != nil {
		return nil, fmt.Errorf("seating player %v in game %v: %w", userId, gameId, err)
	}

	keyframe, err := games.GetJSONBytes(gameId)
	if err != nil {
		return nil, err
	}

	if err := sessions.Update(session.ID, &models.UpdateSession{
		GameID:    gameId,
		UserID:    userId,
		TeamID:    teamId,
		SlotID:    slotId,
		UpdatedAt: time.Now(),
	}); err != nil {
		return keyframe, err
	}

	return keyframe, nil
}

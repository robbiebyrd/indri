package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
)

// Seater is the part of the game service that seats a player, frees the seat
// they leave behind, and renders the game they were seated in.
type Seater interface {
	ConnectPlayer(id string, teamId string, userId string, displayName string) (string, error)
	GetJSONBytes(id string) (*[]byte, error)
	SeatFreer
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

	// A player sits in one game at a time. Only now that the new seat is theirs
	// is the old one given up, so a refused join costs them nothing. A seat that
	// is already gone has nothing to give up; any other failure is reported once
	// the session has followed the player to the new game, since they would
	// otherwise keep a seat in the game they left.
	var freeErr error

	if session.GameID != nil && *session.GameID != gameId && session.SlotID != nil {
		if err := VacateSeat(games, session); err != nil && !errors.Is(err, gameRepo.ErrSlotNotHeld) {
			freeErr = fmt.Errorf("seated in game %v, but could not free the seat in game %v: %w", gameId, *session.GameID, err)
		}
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
		return keyframe, errors.Join(err, freeErr)
	}

	return keyframe, freeErr
}

// HoldsSeat reports whether session's user still holds the slot the session
// names in g. A session can outlive its seat — the player left from another
// connection, or was kicked — so anything acting for a player checks this
// rather than trusting the session alone.
func HoldsSeat(g *models.Game, session *models.Session) bool {
	if session.SlotID == nil || session.UserID == nil || *session.UserID == "" {
		return false
	}

	player, ok := g.Players[*session.SlotID]

	return ok && player.UserID == *session.UserID
}

// GameLeaver takes a session out of a game.
type GameLeaver interface {
	LeaveGame(sessionId string, gameId string) error
}

// DropLostSeat takes session out of g when its user no longer holds the seat it
// names there, and reports whether they still hold it. A session left naming a
// game whose seat it lost would keep receiving that game's broadcasts.
func DropLostSeat(sessions GameLeaver, session *models.Session, g *models.Game) (bool, error) {
	if HoldsSeat(g, session) {
		return true, nil
	}

	return false, sessions.LeaveGame(session.ID, g.ID)
}

// SeatFreer frees a player's slot.
type SeatFreer interface {
	RemovePlayer(id string, slotId string, userId string) error
}

// VacateSeat frees the slot session names in its game. It fails if the
// session's user no longer holds that slot: someone else's seat is not theirs
// to free.
func VacateSeat(games SeatFreer, session *models.Session) error {
	if session.GameID == nil || session.SlotID == nil || session.UserID == nil {
		return fmt.Errorf("session %v holds no seat", session.ID)
	}

	return games.RemovePlayer(*session.GameID, *session.SlotID, *session.UserID)
}

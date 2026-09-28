package utils

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	"github.com/robbiebyrd/indri/internal/transport"
)

// RequireHost checks that the caller on c hosts the game with gameCode, and
// returns the game and the caller's session. The caller is resolved from their
// own connection, never from anything in the message, and found in the game by
// their slot.
func RequireHost(i *injector.Injector, c transport.Conn, gameCode string) (*models.Game, *models.Session, error) {
	sessionId, err := connection.NewService(c, i.Transport).GetKeyAsString("sessionId")
	if err != nil {
		return nil, nil, fmt.Errorf("must be logged in: %w", err)
	}

	session, err := i.SessionService.Get(*sessionId)
	if err != nil {
		return nil, nil, fmt.Errorf("could not resolve calling session: %w", err)
	}

	if session.SlotID == nil || *session.SlotID == "" {
		return nil, nil, fmt.Errorf("calling session has no slot id")
	}

	g, err := i.GameService.GetByCode(gameCode)
	if err != nil {
		return nil, nil, err
	}

	if session.GameID == nil || *session.GameID != g.ID {
		return nil, nil, fmt.Errorf("caller is not in game %v", gameCode)
	}

	if slot := *session.SlotID; !g.Players[slot].Host {
		return nil, nil, fmt.Errorf("caller %v is not the host of game %v", slot, gameCode)
	}

	return g, session, nil
}

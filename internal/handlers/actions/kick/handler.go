package kick

import (
	"fmt"
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle processes a kick request and removes a player from a game if the requesting player is host.
func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.MelodyClient)

	gameCode, err := handlerUtils.RequireGameCode(decodedMsg)
	if err != nil {
		return err
	}

	userId, ok := decodedMsg["userId"].(string)
	if !ok {
		return fmt.Errorf("userId to kick must be provided")
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	gameId := g.ID.Hex()

	session, err := h.i.SessionService.GetByUserID(userId)
	if err != nil {
		return err
	}

	sessionId := session.ID.Hex()

	if *session.GameID != gameId {
		return fmt.Errorf("attempt to kick %v from game %v because player %v "+
			"isn't in the same game", userId, *gameCode, *session.UserID)
	}

	if !g.Players[*session.UserID].Host {
		return fmt.Errorf("attempt to kick %v from game %v failed because %v "+
			"isn't the game host", userId, *gameCode, *session.UserID)
	}

	userConnection, err := cs.Get(&sessionId)
	if err != nil {
		return err
	}

	err = h.i.GameService.RemovePlayer(*gameCode, userId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", userId, gameCode, err)
	}

	entrypoints.HandleDisconnect(userConnection, h.i.MelodyClient, h.i.GameService, h.i.SessionService)

	return nil
}

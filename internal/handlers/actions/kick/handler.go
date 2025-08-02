package kick

import (
	"fmt"
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/session"
	"log"
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
	ss := session.NewService(s, h.i.MelodyClient)

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(decodedMsg)
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

	actorGameId, _, actorPlayerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	if *actorGameId != gameId {
		return fmt.Errorf("attempt to kick %v from game %v because player %v "+
			"isn't in the same game", userId, *gameCode, *actorPlayerId)
	}

	if !g.Players[*actorPlayerId].Host {
		return fmt.Errorf("attempt to kick %v from game %v failed because %v "+
			"isn't the game host", userId, *gameCode, *actorPlayerId)
	}

	userSession, err := ss.Get(&gameId, teamId, &userId)
	if err != nil {
		return err
	}

	err = h.i.GameService.RemovePlayer(*gameCode, userId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", userId, gameCode, err)
	}

	entrypoints.HandleDisconnect(userSession, h.i.MelodyClient, h.i.GameService)

	return nil
}

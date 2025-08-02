package join

import (
	"encoding/json"
	"fmt"
	"github.com/olahol/melody"
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

// Handle processes a join game request, and adds a player to a game.
func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	ss := session.NewService(s, h.i.MelodyClient)

	gameCode, teamId := utils.ParseGameCodeAndTeamID(decodedMsg)
	if gameCode == nil {
		return fmt.Errorf("game code not provided")
	}

	userId, err := ss.GetKeyAsString("userId")
	if err != nil {
		_ = s.Write([]byte(`{"authenticated": false, "stage": { "currentScene": "login"}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	gameId := g.ID.Hex()

	user, err := h.i.UserService.Get(*userId)
	if err != nil {
		return err
	}

	displayName := user.Name
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}

	err = h.i.GameService.ConnectPlayer(gameId, *teamId, *userId, displayName)
	if err != nil {
		log.Printf("error adding player %v to game %v: %v\n", *userId, *gameCode, err)
	}

	g, err = h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	gameJSONBytes, err := json.Marshal(h.i.GameService.Sanitize(g))
	if err != nil {
		return err
	}

	err = s.Write(gameJSONBytes)
	if err != nil {
		return err
	}

	err = ss.SetStandardKeys(&gameId, nil, userId)
	if err != nil {
		return err
	}

	return nil
}

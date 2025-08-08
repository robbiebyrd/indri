package create

import (
	"fmt"
	"log"
	"time"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
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
	cs := connection.NewService(s, h.i.MelodyClient)

	fmt.Println(decodedMsg)

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(decodedMsg)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("game code not provided")
	}

	gamePrivate := true

	if _, ok := decodedMsg["private"]; !ok {
		gamePrivate = false
	}

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		_ = cs.Write([]byte(`{"authenticated": false, "stage": { "currentScene": "login"}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return err
	}

	g, err := h.i.GameService.New(*gameCode, gamePrivate)
	if err != nil {
		return err
	}

	user, err := h.i.UserService.Get(*session.UserID)
	if err != nil {
		return err
	}

	displayName := user.Name
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}

	err = h.i.GameService.ConnectPlayer(g.ID.Hex(), *teamId, *session.UserID, displayName)
	if err != nil {
		log.Printf("error adding player %v to game %v: %v\n", *session.UserID, *gameCode, err)
	}

	gameJSONBytes, err := h.i.GameService.GetJSONBytes(g.ID.Hex())
	if err != nil {
		return err
	}

	err = cs.Write(*gameJSONBytes)
	if err != nil {
		return err
	}

	if err = h.i.SessionService.Update(*sessionId, &models.UpdateSession{
		GameID:    g.ID.Hex(),
		UserID:    *session.UserID,
		TeamID:    *teamId,
		UpdatedAt: time.Time{},
	}); err != nil {
		return err
	}

	return nil
}

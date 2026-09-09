package join

import (
	"fmt"
	"log"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle adds the caller to an existing game.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{
			Responses: [][]byte{[]byte(`{"authenticated": false, "stage": { "currentScene": "login"}}`)},
		}, fmt.Errorf("not authenticated")
	}

	session := req.Session

	gameCode, teamId := utils.ParseGameCodeAndTeamID(req.Payload)
	if gameCode == nil {
		return actions.Result{}, fmt.Errorf("game code not provided")
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	user, err := h.i.UserService.Get(*session.UserID)
	if err != nil {
		return actions.Result{}, err
	}

	displayName := user.Name
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}

	if err = h.i.GameService.ConnectPlayer(g.ID.Hex(), *teamId, *session.UserID, displayName); err != nil {
		log.Printf("error adding player %v to game %v: %v\n", *session.UserID, *gameCode, err)
	}

	gameJSONBytes, err := h.i.GameService.GetJSONBytes(g.ID.Hex())
	if err != nil {
		return actions.Result{}, err
	}

	result := actions.Result{Responses: [][]byte{*gameJSONBytes}}

	if err = h.i.SessionService.Update(session.ID.Hex(), &models.UpdateSession{
		GameID:    g.ID.Hex(),
		UserID:    *session.UserID,
		TeamID:    *teamId,
		UpdatedAt: time.Time{},
	}); err != nil {
		return result, err
	}

	return result, nil
}

package create

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle creates a game and adds the caller to it.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{
			Responses: [][]byte{[]byte(`{"authenticated": false, "stage": { "currentScene": "login"}}`)},
		}, fmt.Errorf("not authenticated")
	}

	session := req.Session

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(req.Payload)
	if err != nil {
		return actions.Result{}, fmt.Errorf("game code not provided: %w", err)
	}

	gamePrivate, _ := req.Payload["private"].(bool)

	g, err := h.i.GameService.New(*gameCode, gamePrivate)
	if err != nil {
		return actions.Result{}, err
	}

	keyframe, err := utils.SeatCaller(h.i.GameService, h.i.UserService, h.i.SessionService, session, g.ID, *teamId)
	if keyframe == nil {
		return actions.Result{}, err
	}

	return actions.Result{Responses: [][]byte{*keyframe}}, err
}

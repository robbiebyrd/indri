package join

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

// Handle adds the caller to an existing game.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{
			Responses: [][]byte{[]byte(`{"authenticated": false, "stage": { "currentScene": "login"}}`)},
		}, fmt.Errorf("not authenticated")
	}

	gameCode, teamId, err := utils.RequireGameCodeAndTeamID(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	keyframe, err := utils.SeatCaller(h.i.GameService, h.i.UserService, h.i.SessionService, req.Session, g.ID, *teamId,
		utils.HeldLayoutHash(req.Payload))
	if keyframe == nil {
		return actions.Result{}, err
	}

	return actions.Result{Responses: [][]byte{*keyframe}}, err
}

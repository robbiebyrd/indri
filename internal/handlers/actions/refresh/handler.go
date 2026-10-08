package refresh

import (
	"errors"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{Responses: [][]byte{models.ErrServerError.BytesError()}}, nil
	}

	session := req.Session

	if session.GameID == nil || *session.GameID == "" {
		return actions.Result{Responses: [][]byte{models.ErrNoGame.BytesError()}}, errors.New(models.ErrNoGame.Description())
	}

	g, err := h.i.GameService.Get(*session.GameID)
	if err != nil {
		return actions.Result{Responses: [][]byte{models.ErrGameNotFound.BytesError()}}, err
	}

	// A keyframe is the whole game, for its players only: a session that
	// outlived its seat is no longer one.
	if !handlerUtils.HoldsSeat(g, session) {
		return actions.Result{Responses: [][]byte{models.ErrNoGame.BytesError()}}, errors.New(models.ErrNoGame.Description())
	}

	jsonData, err := h.i.GameService.Keyframe(g, handlerUtils.HeldLayoutHash(req.Payload))
	if err != nil {
		return actions.Result{Responses: [][]byte{models.ErrServerError.BytesError()}}, err
	}

	return actions.Result{Responses: [][]byte{jsonData}}, nil
}

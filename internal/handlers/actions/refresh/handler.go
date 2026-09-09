package refresh

import (
	"encoding/json"
	"errors"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
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

	jsonData, err := json.Marshal(h.i.GameService.Sanitize(g))
	if err != nil {
		return actions.Result{Responses: [][]byte{models.ErrServerError.BytesError()}}, err
	}

	return actions.Result{Responses: [][]byte{jsonData}}, nil
}

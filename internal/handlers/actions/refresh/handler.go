package refresh

import (
	"encoding/json"
	"errors"

	"github.com/olahol/melody"

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

func (h *Handler) Handle(
	s *melody.Session,
	_ map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.MelodyClient)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		_ = cs.Write(models.ErrServerError.BytesError())
		return nil
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		_ = cs.WriteError(models.ErrSessionNotFound)
		return nil
	}

	if session.GameID == nil || *session.GameID == "" {
		_ = cs.Write(models.ErrNoGame.BytesError())
		return errors.New(models.ErrNoGame.Description())
	}

	g, err := h.i.GameService.Get(*session.GameID)
	if err != nil {
		_ = cs.WriteError(models.ErrGameNotFound)
		return err
	}

	jsonData, err := json.Marshal(h.i.GameService.Sanitize(g))
	if err != nil {
		_ = cs.WriteError(models.ErrServerError)
		return err
	}

	_ = cs.Write(jsonData)

	return nil
}

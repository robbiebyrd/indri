package refresh

import (
	"encoding/json"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/session"
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
	ss := session.NewService(s, h.i.MelodyClient)

	gameId, err := ss.GetKeyAsString("gameId")
	if err != nil {
		_ = s.Write(models.ErrServerError.BytesError())
		return nil
	}

	g, err := h.i.GameService.Get(*gameId)
	if err != nil {
		_ = s.Write(models.ErrGameNotFound.BytesError())
	}

	jsonData, err := json.Marshal(g)
	if err != nil {
		_ = s.Write(models.ErrServerError.BytesError())
	}

	_ = ss.Write(jsonData)

	return nil
}

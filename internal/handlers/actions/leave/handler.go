package leave

import (
	"fmt"
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/injector"
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
	ss := connection.NewService(s, h.i.MelodyClient)

	gameId, _, playerId, err := ss.GetStandardKeys()
	if err != nil {
		return err
	}

	g, err := h.i.GameService.Get(*gameId)
	if err != nil {
		return err
	}

	if *gameId != g.ID.Hex() {
		return fmt.Errorf("player is in game %v but asking to leave game %v", *gameId, g.Code)
	}

	err = h.i.GameService.RemovePlayer(g.ID.Hex(), *playerId)
	if err != nil {
		log.Printf("could not disconnect player %v from game %v: %v\n", playerId, gameId, err)
	}

	return nil
}

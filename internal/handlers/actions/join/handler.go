package join

import (
	"fmt"
	"log"
	"time"

	"github.com/robbiebyrd/indri/internal/transport"

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
	s transport.Conn,
	decodedMsg map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.Transport)

	gameCode, teamId := utils.ParseGameCodeAndTeamID(decodedMsg)
	if gameCode == nil {
		return fmt.Errorf("game code not provided")
	}

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		_ = cs.Write([]byte(`{"authenticated": false, "stage": { "currentScene": "login"}}`))
		return fmt.Errorf("unable to get userId: %w", err)
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return err
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
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

	slotId, err := h.i.GameRepo.AssignSlot(g.ID, *teamId, *session.UserID, displayName)
	if err != nil {
		log.Printf("error assigning slot for player %v in game %v: %v\n", *session.UserID, *gameCode, err)
		return err
	}

	fresh, err := h.i.GameService.Get(g.ID)
	if err != nil {
		return err
	}

	if err = h.i.GameService.WriteKeyframe(s, fresh, h.i.LayoutHash, h.i.LayoutData); err != nil {
		return err
	}

	if err = h.i.SessionService.Update(*sessionId, &models.UpdateSession{
		GameID:    g.ID,
		UserID:    *session.UserID,
		TeamID:    *teamId,
		SlotID:    slotId,
		UpdatedAt: time.Now(),
	}); err != nil {
		return err
	}

	return nil
}

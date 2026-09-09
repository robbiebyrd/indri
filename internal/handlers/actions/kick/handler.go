package kick

import (
	"fmt"
	"log"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/connection"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle processes a kick request and removes a player from a game if the requesting player is host.
func (h *Handler) Handle(
	s *melody.Session,
	decodedMsg map[string]interface{},
) error {
	cs := connection.NewService(s, h.i.MelodyClient)

	gameCode, err := handlerUtils.RequireGameCode(decodedMsg)
	if err != nil {
		return err
	}

	targetUserId, ok := decodedMsg["userId"].(string)
	if !ok || targetUserId == "" {
		return fmt.Errorf("userId to kick must be provided")
	}

	// Authorize the CALLER from their own connection, never from the
	// client-supplied target userId.
	callerSessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return fmt.Errorf("must be logged in to kick a player: %w", err)
	}

	callerSession, err := h.i.SessionService.Get(*callerSessionId)
	if err != nil {
		return fmt.Errorf("could not resolve calling session: %w", err)
	}

	if callerSession.UserID == nil {
		return fmt.Errorf("calling session has no user id")
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return err
	}

	gameId := g.ID.Hex()

	if callerSession.GameID == nil || *callerSession.GameID != gameId {
		return fmt.Errorf("caller %v is not in game %v", *callerSession.UserID, *gameCode)
	}

	if !g.Players[*callerSession.UserID].Host {
		return fmt.Errorf("caller %v is not the host of game %v", *callerSession.UserID, *gameCode)
	}

	// Resolve the target and remove them from the game.
	targetSession, err := h.i.SessionService.GetByUserID(targetUserId)
	if err != nil {
		return err
	}

	if targetSession.GameID == nil || *targetSession.GameID != gameId {
		return fmt.Errorf("target %v is not in game %v", targetUserId, *gameCode)
	}

	targetSessionId := targetSession.ID.Hex()

	if err = h.i.GameService.RemovePlayer(gameId, targetUserId); err != nil {
		log.Printf("could not remove player %v from game %v: %v\n", targetUserId, gameId, err)
	}

	// Force-disconnect the target if they are currently connected. A target
	// who is offline has still been removed from the game above.
	if userConnection, err := cs.Get(&targetSessionId); err == nil {
		entrypoints.HandleDisconnect(userConnection, h.i.MelodyClient, h.i.GameService, h.i.SessionService)
	}

	return nil
}

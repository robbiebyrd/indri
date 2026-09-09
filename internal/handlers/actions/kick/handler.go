package kick

import (
	"fmt"
	"log"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
	"github.com/robbiebyrd/indri/internal/injector"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle removes a player from a game if the caller is the game's host, and
// asks the transport to force-disconnect the removed player.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if req.Session == nil {
		return actions.Result{}, fmt.Errorf("must be logged in to kick a player")
	}

	callerSession := req.Session
	if callerSession.UserID == nil {
		return actions.Result{}, fmt.Errorf("calling session has no user id")
	}

	gameCode, err := handlerUtils.RequireGameCode(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	targetUserId, ok := req.Payload["userId"].(string)
	if !ok || targetUserId == "" {
		return actions.Result{}, fmt.Errorf("userId to kick must be provided")
	}

	g, err := h.i.GameService.GetByCode(*gameCode)
	if err != nil {
		return actions.Result{}, err
	}

	gameId := g.ID.Hex()

	if callerSession.GameID == nil || *callerSession.GameID != gameId {
		return actions.Result{}, fmt.Errorf("caller %v is not in game %v", *callerSession.UserID, *gameCode)
	}

	if !g.Players[*callerSession.UserID].Host {
		return actions.Result{}, fmt.Errorf("caller %v is not the host of game %v", *callerSession.UserID, *gameCode)
	}

	targetSession, err := h.i.SessionService.GetByUserID(targetUserId)
	if err != nil {
		return actions.Result{}, err
	}

	if targetSession.GameID == nil || *targetSession.GameID != gameId {
		return actions.Result{}, fmt.Errorf("target %v is not in game %v", targetUserId, *gameCode)
	}

	if err = h.i.GameService.RemovePlayer(gameId, targetUserId); err != nil {
		log.Printf("could not remove player %v from game %v: %v\n", targetUserId, gameId, err)
	}

	// The transport force-disconnects the target if they are currently
	// connected; an offline target has still been removed above.
	return actions.Result{DisconnectIDs: []string{targetSession.ID.Hex()}}, nil
}

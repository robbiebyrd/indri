package reconnect

import (
	"bytes"
	"encoding/json"
	"fmt"

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

func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	// Refuse to resume onto a caller that is already authenticated; log out first.
	if req.Session != nil {
		return actions.Result{}, fmt.Errorf("already authenticated; log out before reconnecting")
	}

	token, ok := req.Payload["sessionId"].(string)
	if !ok || token == "" {
		return actions.Result{}, fmt.Errorf("sessionId not a string or empty string")
	}

	session, err := h.i.SessionService.GetByToken(token)
	if err != nil {
		return actions.Result{}, fmt.Errorf("could not resume session: %w", err)
	}

	if session.UserID == nil || *session.UserID == "" {
		return actions.Result{}, fmt.Errorf("session user id not a string or empty string")
	}

	user, err := h.i.UserService.Get(*session.UserID)
	if err != nil {
		return actions.Result{}, err
	}

	jsonUserBytes, err := json.Marshal(h.i.UserService.Sanitize(user))
	if err != nil {
		return actions.Result{}, err
	}

	authSuccessMessage := bytes.Join([][]byte{
		[]byte(`{"authenticated": true, "sessionId": "` + token + `", "user": `),
		jsonUserBytes,
		[]byte(`}`),
	}, []byte(""))

	responses := [][]byte{authSuccessMessage}

	if session.GameID != nil && *session.GameID != "" {
		g, err := h.i.GameService.Get(*session.GameID)
		if err != nil {
			return actions.Result{}, err
		}

		// A session that outlived its seat resumes outside the game: no keyframe
		// of a game the user is no longer in, and none of its broadcasts.
		held, err := handlerUtils.DropLostSeat(h.i.SessionService, session, g)
		if err != nil {
			return actions.Result{}, fmt.Errorf("taking session %v out of game %v: %w", session.ID, g.ID, err)
		}

		if !held {
			// Handed back as it now stands, out of the game.
			if fresh, err := h.i.SessionService.Get(session.ID); err == nil {
				session = fresh
			}

			return actions.Result{Responses: responses, Session: session}, nil
		}

		jsonGameBytes, err := h.i.GameService.Keyframe(g, handlerUtils.HeldLayoutHash(req.Payload))
		if err != nil {
			return actions.Result{}, err
		}

		responses = append(responses, jsonGameBytes)
	}

	return actions.Result{Responses: responses, Session: session}, nil
}

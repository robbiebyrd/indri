package register

import (
	"encoding/json"
	"fmt"

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
	// A session already bound to this caller means they are logged in.
	if req.Session != nil {
		return actions.Result{
			Responses: [][]byte{[]byte(`{"registered": false, "error": "user already logged in"}`)},
		}, nil
	}

	msg, err := remarshal(req.Payload)
	if err != nil {
		return actions.Result{}, err
	}

	createdUser, err := h.i.UserService.New(*msg)
	if err != nil {
		return actions.Result{}, err
	}

	return actions.Result{
		Responses: [][]byte{[]byte(fmt.Sprintf(`{"registered": true, "userId": "%s"}`, createdUser.ID))},
	}, nil
}

func remarshal(decodedMsg map[string]interface{}) (*models.CreateUser, error) {
	jsonStr, err := json.Marshal(decodedMsg)
	if err != nil {
		return nil, err
	}

	var msg models.CreateUser

	if err := json.Unmarshal(jsonStr, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

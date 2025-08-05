package models

import "encoding/json"

type WSError struct {
	ErrorCode     int    `json:"errorCode"`
	Message       string `json:"message"`
	OperationType string `json:"op"`
}

func (e *WSError) Error() string {
	return string(e.BytesError())
}

func (e *WSError) BytesError() []byte {
	a, _ := json.Marshal(e)
	return a
}

var (
	ErrServerError       = WSError{1011, "internal server error", "error"}
	ErrUnauthorized      = WSError{3000, "authentication required", "error"}
	ErrAlreadyAuthorized = WSError{1003, "user is already logged in", "error"}
	ErrNoGame            = WSError{3003, "user must join a game first", "error"}
	ErrGameNotFound            = WSError{3003, "could not find game", "error"}
)

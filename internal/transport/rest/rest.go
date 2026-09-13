// Package rest implements the client -> server half of transport.Transport as a
// JSON action API, mounted under /api. It is the inbound counterpart to the SSE
// transport, which is push-only: a client POSTs an action here and receives its
// deltas on its event stream.
//
// Each route maps to exactly one action in the shared handler registry, with the
// same payload keys the WebSocket and GraphQL transports use. It holds no
// connections of its own — the embedded Registry stays empty, so this transport
// contributes routes to the mux and nothing to fan-out.
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// prefix is the path every action route is mounted under.
const prefix = "/api/"

// SessionLookup resolves a session from its bearer token (the same opaque token
// login and reconnect issue). Satisfied by the session store.
type SessionLookup interface {
	GetByToken(token string) (*models.Session, error)
}

// Dispatcher runs an action through the shared handler registry. It is a field
// so tests can drive the transport without booting the whole injector.
type Dispatcher func(
	ctx context.Context,
	session *models.Session,
	action string,
	payload map[string]interface{},
) (actions.Result, error)

// Transport is the REST action API.
type Transport struct {
	*transport.Registry

	sessions SessionLookup

	// Dispatch runs an action. Defaults to the shared router.
	Dispatch Dispatcher
}

// New builds the REST transport. sessions authenticates bearer tokens.
func New(sessions SessionLookup) *Transport {
	return &Transport{
		Registry: &transport.Registry{},
		sessions: sessions,
		Dispatch: router.Dispatch,
	}
}

// Handle is a no-op: REST requests are dispatched per-route, not delivered as
// opaque messages on a persistent connection.
func (t *Transport) Handle(_ transport.Handlers) {}

func (t *Transport) Register(mux *http.ServeMux) {
	for action := range routes {
		mux.HandleFunc("POST "+prefix+action, t.handle)
		// Without this, a GET falls through to the catch-all below and reports
		// the action as unknown rather than as the wrong method.
		mux.HandleFunc(prefix+action, methodNotAllowed)
	}

	mux.HandleFunc(prefix+"{action}", notFound)
}

// handle authenticates the caller, decodes and validates the action's
// arguments, dispatches, and writes the action's response document back.
func (t *Transport) handle(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, prefix)

	build, ok := routes[action]
	if !ok {
		notFound(w, r)

		return
	}

	var raw map[string]interface{}

	if err := decodeBody(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	payload, err := build(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	// Authorization always comes from the caller's own bearer token, never from
	// a body field they control. An unknown token dispatches unauthenticated;
	// actions that require a session reject it themselves.
	session, _ := t.sessions.GetByToken(bearerToken(r))

	// The request's own context bounds the action: a client that hangs up mid
	// action unwinds the handler instead of leaving it on a game lock.
	result, err := t.Dispatch(r.Context(), session, action, payload)

	if len(result.DisconnectIDs) > 0 {
		t.Disconnect(result.DisconnectIDs)
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	writeJSON(w, http.StatusOK, result)
}

// decodeBody reads the request body as a JSON object. An empty body is an empty
// object, so argument-less actions can be POSTed with nothing at all.
func decodeBody(r *http.Request, into *map[string]interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, transport.MaxBodyBytes)

	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(into); err != nil {
		if errors.Is(err, io.EOF) {
			*into = map[string]interface{}{}

			return nil
		}

		return fmt.Errorf("request body is not a JSON object: %w", err)
	}

	if *into == nil {
		*into = map[string]interface{}{}
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, result actions.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// An action that changes state returns nothing: the change reaches the
	// client as a broadcast delta on its stream, not in this response.
	document := []byte("{}")
	if len(result.Responses) > 0 {
		document = result.Responses[0]
	}

	_, _ = w.Write(document)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	document, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
	if marshalErr != nil {
		document = []byte(`{"error":"unknown"}`)
	}

	_, _ = w.Write(document)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound,
		fmt.Errorf("unknown action %q", strings.TrimPrefix(r.URL.Path, prefix)))
}

func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	writeError(w, http.StatusMethodNotAllowed, errors.New("actions are POSTed"))
}

// bearerToken reads the session token from the Authorization header.
func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

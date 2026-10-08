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
	"slices"
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

	// ScriptActions lists the actions the game scripts declared, each routed
	// at POST /api/<action> with its body as the payload, bounded by
	// actions.CheckScriptPayload. Set at boot, before Register, from the
	// script engine's manifest; nil serves none.
	ScriptActions func() []string
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
	for _, action := range t.Actions() {
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

	build, ok := t.builder(action)
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

	writeJSON(w, http.StatusOK, action, result)
}

// decodeBody reads the request body as a JSON object. An empty body is an empty
// object, so argument-less actions can be POSTed with nothing at all.
// Actions is every action this transport routes: the built-ins, then the
// actions the game scripts declared.
func (t *Transport) Actions() []string {
	routed := Actions()

	for _, action := range t.scriptActions() {
		if _, builtIn := routes[action]; !builtIn {
			routed = append(routed, action)
		}
	}

	return routed
}

// builder is how action's body becomes its payload: its own allowlist for a
// built-in, the bounded whole body for a declared script action.
func (t *Transport) builder(action string) (build, bool) {
	if build, ok := routes[action]; ok {
		return build, true
	}

	if slices.Contains(t.scriptActions(), action) {
		return scriptPayload, true
	}

	return nil, false
}

func (t *Transport) scriptActions() []string {
	if t.ScriptActions == nil {
		return nil
	}

	return t.ScriptActions()
}

// scriptPayload passes a script action's body through whole, since only its
// script knows the shape, bounded to what the script engine accepts. It is a
// weaker posture than every built-in route, which drops undeclared keys, and
// the bound is what keeps it from being an unbounded one.
func scriptPayload(in map[string]interface{}) (map[string]interface{}, error) {
	if err := actions.CheckScriptPayload(in); err != nil {
		return nil, err
	}

	return in, nil
}

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

func writeJSON(w http.ResponseWriter, status int, action string, result actions.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// One request, one document: a dispatch that produced several responses can
	// only answer with the first, and FirstResponse reports the rest rather than
	// dropping them silently.
	document := transport.FirstResponse(action, result.Responses)

	// An action that changes state returns nothing: the change reaches the
	// client as a broadcast delta on its stream, not in this response.
	if document == nil {
		document = []byte("{}")
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

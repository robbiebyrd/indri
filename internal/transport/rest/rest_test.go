package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
)

// fakeSessions resolves exactly one token, standing in for the session store.
type fakeSessions struct {
	token string
	id    bson.ObjectID
}

func (f fakeSessions) GetByToken(token string) (*models.Session, error) {
	if token != f.token {
		return nil, errors.New("no such session")
	}

	return &models.Session{ID: f.id, Token: token}, nil
}

// recorder captures what the transport dispatched, and replays a canned result.
type recorder struct {
	mu sync.Mutex

	ctx     context.Context
	action  string
	payload map[string]interface{}
	session *models.Session
	calls   int

	result actions.Result
	err    error
}

func (d *recorder) dispatch(
	ctx context.Context,
	session *models.Session,
	action string,
	payload map[string]interface{},
) (actions.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls++
	d.ctx = ctx
	d.action = action
	d.payload = payload
	d.session = session

	return d.result, d.err
}

func newTestTransport(t *testing.T, sessions SessionLookup) (*Transport, *recorder, string) {
	t.Helper()

	rec := &recorder{}

	tr := New(sessions)
	tr.Dispatch = rec.dispatch

	mux := http.NewServeMux()
	tr.Register(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return tr, rec, srv.URL
}

func post(t *testing.T, url, body string, header http.Header) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sending request: %v", err)
	}

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func bearer(token string) http.Header {
	return http.Header{"Authorization": {"Bearer " + token}}
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	return strings.TrimSpace(string(b))
}

// Each route dispatches the action the WebSocket and GraphQL transports use,
// with the same payload keys. If these drift, the same client request produces
// different results depending on which transport it arrived over.
func TestRoutes_DispatchTheSameActionsAndPayloadsAsTheOtherTransports(t *testing.T) {
	cases := []struct {
		route   string
		body    string
		action  string
		payload map[string]interface{}
	}{
		{
			route:   "/api/register",
			body:    `{"email":"a@b.c","password":"pw","name":"Ada"}`,
			action:  "register",
			payload: map[string]interface{}{"email": "a@b.c", "password": "pw", "name": "Ada"},
		},
		{
			route:   "/api/login",
			body:    `{"email":"a@b.c","password":"pw"}`,
			action:  "login",
			payload: map[string]interface{}{"email": "a@b.c", "password": "pw"},
		},
		{
			// The reconnect handler reads the token from "sessionId".
			route:   "/api/reconnect",
			body:    `{"token":"tok"}`,
			action:  "reconnect",
			payload: map[string]interface{}{"sessionId": "tok"},
		},
		{
			route:   "/api/create",
			body:    `{"code":"ABCD","teamId":"red","private":true}`,
			action:  "create",
			payload: map[string]interface{}{"code": "ABCD", "teamId": "red", "private": true},
		},
		{
			// private defaults to false when omitted, matching createGame.
			route:   "/api/create",
			body:    `{"code":"ABCD","teamId":"red"}`,
			action:  "create",
			payload: map[string]interface{}{"code": "ABCD", "teamId": "red", "private": false},
		},
		{
			route:   "/api/join",
			body:    `{"code":"ABCD","teamId":"red"}`,
			action:  "join",
			payload: map[string]interface{}{"code": "ABCD", "teamId": "red"},
		},
		{
			route:   "/api/leave",
			body:    `{}`,
			action:  "leave",
			payload: map[string]interface{}{},
		},
		{
			route:   "/api/kick",
			body:    `{"code":"ABCD","userId":"u1"}`,
			action:  "kick",
			payload: map[string]interface{}{"code": "ABCD", "userId": "u1"},
		},
		{
			route:   "/api/logout",
			body:    `{}`,
			action:  "logout",
			payload: map[string]interface{}{},
		},
		{
			// The keyframe an SSE client bootstraps from.
			route:   "/api/refresh",
			body:    `{}`,
			action:  "refresh",
			payload: map[string]interface{}{},
		},
		{
			route:   "/api/inquire",
			body:    `{"inquiryType":"games"}`,
			action:  "inquire",
			payload: map[string]interface{}{"inquiryType": "games"},
		},
		{
			route:   "/api/inquire",
			body:    `{"inquiryType":"games","inquiry":"q","code":"ABCD"}`,
			action:  "inquire",
			payload: map[string]interface{}{"inquiryType": "games", "inquiry": "q", "code": "ABCD"},
		},
	}

	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}

	for _, tc := range cases {
		t.Run(tc.route+" "+tc.body, func(t *testing.T) {
			_, rec, url := newTestTransport(t, sessions)

			resp := post(t, url+tc.route, tc.body, bearer("good-token"))

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d (%s), want %d", resp.StatusCode, body(t, resp), http.StatusOK)
			}

			if rec.action != tc.action {
				t.Errorf("dispatched action = %q, want %q", rec.action, tc.action)
			}

			gotJSON, _ := json.Marshal(rec.payload)
			wantJSON, _ := json.Marshal(tc.payload)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("payload = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestRoutes_RejectMissingRequiredFields(t *testing.T) {
	cases := map[string]struct{ route, body string }{
		"register without a name":   {"/api/register", `{"email":"a@b.c","password":"pw"}`},
		"login without a password":  {"/api/login", `{"email":"a@b.c"}`},
		"reconnect without a token": {"/api/reconnect", `{}`},
		"join without a code":       {"/api/join", `{"teamId":"red"}`},
		"kick without a userId":     {"/api/kick", `{"code":"ABCD"}`},
		"inquire without a type":    {"/api/inquire", `{}`},
	}

	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, rec, url := newTestTransport(t, sessions)

			resp := post(t, url+tc.route, tc.body, bearer("good-token"))

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}

			if rec.calls != 0 {
				t.Errorf("dispatched %d times, want 0 — an invalid request reached the handlers", rec.calls)
			}
		})
	}
}

func TestRoutes_RejectAMalformedBody(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, rec, url := newTestTransport(t, sessions)

	resp := post(t, url+"/api/login", `{"email":`, bearer("good-token"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	if rec.calls != 0 {
		t.Errorf("dispatched %d times, want 0", rec.calls)
	}
}

func TestRoutes_RejectANonPostMethod(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, _, url := newTestTransport(t, sessions)

	resp, err := http.Get(url + "/api/login")
	if err != nil {
		t.Fatalf("sending request: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestRoutes_UnknownActionIsNotFound(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, rec, url := newTestTransport(t, sessions)

	resp := post(t, url+"/api/definitely-not-an-action", `{}`, bearer("good-token"))

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	if rec.calls != 0 {
		t.Errorf("dispatched %d times, want 0 — an unknown action reached the handlers", rec.calls)
	}
}

// Authorization must come from the caller's own bearer token, never from a
// field in the body they control.
func TestDispatch_ResolvesTheSessionFromTheBearerToken(t *testing.T) {
	id := bson.NewObjectID()
	sessions := fakeSessions{token: "good-token", id: id}

	t.Run("valid token", func(t *testing.T) {
		_, rec, url := newTestTransport(t, sessions)

		post(t, url+"/api/leave", `{}`, bearer("good-token"))

		if rec.session == nil {
			t.Fatal("dispatched with a nil session despite a valid token")
		}

		if rec.session.ID != id {
			t.Errorf("session ID = %v, want %v", rec.session.ID, id)
		}
	})

	t.Run("unknown token dispatches unauthenticated", func(t *testing.T) {
		_, rec, url := newTestTransport(t, sessions)

		post(t, url+"/api/leave", `{}`, bearer("bad-token"))

		if rec.session != nil {
			t.Errorf("dispatched with session %v, want nil for an unknown token", rec.session.ID)
		}
	})
}

// Actions that need no prior session must work without one; those that do are
// rejected by the handlers themselves, exactly as over WebSocket.
func TestRoutes_AllowUnauthenticatedRegisterLoginAndReconnect(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}

	for _, route := range []string{"/api/register", "/api/login", "/api/reconnect"} {
		t.Run(route, func(t *testing.T) {
			_, rec, url := newTestTransport(t, sessions)

			bodies := map[string]string{
				"/api/register":  `{"email":"a@b.c","password":"pw","name":"Ada"}`,
				"/api/login":     `{"email":"a@b.c","password":"pw"}`,
				"/api/reconnect": `{"token":"tok"}`,
			}

			resp := post(t, url+route, bodies[route], nil)

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			if rec.calls != 1 {
				t.Errorf("dispatched %d times, want 1", rec.calls)
			}
		})
	}
}

func TestDispatch_ReturnsTheActionResponseDocument(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, rec, url := newTestTransport(t, sessions)

	rec.result = actions.Result{Responses: [][]byte{[]byte(`{"authenticated":true}`)}}

	resp := post(t, url+"/api/login", `{"email":"a@b.c","password":"pw"}`, nil)

	if got, want := body(t, resp), `{"authenticated":true}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// A handler with nothing to say still needs a valid JSON body on the wire.
func TestDispatch_ReturnsAnEmptyObjectWhenTheActionHasNoResponse(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, _, url := newTestTransport(t, sessions)

	resp := post(t, url+"/api/leave", `{}`, bearer("good-token"))

	if got, want := body(t, resp), `{}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestDispatch_ReportsAHandlerErrorAsBadRequest(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	_, rec, url := newTestTransport(t, sessions)

	rec.err = errors.New("that move is not legal")

	resp := post(t, url+"/api/leave", `{}`, bearer("good-token"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	if got := body(t, resp); !strings.Contains(got, "that move is not legal") {
		t.Errorf("body = %q, want it to carry the handler error", got)
	}
}

// Kick and logout close the target's push connections, which live on the SSE
// and GraphQL transports, not on this one.
func TestDispatch_DisconnectsSessionsAcrossTransports(t *testing.T) {
	sessions := fakeSessions{token: "good-token", id: bson.NewObjectID()}
	tr, rec, url := newTestTransport(t, sessions)

	peer := &peerTransport{Registry: &transport.Registry{}}
	victim := transport.NewBufferedConn[[]byte]("kick-me", 1)
	peer.AddSink(victim)

	tr.SetPeer(transport.NewMulti(tr, peer))

	rec.result = actions.Result{DisconnectIDs: []string{"kick-me"}}

	post(t, url+"/api/kick", `{"code":"ABCD","userId":"u1"}`, bearer("good-token"))

	if !victim.IsClosed() {
		t.Error("the kicked session's connection on the peer transport was left open")
	}
}

type peerTransport struct{ *transport.Registry }

func (peerTransport) Handle(transport.Handlers) {}
func (peerTransport) Register(*http.ServeMux)   {}

// The transport must be usable inside a transport.Multi alongside the others.
var _ transport.Transport = (*Transport)(nil)

// staticHandler returns one response document, the way every built-in action
// does. Two of them registered under the same action is the multi-handler
// dispatch router.Dispatch merges — a game handler alongside a built-in, a Lua
// hook, or a received/processed pre/post hook.
type staticHandler struct{ document string }

func (h staticHandler) Handle(actions.Request) (actions.Result, error) {
	return actions.Result{Responses: [][]byte{[]byte(h.document)}}, nil
}

// syncBuffer is a log sink safe to read from the test goroutine while the
// server goroutine writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()

	buf := &syncBuffer{}
	out := log.Writer()

	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(out) })

	return buf
}

// One HTTP request answers with one document, so a dispatch that produced two
// responses can deliver only the first. That is the contract (docs/PROTOCOL.md),
// and the WebSocket transport deliberately differs — see
// TestHandleClientMessage_WritesEveryResponseOfAMultiHandlerDispatch in
// internal/services/boot. What must never happen is the drop going unreported.
func TestDispatch_MultiHandlerActionDeliversTheFirstResponseAndReportsTheRest(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_first", "logout", staticHandler{`{"first":true}`})
	router.RegisterHandler("test_logout_second", "logout", staticHandler{`{"second":true}`})

	logged := captureLog(t)

	// The real router, not the recorder: the point is that two registered
	// handlers really do merge into one Result here.
	tr := New(fakeSessions{token: "good-token", id: bson.NewObjectID()})

	mux := http.NewServeMux()
	tr.Register(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp := post(t, srv.URL+"/api/logout", `{}`, bearer("good-token"))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if got, want := body(t, resp), `{"first":true}`; got != want {
		t.Errorf("body = %q, want %q (the first response, not the merged set)", got, want)
	}

	report := logged.String()

	if !strings.Contains(report, `"logout"`) {
		t.Errorf("the dropped response was not reported (log was %q); a silent drop is "+
			"indistinguishable from a handler that never ran", report)
	}
}

// The single-response case is every action today, and it must stay exactly as
// it was: one document, and nothing logged.
func TestDispatch_SingleResponseActionIsUnchangedAndSilent(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_only", "logout", staticHandler{`{"only":true}`})

	logged := captureLog(t)

	tr := New(fakeSessions{token: "good-token", id: bson.NewObjectID()})

	mux := http.NewServeMux()
	tr.Register(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp := post(t, srv.URL+"/api/logout", `{}`, bearer("good-token"))

	if got, want := body(t, resp), `{"only":true}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}

	if report := logged.String(); report != "" {
		t.Errorf("a single-response action logged %q, want nothing", report)
	}
}

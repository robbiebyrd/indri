package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/graphql/resolvers"
	"github.com/robbiebyrd/indri/internal/transport/rest"
)

// The gameUpdates subscription is a WebSocket upgrade, and gqlgen's upgrader
// defaults to same-origin only. Without the shared policy an allowlisted
// browser clears the CORS middleware, sends its mutations happily, and then
// silently never receives a delta.
func TestRegister_SubscriptionUpgradeUsesTheOriginPolicy(t *testing.T) {
	cases := map[string]struct {
		origin     string
		sameOrigin bool
		allowed    bool
	}{
		"allowlisted cross origin": {origin: "http://localhost:8081", allowed: true},
		"same origin":              {sameOrigin: true, allowed: true},
		"no origin (native)":       {allowed: true},
		"disallowed origin":        {origin: "http://evil.example"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := New(nil)
			tr.origins = transport.NewOriginPolicy("http://localhost:8081")

			mux := http.NewServeMux()
			tr.Register(mux)

			server := httptest.NewServer(mux)
			defer server.Close()

			origin := tc.origin
			if tc.sameOrigin {
				origin = server.URL
			}

			header := http.Header{}
			if origin != "" {
				header.Set("Origin", origin)
			}

			conn, resp, err := coderws.Dial(
				context.Background(),
				"ws"+strings.TrimPrefix(server.URL, "http")+path,
				&coderws.DialOptions{HTTPHeader: header},
			)
			if conn != nil {
				defer conn.CloseNow()
			}

			// On a successful handshake the dialler returns the 101 response
			// with no body to drain.
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}

			if tc.allowed {
				if err != nil {
					t.Fatalf("Dial(origin=%q) = %v, want the upgrade to succeed", origin, err)
				}

				return
			}

			if err == nil {
				t.Fatalf("Dial(origin=%q) succeeded, want it refused", origin)
			}

			// gqlgen answers a refused upgrade itself, with 400. Served through
			// internal/entrypoints/http the CORS middleware refuses this origin
			// with 403 before the request ever reaches the transport; this is
			// the second line of defence, so what matters is that no connection
			// is established.
			if resp == nil || resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Dial(origin=%q) status = %v, want %d", origin, resp, http.StatusBadRequest)
			}
		})
	}
}

// A GraphQL operation is an HTTP body like any other, and until it was capped
// this was the one inbound surface with no limit: REST wraps its bodies in a
// MaxBytesReader and the WebSocket transport sets MaxMessageSize, so an
// oversized layout op — a script source, an opaque config — had exactly one way
// in. The cap has to refuse it before the executor parses or resolves anything,
// and has to leave an ordinary operation untouched.
func TestRegister_BodyLimit(t *testing.T) {
	// pad makes a valid { ping } operation of roughly the requested size by
	// padding an unused variable. It stays valid GraphQL, so "pong" in the
	// response means the resolver ran and the cap failed to stop it.
	pad := func(size int) string {
		body, err := json.Marshal(map[string]interface{}{
			"query":     "{ ping }",
			"variables": map[string]string{"pad": strings.Repeat("a", size)},
		})
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}

		return string(body)
	}

	cases := map[string]struct {
		body string
		// chunked sends the body without a Content-Length, so the limit can
		// only be enforced as the handler reads.
		chunked bool
		// wantStatus of 0 is not asserted: gqlgen answers a body it could not
		// read with its own error document, and the status it picks for that is
		// its business. What matters there is that no resolver ran.
		wantStatus int
		wantPong   bool
	}{
		"ordinary operation": {
			body:       pad(64),
			wantStatus: http.StatusOK,
			wantPong:   true,
		},
		"just inside the limit": {
			body:       pad(transport.MaxBodyBytes - 512),
			wantStatus: http.StatusOK,
			wantPong:   true,
		},
		"over the limit": {
			body:       pad(transport.MaxBodyBytes),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		"over the limit without a declared length": {
			body:    pad(transport.MaxBodyBytes),
			chunked: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := New(nil)

			mux := http.NewServeMux()
			tr.Register(mux)

			server := httptest.NewServer(mux)
			defer server.Close()

			req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewRequest() = %v", err)
			}

			req.Header.Set("Content-Type", "application/json")

			if tc.chunked {
				// A negative length is what net/http reads as "unknown", which
				// makes it send the body chunked.
				req.ContentLength = -1
			}

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("Do() = %v", err)
			}
			defer resp.Body.Close()

			document, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("ReadAll() = %v", err)
			}

			if tc.wantStatus != 0 && resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d (body %q)", resp.StatusCode, tc.wantStatus, document)
			}

			// The resolver returns "pong" and nothing else in this schema does,
			// so its absence is proof the operation never executed.
			if gotPong := strings.Contains(string(document), "pong"); gotPong != tc.wantPong {
				t.Errorf("response %q contains pong = %v, want %v", document, gotPong, tc.wantPong)
			}
		})
	}
}

// The subscription shares the endpoint with the mutations, so the body cap sits
// in front of its upgrade too. An upgrade carries no body, but it does need the
// ResponseWriter to stay hijackable — wrapping it would break every push client
// while leaving the mutations looking healthy.
func TestRegister_BodyLimitLeavesTheSubscriptionUpgradeAlone(t *testing.T) {
	tr := New(nil)
	tr.origins = transport.NewOriginPolicy("")

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := coderws.Dial(
		ctx,
		"ws"+strings.TrimPrefix(server.URL, "http")+path,
		&coderws.DialOptions{Subprotocols: []string{"graphql-transport-ws"}},
	)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		t.Fatalf("Dial() = %v, want the upgrade to succeed", err)
	}

	defer conn.CloseNow()

	// The handshake alone only proves the hijack worked. Completing
	// connection_init proves the socket carries traffic in both directions.
	if err := conn.Write(ctx, coderws.MessageText, []byte(`{"type":"connection_init"}`)); err != nil {
		t.Fatalf("Write(connection_init) = %v", err)
	}

	ack, err := readMessageType(ctx, conn)
	if err != nil {
		t.Fatalf("reading the reply to connection_init: %v", err)
	}

	if ack != "connection_ack" {
		t.Errorf("reply to connection_init = %q, want %q", ack, "connection_ack")
	}
}

// readMessageType reads one graphql-transport-ws frame and returns its type.
func readMessageType(ctx context.Context, conn *coderws.Conn) (string, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return "", fmt.Errorf("reading a frame: %w", err)
	}

	var message struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &message); err != nil {
		return "", fmt.Errorf("decoding %q: %w", data, err)
	}

	return message.Type, nil
}

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

// postGraphQL runs one operation against a server carrying only the registry
// the test populated, and returns the raw response document. token, when set,
// is sent as the caller's bearer credential — the one place a resolver is
// allowed to learn who is asking.
func postGraphQL(t *testing.T, sessions resolvers.SessionLookup, token, query string) []byte {
	t.Helper()

	tr := New(sessions)

	mux := http.NewServeMux()
	tr.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	body, err := json.Marshal(map[string]interface{}{"query": query})
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("NewRequest() = %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() = %v", err)
	}
	defer resp.Body.Close()

	document, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}

	return document
}

// mutate runs one anonymous operation and returns the decoded response.
func mutate(t *testing.T, query string) map[string]interface{} {
	t.Helper()

	document := postGraphQL(t, nil, "", query)

	var decoded map[string]interface{}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) = %v", document, err)
	}

	if errs, ok := decoded["errors"]; ok {
		t.Fatalf("the operation returned errors: %v", errs)
	}

	return decoded
}

// A mutation resolves to one value, so a dispatch that produced two responses
// can deliver only the first. That is the contract (docs/PROTOCOL.md), and the
// WebSocket transport deliberately differs — see
// TestHandleClientMessage_WritesEveryResponseOfAMultiHandlerDispatch in
// internal/services/boot. What must never happen is the drop going unreported.
func TestMutation_MultiHandlerActionResolvesToTheFirstResponseAndReportsTheRest(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_first", "logout", staticHandler{`{"first":true}`})
	router.RegisterHandler("test_logout_second", "logout", staticHandler{`{"second":true}`})

	logged := captureLog(t)

	decoded := mutate(t, "mutation { logout }")

	data, _ := decoded["data"].(map[string]interface{})

	want := map[string]interface{}{"first": true}
	if got := data["logout"]; !reflect.DeepEqual(got, want) {
		t.Errorf("logout = %v, want %v (the first response, not the merged set)", got, want)
	}

	report := logged.String()

	if !strings.Contains(report, `"logout"`) {
		t.Errorf("the dropped response was not reported (log was %q); a silent drop is "+
			"indistinguishable from a handler that never ran", report)
	}
}

// The single-response case is every action today, and it must stay exactly as
// it was: one document, and nothing logged.
func TestMutation_SingleResponseActionIsUnchangedAndSilent(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_logout_only", "logout", staticHandler{`{"only":true}`})

	logged := captureLog(t)

	decoded := mutate(t, "mutation { logout }")

	data, _ := decoded["data"].(map[string]interface{})

	want := map[string]interface{}{"only": true}
	if got := data["logout"]; !reflect.DeepEqual(got, want) {
		t.Errorf("logout = %v, want %v", got, want)
	}

	if report := logged.String(); report != "" {
		t.Errorf("a single-response action logged %q, want nothing", report)
	}
}

// keyframe is a sanitised game document of the shape the refresh action answers
// with: nested teams, players, scenes and free-form data blobs, and no
// privateData anywhere. Its exact bytes are the assertion below — a transport
// that re-encoded it, escaped it, or delivered only its top level would not
// reproduce them.
const keyframe = `{"id":"65f1b2c3d4e5f60718293a4b","code":"ABCD","version":7,` +
	`"data":{"round":2,"prompt":"who is the imposter?"},` +
	`"teams":{"red":{"id":"red","name":"Red","data":{"score":3}}},` +
	`"players":{"u1":{"id":"u1","name":"Ada","host":true,"connected":true,"data":{"ready":true}}},` +
	`"stage":{"currentScene":"vote","scenes":{"vote":{"id":"vote","data":{"deadline":"2026-09-13T23:19:58Z"}}}}}`

// fakeSessions resolves exactly one token, standing in for the session store.
type fakeSessions struct {
	token string
	id    string
}

func (f fakeSessions) GetByToken(token string) (*models.Session, error) {
	if token != f.token {
		return nil, errors.New("no such session")
	}

	return &models.Session{ID: f.id, Token: token}, nil
}

// sessionRecorder answers with one document and remembers the session the
// transport resolved for the caller, which is the only thing the refresh action
// has to go on when it decides which game's keyframe to hand back.
type sessionRecorder struct {
	document string

	mu      sync.Mutex
	calls   int
	session *models.Session
}

func (h *sessionRecorder) Handle(req actions.Request) (actions.Result, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.calls++
	h.session = req.Session

	return actions.Result{Responses: [][]byte{[]byte(h.document)}}, nil
}

func (h *sessionRecorder) seen() (int, *models.Session) {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.calls, h.session
}

// refreshField returns the raw bytes the refresh mutation resolved to, failing
// if the operation carried errors. Reading the field as a json.RawMessage rather
// than decoding it is deliberate: decoding and re-encoding would normalise away
// exactly the mangling this test is looking for.
func refreshField(t *testing.T, response []byte) []byte {
	t.Helper()

	var decoded struct {
		Data struct {
			Refresh json.RawMessage `json:"refresh"`
		} `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}

	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) = %v", response, err)
	}

	if len(decoded.Errors) > 0 {
		t.Fatalf("the refresh mutation returned errors: %s", decoded.Errors)
	}

	if len(decoded.Data.Refresh) == 0 {
		t.Fatalf("the refresh mutation resolved to nothing (response %q)", response)
	}

	return decoded.Data.Refresh
}

// postREST drives the equivalent REST route with the same credential, so the two
// request-response transports can be compared on the one action they both serve.
func postREST(t *testing.T, sessions rest.SessionLookup, token, action string) []byte {
	t.Helper()

	api := rest.New(sessions)

	mux := http.NewServeMux()
	api.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/"+action, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("NewRequest() = %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() = %v", err)
	}
	defer resp.Body.Close()

	document, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/%s = %d (%q), want %d", action, resp.StatusCode, document, http.StatusOK)
	}

	return bytes.TrimSpace(document)
}

// The subscription carries deltas only, so a GraphQL client that never received
// a keyframe — a reconnect resuming into an active game answers with two
// documents and a mutation resolves to the first — has nothing to replay them
// over. This is the call that gets it one, and it has to hand back the same
// document POST /api/refresh hands an SSE client in the same session: a client
// that switches transports, or a delta replay shared between them, cannot depend
// on two renderings of the same state.
//
// The action itself is stubbed because building the real one needs a game
// repository, and its sanitising is covered where it lives. What is under test
// here is the surface this story added: that the mutation reaches the refresh
// action at all, and that it delivers the action's document whole.
func TestMutation_RefreshReturnsTheSameKeyframeAsTheRestRoute(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	router.RegisterHandler("test_refresh", "refresh", staticHandler{keyframe})

	sessions := fakeSessions{token: "good-token", id: ids.New()}

	overGraphQL := refreshField(t, postGraphQL(t, sessions, "good-token", "mutation { refresh }"))
	overREST := postREST(t, sessions, "good-token", "refresh")

	if !bytes.Equal(overGraphQL, overREST) {
		t.Errorf("the refresh mutation resolved to\n\t%s\nbut POST /api/refresh answered\n\t%s\n"+
			"the same session must see the same keyframe over either transport", overGraphQL, overREST)
	}

	// Equality alone would be satisfied by both transports mangling it the same
	// way, so each is also held to the action's own bytes.
	if string(overGraphQL) != keyframe {
		t.Errorf("the refresh mutation resolved to\n\t%s\nwant the keyframe the action produced\n\t%s",
			overGraphQL, keyframe)
	}

	if string(overREST) != keyframe {
		t.Errorf("POST /api/refresh answered\n\t%s\nwant the keyframe the action produced\n\t%s",
			overREST, keyframe)
	}
}

// refresh hands back the game the caller is in, so who the caller is decides
// what they see. The mutation takes no argument naming a session, a user or a
// game (TestSchema_RefreshTakesNoArguments), which leaves the bearer token as
// the only thing that can answer it — and an unauthenticated caller must reach
// the action with no session at all rather than with somebody else's.
func TestMutation_RefreshResolvesTheCallerFromTheBearerTokenAlone(t *testing.T) {
	id := ids.New()
	sessions := fakeSessions{token: "good-token", id: id}

	cases := map[string]struct {
		token           string
		wantAuthorative bool
	}{
		"the caller's own token": {token: "good-token", wantAuthorative: true},
		"an unknown token":       {token: "not-a-token"},
		"no token at all":        {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router.Reset()
			t.Cleanup(router.Reset)

			handler := &sessionRecorder{document: keyframe}
			router.RegisterHandler("test_refresh", "refresh", handler)

			postGraphQL(t, sessions, tc.token, "mutation { refresh }")

			calls, session := handler.seen()
			if calls != 1 {
				t.Fatalf("the refresh action ran %d times, want exactly 1", calls)
			}

			if !tc.wantAuthorative {
				if session != nil {
					t.Errorf("the action was handed session %v, want none — %s authenticates nobody",
						session.ID, name)
				}

				return
			}

			if session == nil {
				t.Fatal("the action was handed no session despite a valid bearer token")
			}

			if session.ID != id {
				t.Errorf("the action was handed session %v, want %v", session.ID, id)
			}
		})
	}
}

// A gameId or sessionId argument here would let any caller ask for the keyframe
// of a game they never joined, because the action trusts what it is handed. The
// field is deliberately argument-free, and the schema is where that is enforced.
func TestSchema_RefreshTakesNoArguments(t *testing.T) {
	schema := generated.NewExecutableSchema(generated.Config{}).Schema()
	if schema.Mutation == nil {
		t.Fatal("the GraphQL schema declares no Mutation type")
	}

	field := schema.Mutation.Fields.ForName("refresh")
	if field == nil {
		t.Fatal("the GraphQL schema declares no refresh mutation, so a client cannot ask for a keyframe")
	}

	for _, arg := range field.Arguments {
		t.Errorf("the refresh mutation takes an argument %q; the caller must be resolved from their"+
			" bearer token, never from something they can name", arg.Name)
	}
}

package boot

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/transport"
	graphqlTransport "github.com/robbiebyrd/indri/internal/transport/graphql"
	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/rest"
	sseTransport "github.com/robbiebyrd/indri/internal/transport/sse"
	webrtcTransport "github.com/robbiebyrd/indri/internal/transport/webrtc"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

const actionsDir = "../../handlers/actions"

const resolversFile = "../../transport/graphql/resolvers/schema.resolvers.go"

// registeredActions runs the real registration against an injector carrying
// only a melody hub. Handler constructors just store the injector, and
// registerHandlers dereferences nothing else, so no database is needed. It
// returns each action mapped to the base package name of the handler bound to
// it, so tests can check not just presence but correct wiring.
func registeredActions(t *testing.T) map[string]string {
	t.Helper()

	// Reset first so the append-only global registry doesn't accumulate across
	// repeated runs (e.g. -count=2 or a future second test in this package).
	router.Reset()
	t.Cleanup(router.Reset)

	registerHandlers(&injector.Injector{
		ClientsInjector: &injector.ClientsInjector{Transport: ws.New()},
	})

	actions := make(map[string]string)

	for _, h := range router.RegisteredHandlers() {
		// The handler is a *<pkg>.Handler; its package base name is the action
		// package it came from.
		actions[h.Action] = path.Base(reflect.TypeOf(h.Handler).Elem().PkgPath())
	}

	return actions
}

// TestRegisterHandlers_CoversEveryActionPackage guards against an action being
// implemented but never wired into the router, which silently makes it
// unreachable from clients. Each directory under handlers/actions is one
// action, named after its package.
func TestRegisterHandlers_CoversEveryActionPackage(t *testing.T) {
	registered := registeredActions(t)

	entries, err := os.ReadDir(actionsDir)
	if err != nil {
		t.Fatalf("reading %v: %v", actionsDir, err)
	}

	found := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		found++

		if _, ok := registered[entry.Name()]; !ok {
			t.Errorf(
				"action %q is implemented in %v but not registered in registerHandlers, so clients cannot reach it",
				entry.Name(),
				filepath.Join(actionsDir, entry.Name()),
			)
		}
	}

	if found == 0 {
		t.Fatalf("found no action packages under %v; the test is not checking anything", actionsDir)
	}
}

// TestRegisterHandlers_BindsCorrectHandler guards against a mis-wiring where an
// action is registered but pointed at the wrong handler package (e.g. "kick"
// bound to login.New) — a swap that presence-only checks would miss.
func TestRegisterHandlers_BindsCorrectHandler(t *testing.T) {
	for action, handlerPkg := range registeredActions(t) {
		if action != handlerPkg {
			t.Errorf("action %q is wired to the %q handler package; expected a handler from the %q package",
				action, handlerPkg, action)
		}
	}
}

// TestRestRoutesMatchRegisteredActions guards the drift the REST action API
// costs: every action is reachable over WebSocket and GraphQL, so one that is
// missing a REST route is unreachable for SSE clients, and a REST route for an
// action nobody registered is a 404 waiting to happen.
func TestRestRoutesMatchRegisteredActions(t *testing.T) {
	registered := registeredActions(t)

	exposed := make(map[string]struct{}, len(rest.Actions()))
	for _, action := range rest.Actions() {
		exposed[action] = struct{}{}
	}

	for action := range registered {
		if _, ok := exposed[action]; !ok {
			t.Errorf(
				"action %q is registered but has no POST /api/%s route, so SSE clients cannot invoke it",
				action, action,
			)
		}
	}

	for action := range exposed {
		if _, ok := registered[action]; !ok {
			t.Errorf(
				"POST /api/%s is routed but %q is not a registered action, so it can only ever fail",
				action, action,
			)
		}
	}
}

// knownMissingFromGraphQL records actions that have no GraphQL mutation today.
// It is a list of gaps, not of exemptions: "refresh" returns a keyframe, which a
// GraphQL client needs as much as an SSE one does. Entries belong here only
// while somebody means to remove them — but an action that is in neither this
// list nor the schema fails the test, so a gap cannot open by accident.
var knownMissingFromGraphQL = map[string]string{
	"refresh": "GraphQL clients have no keyframe call and can only subscribe to deltas",
}

// TestGraphQLMutationsMatchRegisteredActions closes the gap the WebSocket
// coverage test leaves. TestRegisterHandlers_CoversEveryActionPackage guards the
// router registry only, so an action registered there but missing from
// schema.graphqls passes CI while being unreachable for every GraphQL client.
func TestGraphQLMutationsMatchRegisteredActions(t *testing.T) {
	registered := registeredActions(t)
	exposed := graphqlActions(t)

	for action := range registered {
		_, hasMutation := exposed[action]
		reason, known := knownMissingFromGraphQL[action]

		switch {
		case hasMutation && known:
			t.Errorf("action %q now has a GraphQL mutation; remove it from knownMissingFromGraphQL", action)
		case !hasMutation && !known:
			t.Errorf(
				"action %q is registered but no GraphQL mutation dispatches it, so GraphQL clients cannot invoke it",
				action,
			)
		case !hasMutation:
			t.Logf("action %q has no GraphQL mutation: %v", action, reason)
		}
	}

	for action := range exposed {
		if _, ok := registered[action]; !ok {
			t.Errorf(
				"a GraphQL mutation dispatches %q, which is not a registered action, so it can only ever fail",
				action,
			)
		}
	}
}

// graphqlActions returns every action the GraphQL API can invoke.
//
// It joins the two halves that have to agree: the compiled schema says which
// mutation fields exist, and the resolver source says which action each one
// dispatches. Reading both is what makes the parity check honest — a table
// listing them could drift from either, a schema field with no resolver would
// still compile, and a resolver dispatching the wrong action name would still
// satisfy a check that only counted fields.
func graphqlActions(t *testing.T) map[string]struct{} {
	t.Helper()

	schema := generated.NewExecutableSchema(generated.Config{}).Schema()
	if schema.Mutation == nil {
		t.Fatalf("the GraphQL schema declares no Mutation type; the test is not checking anything")
	}

	dispatched := resolverDispatches(t)
	actions := make(map[string]struct{}, len(schema.Mutation.Fields))

	for _, field := range schema.Mutation.Fields {
		// gqlgen names a resolver after its field, with an upper-case initial.
		method := strings.ToUpper(field.Name[:1]) + field.Name[1:]

		action, ok := dispatched[method]
		if !ok {
			t.Errorf("the %q mutation has no resolver dispatching an action", field.Name)

			continue
		}

		actions[action] = struct{}{}
	}

	return actions
}

// resolverDispatches maps each resolver method to the action it dispatches, by
// reading the single r.dispatch(ctx, "<action>", ...) call in its body.
func resolverDispatches(t *testing.T) map[string]string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), resolversFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %v: %v", resolversFile, err)
	}

	dispatches := make(map[string]string)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if action, ok := dispatchedAction(t, fn); ok {
			dispatches[fn.Name.Name] = action
		}
	}

	if len(dispatches) == 0 {
		t.Fatalf("found no dispatch calls in %v; the test is not checking anything", resolversFile)
	}

	return dispatches
}

func dispatchedAction(t *testing.T, fn *ast.FuncDecl) (string, bool) {
	t.Helper()

	var action string

	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "dispatch" || len(call.Args) < 2 {
			return true
		}

		literal, ok := call.Args[1].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			t.Errorf("%v dispatches a non-literal action name, which no test can follow", fn.Name.Name)

			return false
		}

		unquoted, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Errorf("reading the action name dispatched by %v: %v", fn.Name.Name, err)

			return false
		}

		action = unquoted

		return false
	})

	return action, action != ""
}

// noSessions never authenticates anyone. Every transport built below only
// needs a SessionLookup to construct; this test never drives a real
// handshake for any of them, so nothing ever calls GetByToken.
type noSessions struct{}

func (noSessions) GetByToken(string) (*models.Session, error) {
	return nil, errors.New("no such session")
}

// fakeConn is a minimal transport.Conn double carrying a session key, so a
// conn can be registered on a transport directly (via AddSink) without going
// through that transport's own handshake.
type fakeConn struct {
	*transport.Keys

	mu     sync.Mutex
	writes [][]byte
}

func newFakeConn(sessionID string) *fakeConn {
	return &fakeConn{Keys: transport.NewKeys(sessionID)}
}

func (c *fakeConn) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.writes = append(c.writes, append([]byte(nil), msg...))

	return nil
}

func (c *fakeConn) Close() error {
	c.MarkClosed()

	return nil
}

func (c *fakeConn) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([][]byte(nil), c.writes...)
}

// bareTransport is a Registry promoted to a full Transport, standing in for
// WebSocket -- mirrors the fakeTransport pattern in
// internal/transport/registry_test.go and internal/transport/webrtc/webrtc_test.go.
// WebSocket's melody-backed session cannot be faked this way (there is no
// AddSink seam without a real upgrade), so it is the one leg stood in for
// rather than built for real; GraphQL, SSE and WebRTC below are the real
// transports services.go builds, registering a conn through the exact same
// Registry.AddSink seam a real handshake on each would use.
type bareTransport struct{ *transport.Registry }

func (bareTransport) Handle(transport.Handlers) {}
func (bareTransport) Register(*http.ServeMux)   {}

// TestBroadcastReachesEveryAggregatedTransport proves criterion 5 of story
// 040-9cec at the layer that is honestly testable without a database or a
// four-protocol integration harness: a delta broadcast through the same
// transport.Multi aggregation internal/injector/services.go builds reaches a
// conn registered on each aggregated transport identically, filtered by the
// one session key every transport shares.
func TestBroadcastReachesEveryAggregatedTransport(t *testing.T) {
	sessions := noSessions{}

	wsStandIn := bareTransport{&transport.Registry{}}
	gql := graphqlTransport.New(sessions)
	sse := sseTransport.New(sessions)

	rtc, err := webrtcTransport.New(sessions, webrtcTransport.Config{UDPPort: 0})
	if err != nil {
		t.Fatalf("webrtcTransport.New: %v", err)
	}

	t.Cleanup(func() { _ = rtc.Close() })

	multi := transport.NewMulti(wsStandIn, gql, sse, rtc)

	// Mirrors services.go: everything but the WebSocket leg needs the
	// aggregate wired back in via SetPeer.
	for _, p := range []interface{ SetPeer(transport.Transport) }{gql, sse, rtc} {
		p.SetPeer(multi)
	}

	const sessionID = "player-1"

	conns := map[string]*fakeConn{
		"ws":      newFakeConn(sessionID),
		"graphql": newFakeConn(sessionID),
		"sse":     newFakeConn(sessionID),
		"webrtc":  newFakeConn(sessionID),
	}

	wsStandIn.AddSink(conns["ws"])
	gql.AddSink(conns["graphql"])
	sse.AddSink(conns["sse"])
	rtc.AddSink(conns["webrtc"])

	err = multi.BroadcastFilter([]byte("delta"), func(c transport.Conn) bool {
		id, _ := c.Get(transport.SessionIDKey)

		return id == sessionID
	})
	if err != nil {
		t.Fatalf("BroadcastFilter: %v", err)
	}

	for name, c := range conns {
		got := c.written()
		if len(got) != 1 || string(got[0]) != "delta" {
			t.Errorf("%s conn received %q, want exactly one %q", name, got, "delta")
		}
	}
}

package boot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/transport/graphql/generated"
	"github.com/robbiebyrd/indri/internal/transport/rest"
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

package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
	"github.com/robbiebyrd/indri/internal/transport/rest"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

// hookScript is the script every test below runs. It hooks both ends of "join"
// and nothing else, and each hook announces itself with indri.reply.
//
// Replying is what makes the *order* observable: router.Dispatch merges each
// handler's Responses as it goes, so the frames come back in the order the
// phases ran. Nothing else a script can do is visible to a Go test in sequence.
const hookScript = `
indri.before("join", function(req)
  indri.reply({at = "before", action = req.action, team = tostring(req.payload.team)})
end)

indri.after_action("join", function(req)
  indri.reply({at = "after", action = req.action})
end)
`

// hookInjector builds the injector the real registration runs against: a
// transport it can install handlers on, and an engine over src.
//
// No database and no services beyond the engine. Handler constructors only
// store the injector, and every test below drives either the registry or a
// dispatch whose handlers are the script's own.
func hookInjector(t *testing.T, src string) *injector.Injector {
	t.Helper()

	return &injector.Injector{
		ClientsInjector:  &injector.ClientsInjector{Transport: ws.New()},
		ServicesInjector: &injector.ServicesInjector{LuaEngine: scriptEngine(t, src)},
	}
}

// dispatchJoin runs one "join" through the real dispatcher and returns the
// "at" field of every frame the hooks replied with, in order.
//
// The Go handler standing in for the built-in join is a staticHandler, not the
// real one: join.New needs a game service and a database, and the subject here
// is the order the dispatcher runs things in rather than what joining does.
func dispatchJoin(t *testing.T, session *models.Session) ([]string, error) {
	t.Helper()

	result, err := router.Dispatch(
		context.Background(), session, "join", map[string]interface{}{"team": "reds"})

	return replyMarks(t, result), err
}

// replyMarks reads the "at" field out of every hook reply in a result, in
// order, ignoring anything else the dispatch produced.
func replyMarks(t *testing.T, result actions.Result) []string {
	t.Helper()

	var marks []string

	for _, response := range result.Responses {
		var frame map[string]interface{}

		if err := json.Unmarshal(response, &frame); err != nil {
			t.Fatalf("the frame %q is not a JSON object: %v", response, err)
		}

		if at, ok := frame["at"].(string); ok {
			marks = append(marks, at)
		}
	}

	return marks
}

// registerHooksAround wires the script's hooks the way boot does and puts a
// stand-in Go handler under the action between them.
//
// registerScriptHooks is called rather than registerHandlers because the real
// built-ins would be registered against an injector with no database: join
// would fail, and a failed action never reaches the processed phase, so the
// after-success hook would never run and the ordering test would be measuring
// the wrong thing. TestRegisterScriptHooks_WiresBothPhases below is what holds
// the registration itself to boot's own code path.
func registerHooksAround(t *testing.T, i *injector.Injector, handler actions.MessageHandler) {
	t.Helper()

	router.Reset()
	t.Cleanup(router.Reset)

	registerScriptHooks(i)

	if handler != nil {
		router.RegisterHandler("test_join", "join", handler)
	}
}

// TestHooks_RunBeforeAndAfterTheGoHandler is the whole feature in one
// assertion.
//
// The order is the contract: a before hook runs before the action's Go handler
// and an after-success hook runs after it. Both register under dispatch phases
// rather than under the action, so this is also what proves the phases were
// paired the right way round — swapping them gives "after, handler, before",
// which every other test in this file would still pass.
func TestHooks_RunBeforeAndAfterTheGoHandler(t *testing.T) {
	registerHooksAround(t, hookInjector(t, hookScript), staticHandler{`{"at":"handler"}`})

	marks, err := dispatchJoin(t, &models.Session{UserID: stringPtr("player-1")})
	if err != nil {
		t.Fatalf("dispatching join: %v", err)
	}

	want := []string{"before", "handler", "after"}
	if !slices.Equal(marks, want) {
		t.Errorf("join ran %v, want %v", marks, want)
	}
}

// TestHooks_SeeTheActionAndThePayload covers what a hook is given once it is
// reached through the dispatcher rather than through the engine directly: the
// action name it is registered under, and the arguments the client sent with
// the built-in.
func TestHooks_SeeTheActionAndThePayload(t *testing.T) {
	registerHooksAround(t, hookInjector(t, hookScript), nil)

	result, err := router.Dispatch(
		context.Background(),
		&models.Session{UserID: stringPtr("player-1")},
		"join",
		map[string]interface{}{"team": "reds"},
	)
	if err != nil {
		t.Fatalf("dispatching join: %v", err)
	}

	if len(result.Responses) != 2 {
		t.Fatalf("join produced %d frames, want one per hook", len(result.Responses))
	}

	var before map[string]interface{}
	if err := json.Unmarshal(result.Responses[0], &before); err != nil {
		t.Fatalf("the before hook's frame %q is not a JSON object: %v", result.Responses[0], err)
	}

	if before["action"] != "join" {
		t.Errorf("the before hook saw the action %v, want %q", before["action"], "join")
	}

	if before["team"] != "reds" {
		t.Errorf("the before hook saw payload.team = %v, want %q", before["team"], "reds")
	}
}

// TestHooks_AreNotFiredForOtherActions is the negative half of every assertion
// above, and the one this design could most easily get wrong.
//
// The handlers register under received and processed, which router.Dispatch
// runs for *every* message. If the per-action filter were dropped, the "join"
// hooks would fire on leave, on refresh, on login and on a message naming the
// phase itself — and a test that only checked a hooked action would keep
// passing throughout.
//
// Each case dispatches with no Go handler registered at all, so any frame that
// comes back was produced by a hook that should not have run.
func TestHooks_AreNotFiredForOtherActions(t *testing.T) {
	registerHooksAround(t, hookInjector(t, hookScript), nil)

	for name, action := range map[string]string{
		"another hookable built-in": "leave",
		"a built-in nobody hooked":  "refresh",
		"a credential action":       "login",
		"the phase a hook runs in":  "received",
		"the other phase":           "processed",
		"an action nobody declared": "move",
	} {
		t.Run(name, func(t *testing.T) {
			result, err := router.Dispatch(
				context.Background(), &models.Session{UserID: stringPtr("player-1")}, action, nil)
			if err != nil {
				t.Fatalf("dispatching %q: %v", action, err)
			}

			if marks := replyMarks(t, result); len(marks) != 0 {
				t.Errorf("dispatching %q ran the hooks on \"join\": %v", action, marks)
			}
		})
	}

	// And the control: the same registry does fire for the action that was
	// hooked, so the silence above is the filter working rather than nothing
	// being wired up at all.
	marks, err := dispatchJoin(t, &models.Session{UserID: stringPtr("player-1")})
	if err != nil {
		t.Fatalf("dispatching join: %v", err)
	}

	if want := []string{"before", "after"}; !slices.Equal(marks, want) {
		t.Fatalf("join ran %v, want %v; nothing was wired up, so the silences above prove nothing", marks, want)
	}
}

// TestHooks_ABeforeHookErrorAbortsTheChain is what makes indri.before the place
// a game refuses a move.
//
// router.Dispatch returns on the first non-nil error, so a hook that raises has
// to abort exactly as a Go handler's error does: the action's own handler never
// runs, and neither does the after-success hook. A hook that produced only a
// frame would refuse a move that then went ahead anyway — the most dangerous
// way this could be wrong, because the player would be told no and the game
// would say yes.
func TestHooks_ABeforeHookErrorAbortsTheChain(t *testing.T) {
	i := hookInjector(t, `
indri.before("join", function(req) error("spectators are closed", 0) end)
indri.after_action("join", function(req) indri.reply({at = "after"}) end)
`)

	registerHooksAround(t, i, staticHandler{`{"at":"handler"}`})

	result, err := router.Dispatch(
		context.Background(), &models.Session{UserID: stringPtr("player-1")}, "join", nil)

	if err == nil {
		t.Fatal("the before hook raised but the dispatch succeeded, so the action went ahead anyway")
	}

	if marks := replyMarks(t, result); len(marks) != 0 {
		t.Errorf("the chain carried on after the before hook raised: %v", marks)
	}

	// The player is still told why, over the one channel every transport writes
	// back: handleClientMessage only log.Printfs a dispatch error.
	if got := scriptErrorMessage(t, result); !strings.Contains(got, "spectators are closed") {
		t.Errorf("the caller was answered with %q, which does not carry the hook's own words", got)
	}
}

// TestHooks_AnAfterHookErrorIsReported keeps the second kind from being a place
// failures go to die. Nothing runs after it, so the frame and the error are all
// there is.
func TestHooks_AnAfterHookErrorIsReported(t *testing.T) {
	i := hookInjector(t, `indri.after_action("join", function(req) error("the tally broke", 0) end)`)

	registerHooksAround(t, i, staticHandler{`{"at":"handler"}`})

	result, err := router.Dispatch(
		context.Background(), &models.Session{UserID: stringPtr("player-1")}, "join", nil)

	if err == nil {
		t.Fatal("the after-success hook raised and the dispatch reported nothing")
	}

	if got := scriptErrorMessage(t, result); !strings.Contains(got, "the tally broke") {
		t.Errorf("the caller was answered with %q, which does not carry the hook's own words", got)
	}
}

// TestHooks_CannotAlterTheSessionTheGoHandlerSees is the identity boundary at
// the layer it actually matters on.
//
// router.Dispatch adopts any non-nil Result.Session as the session for the
// remaining phases and reports it back to the transport. A before hook runs
// first, so one able to hand back a session would choose who the action's own
// handler thinks is calling. The hook below does everything a script can —
// replies, reads the caller — and the handler behind it must still see the
// session the transport authenticated, by identity and not merely by value.
func TestHooks_CannotAlterTheSessionTheGoHandlerSees(t *testing.T) {
	seen := &sessionRecorder{}

	registerHooksAround(t, hookInjector(t, hookScript), seen)

	session := &models.Session{UserID: stringPtr("player-1"), GameID: stringPtr("game-1")}

	result, err := router.Dispatch(
		context.Background(), session, "join", map[string]interface{}{"team": "reds"})
	if err != nil {
		t.Fatalf("dispatching join: %v", err)
	}

	if seen.calls != 1 {
		t.Fatalf("the Go handler ran %d times, want once", seen.calls)
	}

	if seen.session != session {
		t.Errorf("the Go handler was given the session %v, want the transport's own %v", seen.session, session)
	}

	if result.Session != nil {
		t.Errorf("the dispatch reported the session %v back to the transport, which would re-bind the connection",
			result.Session)
	}
}

// sessionRecorder keeps the session the dispatcher handed it, so a test can
// assert on identity rather than on equality — two sessions for the same user
// compare equal and are not the same authority.
type sessionRecorder struct {
	calls   int
	session *models.Session
}

func (r *sessionRecorder) Handle(req actions.Request) (actions.Result, error) {
	r.calls++
	r.session = req.Session

	return actions.Result{}, nil
}

// --- what the registration may and may not create ---------------------------

// TestRegisterScriptHooks_WiresBothPhases holds boot's own registration to the
// pairing the ordering test assumes, without needing a database to drive it.
func TestRegisterScriptHooks_WiresBothPhases(t *testing.T) {
	registerHooksAround(t, hookInjector(t, hookScript), nil)

	phases := make(map[string]int)
	for _, h := range router.RegisteredHandlers() {
		phases[h.Action]++
	}

	for _, phase := range []string{"received", "processed"} {
		if phases[phase] != 1 {
			t.Errorf("%d handlers are registered under %q, want exactly one", phases[phase], phase)
		}
	}
}

// TestRegisterScriptHooks_RegistersNothingForAKindNobodyHooked is the
// no-measurable-latency promise, made structural.
//
// received runs for every inbound message on every server, so a handler
// registered there is one more registry entry and one more recover() on the
// hot path of a game that is not using hooks. The promise is kept by not
// registering rather than by returning quickly, which is the only version of it
// a test can hold.
func TestRegisterScriptHooks_RegistersNothingForAKindNobodyHooked(t *testing.T) {
	tests := map[string]struct {
		src   string
		phase string
	}{
		"only a before hook":           {src: `indri.before("join", function(req) end)`, phase: "received"},
		"only an after hook":           {src: `indri.after_action("join", function(req) end)`, phase: "processed"},
		"an action and no hook at all": {src: `indri.on("move", function(req) end)`, phase: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			registerHooksAround(t, hookInjector(t, test.src), nil)

			for _, h := range router.RegisteredHandlers() {
				if h.Action != "received" && h.Action != "processed" {
					continue
				}

				if h.Action != test.phase {
					t.Errorf("a handler is registered under %q, but nothing hooked that end", h.Action)
				}
			}

			if test.phase == "" {
				return
			}

			// The control, so "registered nothing" is not passing because
			// registration is broken outright.
			if !slices.ContainsFunc(router.RegisteredHandlers(), func(h router.Handler) bool {
				return h.Action == test.phase
			}) {
				t.Errorf("nothing is registered under %q either, so this test is not checking anything", test.phase)
			}
		})
	}
}

// TestHooks_CannotReplaceABuiltinHandler is the rule the whole hook design is
// fenced by.
//
// lua.validateRegistration refuses indri.on("join", ...) because registration is
// additive: a script claiming a built-in's name would not replace that handler,
// it would run beside it, on the built-in's payload. Hooks deliberately reach
// built-ins, so the fence has to hold somewhere else — the hook registry is one
// no dispatch can resolve against, and Engine.Actions() is what every inbound
// surface is built from. This asserts the consequence: hooking "join" leaves
// the built-in the only handler registered under "join".
func TestHooks_CannotReplaceABuiltinHandler(t *testing.T) {
	i := hookInjector(t, hookScript)

	if got := i.LuaEngine.Actions(); len(got) != 0 {
		t.Fatalf("the hook script declared the actions %v; hooking a built-in must declare none", got)
	}

	router.Reset()
	t.Cleanup(router.Reset)

	registerHandlers(i)

	var handlers []string

	for _, h := range router.RegisteredHandlers() {
		if h.Action == "join" {
			handlers = append(handlers, h.Name)
		}
	}

	if !slices.Equal(handlers, []string{"indri_join"}) {
		t.Errorf("the action \"join\" is handled by %v, want only the built-in", handlers)
	}
}

// TestHooks_CannotBeDispatchedByName closes the other route to the same thing:
// a client naming the phase a hook runs in, or the action it wraps, must not be
// able to reach the hook's closure through the engine.
func TestHooks_CannotBeDispatchedByName(t *testing.T) {
	engine := scriptEngine(t, hookScript)

	for _, action := range []string{"join", "received", "processed", "before", "after_action"} {
		_, err := engine.Invoke(context.Background(), action, actions.Request{})
		if err == nil {
			t.Errorf("dispatching %q reached a script handler", action)

			continue
		}

		if !strings.Contains(err.Error(), "no lua handler") {
			t.Errorf("dispatching %q failed with %v, want no handler registered", action, err)
		}
	}
}

// TestHooks_AddNoTransportSurface guards the drift the phase pseudo-actions
// could cause.
//
// A hook adds registry entries whose action names are "received" and
// "processed", which are not actions any transport routes.
// TestRestRoutesMatchRegisteredActions would normally catch a registered action
// with no route, but it runs against an injector with no engine and so never
// sees these — this is what says the pseudo-actions stay out of the surface a
// client can reach, and that hooking an action does not cost that action the
// route it already had.
func TestHooks_AddNoTransportSurface(t *testing.T) {
	registerHooksAround(t, hookInjector(t, hookScript), nil)

	routed := rest.Actions()

	for _, phase := range []string{"received", "processed"} {
		if slices.Contains(routed, phase) {
			t.Errorf("POST /api/%s is routed, so a client can dispatch a dispatch phase directly", phase)
		}

		// The registration really is there, so the absences above are the
		// pseudo-actions being unroutable rather than nothing being wired up.
		if !slices.ContainsFunc(router.RegisteredHandlers(), func(h router.Handler) bool {
			return h.Action == phase
		}) {
			t.Errorf("nothing is registered under %q; this test is not checking anything", phase)
		}
	}

	if !slices.Contains(routed, "join") {
		t.Error("the hooked action \"join\" lost its REST route")
	}
}

// TestHooks_LoadFailureNamesTheScript keeps a refused hook reported as what it
// is. boot builds the engine before any of this runs, so a bad hook is a boot
// failure carrying the file and line that wrote it.
func TestHooks_LoadFailureNamesTheScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.lua")

	if err := os.WriteFile(path, []byte(`indri.before("login", function(req) end)`), 0o600); err != nil {
		t.Fatalf("writing %v: %v", path, err)
	}

	engine, err := luaService.NewEngine([]string{path}, nil)
	if err == nil {
		engine.Close()
		t.Fatal("a script hooking \"login\" loaded")
	}

	for _, want := range []string{"hooks.lua", "login", "credentials"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the boot failure %q does not mention %q", err, want)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}

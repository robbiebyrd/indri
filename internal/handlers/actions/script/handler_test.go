package script

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/lua"
)

// recordingEngine stands in for the Lua engine and keeps the context it was
// invoked with, so a test can assert on the deadline the handler set rather
// than on how long a script happened to take.
type recordingEngine struct {
	ctx    context.Context
	action string
	req    actions.Request

	// err is ctx.Err() as it stood while the engine was running, which is not
	// what reading it afterwards gives: invoke cancels its own context on the way
	// out, so every recorded context reports Canceled once the call has returned
	// whatever its caller was doing.
	err error
}

func (e *recordingEngine) Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error) {
	e.ctx, e.err, e.action, e.req = ctx, ctx.Err(), action, req

	return actions.Result{}, nil
}

// panickingEngine is a host function that blew up mid-invocation.
type panickingEngine struct{}

func (panickingEngine) Invoke(context.Context, string, actions.Request) (actions.Result, error) {
	panic("a host function panicked")
}

// dispatching is the handler under test, reduced to the one thing a test can
// substitute: which engine invoke is given. It runs the same invoke the real
// Handle runs, so what the router sees is what a script action's failure looks
// like in production.
type dispatching struct {
	e      engine
	action string
}

func (d dispatching) Handle(req actions.Request) (actions.Result, error) {
	return invoke(req, d.e, d.action, InvocationTimeout)
}

// TestInvoke_BoundsEveryInvocation is the per-invocation timeout: a script gets
// a deadline whether or not the caller brought one, because the VM checks the
// context between instructions and nothing else can stop a runaway loop.
func TestInvoke_BoundsEveryInvocation(t *testing.T) {
	tests := map[string]struct {
		ctx     context.Context
		timeout time.Duration
	}{
		"a caller with no deadline of its own": {ctx: context.Background(), timeout: InvocationTimeout},
		"a request built without a context":    {ctx: nil, timeout: InvocationTimeout},
		"a short timeout":                      {ctx: context.Background(), timeout: time.Millisecond},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := &recordingEngine{}
			before := time.Now()

			if _, err := invoke(actions.Request{Context: test.ctx}, recorder, "move", test.timeout); err != nil {
				t.Fatalf("invoke: %v", err)
			}

			after := time.Now()

			deadline, ok := recorder.ctx.Deadline()
			if !ok {
				t.Fatal("the engine was invoked with no deadline, so a runaway script would never be stopped")
			}

			// Bracketed by the two clock reads around the call rather than
			// compared to one of them: the deadline is set somewhere between
			// them, so either bound alone is off by however long the call took.
			if got := deadline.Sub(after); got > test.timeout {
				t.Errorf("the engine was given %v to run in, want no more than %v", got, test.timeout)
			}

			if got := deadline.Sub(before); got < test.timeout {
				t.Errorf("the engine was given %v to run in, want at least %v", got, test.timeout)
			}
		})
	}
}

// TestInvoke_DerivesTheInvocationContextFromItsCaller is the invariant the bound
// on the whole dispatch tree rests on, and the one a reader of the caps alone
// would never guess at — see the comment on InvocationTimeout.
//
// A script-sent event is dispatched inside the invocation that queued it, so a
// hop's "caller" is the invocation above it and its context already carries the
// root client message's deadline. Laying WithTimeout over that keeps the earlier
// of the two, which is why a tree ten deep and thirty-two wide still shares one
// budget. Deriving from context.Background() here — a two-character edit, and a
// plausible one while fixing a cancellation bug — would hand every hop a fresh
// InvocationTimeout and leave 32^10 dispatches bounded by nothing.
//
// Each case below fails under exactly that substitution: three on the deadline
// the caller brought, one on a value only the caller's own chain carries.
func TestInvoke_DerivesTheInvocationContextFromItsCaller(t *testing.T) {
	type callerKey struct{}

	tests := map[string]struct {
		caller func(t *testing.T) context.Context
		check  func(t *testing.T, caller context.Context, invoked *recordingEngine)
	}{
		// The hop case, and the one the tree depends on: an invocation below the
		// root is called with a budget already shorter than InvocationTimeout, and
		// must be given that budget rather than a new one.
		"a caller whose budget is already shorter than the timeout": {
			caller: func(t *testing.T) context.Context {
				return deadlineIn(t, InvocationTimeout/4)
			},
			check: func(t *testing.T, caller context.Context, invoked *recordingEngine) {
				want, _ := caller.Deadline()

				got, ok := invoked.ctx.Deadline()
				if !ok {
					t.Fatal("the engine was invoked with no deadline at all")
				}

				if !got.Equal(want) {
					t.Errorf(
						"the engine was given a deadline %v past its caller's, so a dispatch tree "+
							"would extend its budget at every hop instead of sharing it",
						got.Sub(want),
					)
				}
			},
		},
		// The same thing one step further on: a caller whose budget has run out
		// buys its callee nothing. This is how the tree unwinds rather than
		// carrying on a level deeper.
		"a caller whose budget has already run out": {
			caller: func(t *testing.T) context.Context {
				return deadlineIn(t, -time.Second)
			},
			check: func(t *testing.T, _ context.Context, invoked *recordingEngine) {
				if !errors.Is(invoked.err, context.DeadlineExceeded) {
					t.Errorf("the engine ran with err %v, want %v", invoked.err, context.DeadlineExceeded)
				}
			},
		},
		// A client that has gone away must not leave a script running for the
		// whole timeout, and a shutdown has to reach it too.
		"a caller that went away": {
			caller: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			},
			// Read while the engine was running, never afterwards: invoke cancels
			// its own context as it returns, so a context read after the call
			// reports Canceled whether or not the caller had gone away — which is
			// what made the older version of this assertion unable to fail.
			check: func(t *testing.T, _ context.Context, invoked *recordingEngine) {
				if !errors.Is(invoked.err, context.Canceled) {
					t.Errorf("the engine ran with err %v although the caller had gone away, want %v",
						invoked.err, context.Canceled)
				}
			},
		},
		// A root client message brings no deadline of its own, so there is none to
		// compare against. What proves the parentage there is a value: a context
		// built on the background carries nothing the caller put on its own.
		"a caller with no budget of its own": {
			caller: func(*testing.T) context.Context {
				return context.WithValue(context.Background(), callerKey{}, "the caller")
			},
			check: func(t *testing.T, _ context.Context, invoked *recordingEngine) {
				if got := invoked.ctx.Value(callerKey{}); got != "the caller" {
					t.Errorf(
						"the engine was invoked with a context carrying %v for the caller's key, "+
							"want %q — this context was not built on its caller's",
						got, "the caller",
					)
				}

				if _, ok := invoked.ctx.Deadline(); !ok {
					t.Error("the engine was invoked with no deadline, so a runaway script would never be stopped")
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			caller := test.caller(t)
			recorder := &recordingEngine{}

			if _, err := invoke(actions.Request{Context: caller}, recorder, "move", InvocationTimeout); err != nil {
				t.Fatalf("invoke: %v", err)
			}

			test.check(t, caller, recorder)
		})
	}
}

// deadlineIn returns a context due in d, which may be negative for one that is
// already past.
func deadlineIn(t *testing.T, d time.Duration) context.Context {
	t.Helper()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(d))
	t.Cleanup(cancel)

	return ctx
}

// TestInvoke_PassesTheRegisteredActionThrough proves a handler can only run the
// script it was wired to. The action is fixed at registration, so a payload or
// a lifecycle phase cannot redirect it.
func TestInvoke_PassesTheRegisteredActionThrough(t *testing.T) {
	recorder := &recordingEngine{}
	req := actions.Request{Action: "move", Payload: map[string]interface{}{"cell": "1,1"}}

	if _, err := invoke(req, recorder, "pass", InvocationTimeout); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if recorder.action != "pass" {
		t.Errorf("the engine was asked to run %q, want the registered action %q", recorder.action, "pass")
	}

	if recorder.req.Payload["cell"] != "1,1" {
		t.Errorf("the engine got payload %v, want the caller's %v", recorder.req.Payload, req.Payload)
	}
}

// TestHandle_WithoutAnEngineReportsIt covers the injector the registry tests
// build: no services at all. Reaching the action then is a wiring failure, and
// it must read as one rather than panic inside a nil dereference.
func TestHandle_WithoutAnEngineReportsIt(t *testing.T) {
	tests := map[string]*injector.Injector{
		"no injector": nil,
		"no services": {},
		"services with no engine": {
			ServicesInjector: &injector.ServicesInjector{},
		},
	}

	for name, i := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := New(i, "move").Handle(actions.Request{})
			if err == nil {
				t.Fatal("a script action answered although no engine is loaded")
			}

			if !strings.Contains(err.Error(), "move") {
				t.Errorf("the error %q does not name the action that could not run", err)
			}
		})
	}
}

// TestDispatch_ContainsAPanicFromTheEngine is why the script path needs no
// recover of its own: a panic crossing out of an invocation is converted into
// an error by the router's invokeHandler, so one bad script cannot take the
// server down with it.
func TestDispatch_ContainsAPanicFromTheEngine(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	// The recover logs what it caught. Captured rather than left to spill into
	// the test output, and then asserted on: an operator finding out which
	// script took an action down is the other half of containing it.
	var logged bytes.Buffer

	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	router.RegisterHandler("lua_move", "move", dispatching{e: panickingEngine{}, action: "move"})

	_, err := router.Dispatch(context.Background(), nil, "move", nil)
	if err == nil {
		t.Fatal("a panicking script invocation dispatched without an error")
	}

	if !strings.Contains(err.Error(), "a host function panicked") {
		t.Errorf("the error %q does not report what panicked", err)
	}

	if !strings.Contains(logged.String(), "a host function panicked") {
		t.Errorf("the recovered panic was not logged; the server logged %q", logged.String())
	}
}

// --- what bounds the dispatch tree -----------------------------------------------

// TestInvocationTimeout_NamesTheCodeItsBoundDependsOn keeps the comment on
// InvocationTimeout from rotting into a lie.
//
// The bound it documents is not local: it holds only because a script's queued
// events are dispatched from inside the invocation that queued them, which is
// two functions in another package. A comment naming them is worth nothing if
// either can be renamed or moved without anybody noticing, so the names are
// asserted from both ends — the comment must still name the file and the symbol,
// and that file must still declare it.
func TestInvocationTimeout_NamesTheCodeItsBoundDependsOn(t *testing.T) {
	doc := invocationTimeoutDoc(t)

	tests := map[string]struct {
		path   string
		symbol string
		decl   string
	}{
		"the call the events of an invocation are delivered from": {
			path:   "internal/services/lua/invoke.go",
			symbol: "Engine.Invoke",
			decl:   "*Engine) Invoke(",
		},
		"the delivery that dispatches one sent event": {
			path:   "internal/services/lua/host_io.go",
			symbol: "sendEffect.deliver",
			decl:   "sendEffect) deliver(",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{test.path, test.symbol} {
				if !strings.Contains(doc, want) {
					t.Errorf("the comment on InvocationTimeout no longer names %q, which its bound rests on", want)
				}
			}

			// Four levels up from internal/handlers/actions/script.
			source, err := os.ReadFile(filepath.Join("..", "..", "..", "..", test.path))
			if err != nil {
				t.Fatalf("the comment on InvocationTimeout names %s, which cannot be read: %v", test.path, err)
			}

			if !strings.Contains(string(source), test.decl) {
				t.Errorf(
					"%s no longer declares %s; the bound on the dispatch tree moved and the comment naming it did not",
					test.path, test.symbol,
				)
			}
		})
	}
}

// invocationTimeoutDoc returns the doc comment on the InvocationTimeout
// declaration, read from the source rather than from a copy kept here.
func invocationTimeoutDoc(t *testing.T) string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing handler.go: %v", err)
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST || gen.Doc == nil {
			continue
		}

		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range value.Names {
				if name.Name == "InvocationTimeout" {
					return gen.Doc.Text()
				}
			}
		}
	}

	t.Fatal("handler.go declares no documented InvocationTimeout constant")

	return ""
}

// sharedBudget is the router a script-sent event is dispatched through, and the
// assertion about what stops a runaway tree of them.
//
// It records the deadline every hop arrived with, so "they all shared the root
// message's budget" is asserted rather than inferred from the tree having
// stopped. And it refuses any hop that arrives with a deadline of its own: under
// the derivation that cannot happen, and if it ever does, carrying on would mean
// running the 32^10 dispatches the caps alone permit to find out.
type sharedBudget struct {
	mu sync.Mutex

	// root is the deadline the root invocation was given — the one budget every
	// dispatch below it has to still be running under — and rootCtx is the
	// context carrying it, which reports afterwards whether that budget is what
	// ran out.
	root    time.Time
	rootCtx context.Context

	dispatches  int
	independent int
	byDepth     map[int]int
	deepest     int

	engine  engine
	timeout time.Duration
}

// errIndependentBudget stops the tree at the first hop that was handed a budget
// of its own, because from there on nothing bounds it.
var errIndependentBudget = errors.New("a dispatched invocation was given a deadline of its own")

// recordRoot keeps the deadline the root invocation ran under.
func (b *sharedBudget) recordRoot(ctx context.Context) {
	deadline, _ := ctx.Deadline()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.root, b.rootCtx = deadline, ctx
}

// record books one dispatch and reports whether it is still running under the
// root's budget.
func (b *sharedBudget) record(ctx context.Context, depth int) bool {
	deadline, ok := ctx.Deadline()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.dispatches++
	b.byDepth[depth]++

	if depth > b.deepest {
		b.deepest = depth
	}

	if !ok || !deadline.Equal(b.root) {
		b.independent++

		return false
	}

	return true
}

// dispatcher is what the engine sends events through: the same invoke the real
// Handle runs, so every hop derives its context exactly as production does.
func (b *sharedBudget) dispatcher() lua.Dispatcher {
	return func(
		ctx context.Context,
		session *models.Session,
		action string,
		payload map[string]interface{},
	) (actions.Result, error) {
		depth, _ := payload["d"].(float64)

		if !b.record(ctx, int(depth)) {
			return actions.Result{}, errIndependentBudget
		}

		return invoke(actions.Request{
			Context: ctx,
			Action:  action,
			Session: session,
			Payload: payload,
		}, b.engine, action, b.timeout)
	}
}

// rootBudget records the deadline the root invocation is given before handing
// the call on. The hops below it go straight to the engine, so only the root
// arrives here.
type rootBudget struct {
	inner  engine
	budget *sharedBudget
}

func (r rootBudget) Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error) {
	r.budget.recordRoot(ctx)

	return r.inner.Invoke(ctx, action, req)
}

// TestSend_MaximumFanOutAtEveryDepthStopsOnTheSharedBudget drives the worst tree
// the caps permit and names what stops it.
//
// The script sends until the host refuses it, so every invocation fans out as
// wide as maxEventFanout allows, and every event it sends runs the same script
// again. The caps alone would let that run 32 + 32^2 + ... + 32^10 times, about
// 1.1e15 dispatches; what ends it in milliseconds is that every hop derives its
// context from the one above, so the whole tree is spending a single budget.
//
// The assertions are about that mechanism rather than about the tree having
// stopped: a test that only observed termination would go on passing after the
// derivation was removed, right up until the 32^10 dispatches were reached.
func TestSend_MaximumFanOutAtEveryDepthStopsOnTheSharedBudget(t *testing.T) {
	// Short on purpose. The budget is what the tree is bounded by, so the test
	// costs whatever it is set to; the production 100ms would buy nothing here
	// but a slower test. The property under test is the derivation, which does
	// not depend on the number.
	const budget = 5 * time.Millisecond

	// The dispatches that outlive the budget fail with a deadline, and each logs
	// its correlation id. Expected output, so it is captured and then checked
	// rather than left to flood the run.
	var logged bytes.Buffer

	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// Sends until indri.send refuses it, which is maxEventFanout wide without
	// this test having to know the number, and carries its own depth so the
	// dispatcher can see how far down the tree it got.
	e := luaEngine(t, `
indri.on("move", function(req)
  local depth = (req.payload and req.payload.d or 0) + 1

  while pcall(indri.send, "move", {d = depth}) do end
end)
`)

	budgetGuard := &sharedBudget{byDepth: map[int]int{}, engine: e, timeout: budget}
	e.Dispatch = budgetGuard.dispatcher()

	if _, err := invoke(
		actions.Request{Context: context.Background(), Action: "move"},
		rootBudget{inner: e, budget: budgetGuard},
		"move",
		budget,
	); err != nil {
		t.Fatalf("the invocation that started the tree failed: %v", err)
	}

	// The mechanism, asserted directly: not one dispatch in the tree was given a
	// budget of its own to spend.
	if budgetGuard.independent != 0 {
		t.Fatalf(
			"%d of %d dispatches arrived with a deadline of their own rather than the root message's: "+
				"the tree no longer shares one budget, and the caps alone permit %d dispatches",
			budgetGuard.independent, budgetGuard.dispatches, capsPermit(),
		)
	}

	// And that the tree really was the pathological one, rather than something
	// that stopped for a reason this test would not have noticed.
	if budgetGuard.deepest < 2 || budgetGuard.byDepth[1] < 2 {
		t.Fatalf(
			"the tree ran %d dispatches, %d of them at depth 1, and reached depth %d: "+
				"it never fanned out or descended, so it proved nothing",
			budgetGuard.dispatches, budgetGuard.byDepth[1], budgetGuard.deepest,
		)
	}

	// The budget is what ran out. Had the tree finished inside it, the caps would
	// have been what bounded this run and the test would have proved nothing
	// about the deadline.
	if err := budgetGuard.rootCtx.Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"the tree ended with the root context reporting %v, want %v: "+
				"%d dispatches finished inside a %v budget, so this run says nothing about what stops a full tree",
			err, context.DeadlineExceeded, budgetGuard.dispatches, budget,
		)
	}

	// The deadline reached the interpreter itself, rather than the tree petering
	// out between invocations: that is what makes it a bound on a running script
	// and not only on the dispatching around one.
	if !strings.Contains(logged.String(), context.DeadlineExceeded.Error()) {
		t.Errorf(
			"no invocation was interrupted by the deadline; the server logged %q",
			logged.String(),
		)
	}
}

// capsPermit is how many dispatches the depth and fan-out caps allow on their
// own, which is the number the comment on InvocationTimeout says is not a bound.
func capsPermit() int64 {
	const (
		depth  = 10
		fanout = 32
	)

	total, level := int64(0), int64(1)

	for range depth {
		level *= fanout
		total += level
	}

	return total
}

// luaEngine builds a real engine over src, with no game store: these tests
// exercise indri.send, which never touches one.
func luaEngine(t *testing.T, src string) *lua.Engine {
	t.Helper()

	path := filepath.Join(t.TempDir(), "game.lua")

	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("writing the script: %v", err)
	}

	e, err := lua.NewEngine([]string{path}, nil)
	if err != nil {
		t.Fatalf("building an engine over %q: %v", path, err)
	}

	t.Cleanup(e.Close)

	return e
}

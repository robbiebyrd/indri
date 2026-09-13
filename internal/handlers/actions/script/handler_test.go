package script

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
)

// recordingEngine stands in for the Lua engine and keeps the context it was
// invoked with, so a test can assert on the deadline the handler set rather
// than on how long a script happened to take.
type recordingEngine struct {
	ctx    context.Context
	action string
	req    actions.Request
}

func (e *recordingEngine) Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error) {
	e.ctx, e.action, e.req = ctx, action, req

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
	return invoke(req, d.e, d.action, defaultTimeout)
}

// TestInvoke_BoundsEveryInvocation is the per-invocation timeout: a script gets
// a deadline whether or not the caller brought one, because the VM checks the
// context between instructions and nothing else can stop a runaway loop.
func TestInvoke_BoundsEveryInvocation(t *testing.T) {
	tests := map[string]struct {
		ctx     context.Context
		timeout time.Duration
	}{
		"a caller with no deadline of its own": {ctx: context.Background(), timeout: defaultTimeout},
		"a request built without a context":    {ctx: nil, timeout: defaultTimeout},
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

// TestInvoke_InheritsTheCallersCancellation keeps an abandoned request from
// leaving a script running for the whole timeout: the deadline is layered onto
// the caller's context, never onto the background.
func TestInvoke_InheritsTheCallersCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := &recordingEngine{}

	if _, err := invoke(actions.Request{Context: ctx}, recorder, "move", defaultTimeout); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if recorder.ctx.Err() == nil {
		t.Error("the engine was invoked with a live context although the caller had gone away")
	}
}

// TestInvoke_PassesTheRegisteredActionThrough proves a handler can only run the
// script it was wired to. The action is fixed at registration, so a payload or
// a lifecycle phase cannot redirect it.
func TestInvoke_PassesTheRegisteredActionThrough(t *testing.T) {
	recorder := &recordingEngine{}
	req := actions.Request{Action: "move", Payload: map[string]interface{}{"cell": "1,1"}}

	if _, err := invoke(req, recorder, "pass", defaultTimeout); err != nil {
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

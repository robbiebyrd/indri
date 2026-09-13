// Package script dispatches a script-declared action to the Lua engine.
//
// It is the one package under handlers/actions that is not itself an action.
// There is no "script" action: boot.registerHandlers builds one Handler per
// entry in Engine.Actions(), each bound to the action it answers for, so a game
// script's actions arrive through the same router, the same lifecycle phases
// and the same Request/Result contract as the built-ins.
package script

import (
	"context"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
)

// defaultTimeout bounds one script invocation.
//
// It is deliberately far below the 10s lease a Redis lock holds: a script that
// outlived its game's lock could run beside another instance's attempt on the
// same game, and while the version fence would still keep the write safe, both
// scripts would have run. The margin is what keeps that from happening at all.
const defaultTimeout = 100 * time.Millisecond

// engine is the only capability this handler needs: run the script registered
// for an action, under a deadline. Narrow on purpose, and satisfied by
// *lua.Engine.
type engine interface {
	Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error)
}

type Handler struct {
	i *injector.Injector

	// action is the script action this handler answers for. It is fixed at
	// registration rather than read from the request, so a handler can only
	// ever run the script it was wired to.
	action string
}

// New returns the handler for one script-declared action.
func New(i *injector.Injector, action string) *Handler {
	return &Handler{i: i, action: action}
}

// Handle runs the action's script handler with a deadline of its own.
//
// The engine is resolved here rather than in New for the same reason the layout
// handler resolves its dependencies late: registerHandlers is exercised with a
// partial injector by the registry coverage tests, and a constructor that
// dereferenced the services would panic there.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
	if h.i == nil || h.i.ServicesInjector == nil || h.i.LuaEngine == nil {
		return actions.Result{}, fmt.Errorf("the action %q is handled by a game script, but no script engine is loaded", h.action)
	}

	return invoke(req, h.i.LuaEngine, h.action, defaultTimeout)
}

// invoke bounds one call to the engine.
//
// The deadline is derived from the caller's own context, not from the
// background: a client that has already gone away must not leave a script
// running for the full timeout, and a shutdown must reach it too.
func invoke(req actions.Request, e engine, action string, timeout time.Duration) (actions.Result, error) {
	ctx, cancel := context.WithTimeout(req.Ctx(), timeout)
	defer cancel()

	return e.Invoke(ctx, action, req)
}

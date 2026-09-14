package script

import (
	"context"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/lua"
)

// hookEngine is the only capability the hook handler needs: run the script hook
// of one kind wrapped around one action, under a deadline. Narrow on purpose,
// and satisfied by *lua.Engine.
type hookEngine interface {
	InvokeHook(
		ctx context.Context,
		kind lua.HookKind,
		action string,
		req actions.Request,
	) (actions.Result, error)
}

// HookHandler runs a game script's hooks for one kind.
//
// # Why this is registered under received/processed and not under the action
//
// router.Dispatch runs three pseudo-phases per message — received, <action>,
// processed — and a handler registered under the literal names received or
// processed runs on every message. That is the only place in the dispatcher
// where "before every handler for this action" and "after every handler for
// this action" exist at all, so it is where hooks have to live:
//
//   - Registering a hook under the action's own name could not produce a
//     before hook. The registry is a flat ordered slice and every handler
//     matching the phase runs in registration order, so a hook would run before
//     the built-in only if it were registered before it — making hook order a
//     property of the boot sequence rather than of the hook.
//   - It would also put a script-supplied handler into the registry under a
//     built-in's name, which is the exact thing lua.validateRegistration
//     refuses: registration is additive, so that handler would not replace the
//     built-in, it would run alongside it. Hooks reach built-ins deliberately;
//     acquiring a built-in's *name* is not part of the deal, and keeping them
//     out of the action registry is what keeps the two apart.
//
// The cost of that choice is that this handler is invoked for every inbound
// message, hooked or not, so the first thing it does is a map lookup on the
// action and a return. The set is fixed at registration from the engine's frozen
// manifest, so the unhooked path is one lookup and no allocation — no pooled
// state is acquired and no Lua runs. boot does not register this at all when no
// script hooked anything of this kind, so a server without hooks pays nothing
// whatsoever.
type HookHandler struct {
	i *injector.Injector

	// kind is which end of the action this handler is. Fixed at registration
	// rather than derived from the phase, so a handler can only ever run the
	// kind it was wired to.
	kind lua.HookKind

	// hooked is every action some script wrapped with this kind. A set rather
	// than a slice because it is consulted on every message that reaches the
	// server.
	hooked map[string]struct{}
}

// NewHook returns the handler for one hook kind, covering the actions given.
func NewHook(i *injector.Injector, kind lua.HookKind, hooked []string) *HookHandler {
	set := make(map[string]struct{}, len(hooked))
	for _, action := range hooked {
		set[action] = struct{}{}
	}

	return &HookHandler{i: i, kind: kind, hooked: set}
}

// Handle runs this kind's hook for the dispatched action, if there is one.
//
// The engine is resolved here rather than in NewHook for the reason Handler
// resolves its own late: registerHandlers is exercised with a partial injector
// by the registry coverage tests, and a constructor that dereferenced the
// services would panic there.
func (h *HookHandler) Handle(req actions.Request) (actions.Result, error) {
	if _, hooked := h.hooked[req.Action]; !hooked {
		return actions.Result{}, nil
	}

	if h.i == nil || h.i.ServicesInjector == nil || h.i.LuaEngine == nil {
		return actions.Result{}, fmt.Errorf(
			"the action %q carries a %s script hook, but no script engine is loaded", req.Action, h.kind)
	}

	return invokeHook(req, h.i.LuaEngine, h.kind, InvocationTimeout)
}

// invokeHook bounds one call to the engine and strips the one field a hook may
// not return.
//
// The deadline is derived from the caller's own context for the reason invoke's
// is: a client that has already gone away must not leave a script running for
// the full timeout, and a shutdown must reach it too. A hook shares that budget
// with the action it wraps rather than extending it, because it runs inside the
// same dispatch.
//
// Result.Session is cleared, and that is a security boundary rather than tidying
// up. router.Dispatch adopts any non-nil Result.Session as *the* session for the
// remaining phases and reports it back to the transport, which binds the
// connection to it — that is how login establishes auth. A hook runs on a
// session the transport already authenticated, and letting one hand back a
// different session would let a game script promote its own caller. Nothing in
// the Lua engine sets the field today (an invocation's Result is assembled from
// its effect ledger, which only ever appends Responses and DisconnectIDs), so
// this is the line that keeps it that way when something eventually does.
func invokeHook(
	req actions.Request,
	e hookEngine,
	kind lua.HookKind,
	timeout time.Duration,
) (actions.Result, error) {
	ctx, cancel := context.WithTimeout(req.Ctx(), timeout)
	defer cancel()

	res, err := e.InvokeHook(ctx, kind, req.Action, req)
	res.Session = nil

	return res, err
}

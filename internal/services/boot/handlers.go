package boot

import (
	"context"
	"log"

	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/actions/create"
	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/actions/kick"
	"github.com/robbiebyrd/indri/internal/handlers/actions/layout"
	"github.com/robbiebyrd/indri/internal/handlers/actions/leave"
	"github.com/robbiebyrd/indri/internal/handlers/actions/login"
	"github.com/robbiebyrd/indri/internal/handlers/actions/logout"
	"github.com/robbiebyrd/indri/internal/handlers/actions/reconnect"
	"github.com/robbiebyrd/indri/internal/handlers/actions/refresh"
	"github.com/robbiebyrd/indri/internal/handlers/actions/register"
	"github.com/robbiebyrd/indri/internal/handlers/actions/script"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/connection"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
	"github.com/robbiebyrd/indri/internal/transport"
)

func registerHandlers(i *injector.Injector) {
	i.Transport.Handle(transport.Handlers{
		Connect: func(c transport.Conn) {
			entrypoints.HandleConnect(c, i.Transport, i.GameService, i.SessionService)
		},
		Disconnect: func(c transport.Conn) {
			entrypoints.HandleDisconnect(c, i.Transport, i.GameService, i.SessionService)
		},
		Message: func(c transport.Conn, msg []byte) {
			handleClientMessage(i, c, msg)
		},
		Error: func(c transport.Conn, err error) {
			log.Printf("client transport error: %v", err)
		},
	})

	actionToHandlerMap := []router.Handler{
		{
			Name:    "indri_refresh",
			Action:  "refresh",
			Handler: refresh.New(i),
		},
		{
			Name:    "indri_register",
			Action:  "register",
			Handler: register.New(i),
		},
		{
			Name:    "indri_create",
			Action:  "create",
			Handler: create.New(i),
		},
		{
			Name:    "indri_join",
			Action:  "join",
			Handler: join.New(i),
		},
		{
			Name:    "indri_reconnect",
			Action:  "reconnect",
			Handler: reconnect.New(i),
		},
		{
			Name:    "indri_leave",
			Action:  "leave",
			Handler: leave.New(i),
		},
		{
			Name:    "indri_kick",
			Action:  "kick",
			Handler: kick.New(i),
		},
		{
			Name:    "indri_login",
			Action:  "login",
			Handler: login.New(i),
		},
		{
			Name:    "indri_logout",
			Action:  "logout",
			Handler: logout.New(i),
		},
		{
			Name:    "indri_inquire",
			Action:  "inquire",
			Handler: inquire.New(i),
		},
		{
			Name:    "indri_layout",
			Action:  "layout",
			Handler: layout.New(i),
		},
	}

	router.RegisterHandlers(actionToHandlerMap)

	registerScriptHandlers(i)
}

// registerScriptHandlers gives every action a game script declared its own
// router registration, after the built-ins.
//
// One Handler per action rather than one dispatcher for all of them: the
// registry matches on the action name, so an action with no entry here is
// silently unreachable no matter what the script declared. That is what
// TestRegisterHandlers_CoversEveryScriptAction exists to catch.
//
// The name is prefixed to keep a script's "move" distinct from a game's own Go
// "move" — registration is additive, and both are meant to be able to run.
func registerScriptHandlers(i *injector.Injector) {
	// A server always has an engine, even with no scripts to load. The registry
	// coverage tests build a partial injector on purpose, and have no engine.
	if i.ServicesInjector == nil || i.LuaEngine == nil {
		return
	}

	for _, action := range i.LuaEngine.Actions() {
		router.RegisterHandler("lua_"+action, action, script.New(i, action))
	}

	registerScriptHooks(i)
}

// hookPhases pairs each hook kind with the dispatch phase that runs it.
//
// A slice rather than a map so registration order is the same on every boot;
// the two land in different phases, so the order does not change behaviour, but
// a registry that reorders itself between runs is not something a test or a log
// should have to allow for.
//
// The pairing is the whole mechanism. router.Dispatch runs received before any
// handler for the action and processed after all of them, so a hook needs no
// new dispatcher: it needs the two phases the dispatcher already has. That
// processed is reached only when nothing errored is why the second kind is
// named after-*success* — see lua.HookKind.
var hookPhases = []struct {
	kind  luaService.HookKind
	phase string
}{
	{kind: luaService.HookBefore, phase: "received"},
	{kind: luaService.HookAfterSuccess, phase: "processed"},
}

// registerScriptHooks wires the hooks a game script wrapped around built-in
// actions.
//
// One handler per kind, not one per hooked action: the phases these register
// under run for every message, so a handler per action would multiply the work
// every unhooked message does by the number of hooks in the game. The handler
// filters on the action itself — see script.HookHandler.
//
// A kind nothing hooked is not registered at all. That is what keeps the
// promise that hooks cost nothing when a game has none: without it every
// message on every server would walk one more registry entry and one more
// recover(), for a feature that game is not using.
func registerScriptHooks(i *injector.Injector) {
	hooked := make(map[luaService.HookKind][]string)

	for _, hook := range i.LuaEngine.Hooks() {
		hooked[hook.Kind] = append(hooked[hook.Kind], hook.Action)
	}

	for _, wiring := range hookPhases {
		hooks := hooked[wiring.kind]
		if len(hooks) == 0 {
			continue
		}

		router.RegisterHandler(
			"lua_hook_"+string(wiring.kind),
			wiring.phase,
			script.NewHook(i, wiring.kind, hooks),
		)
	}
}

// handleClientMessage bridges a message-oriented transport (WebSocket) to the
// connection-independent dispatcher: it resolves the socket's current session,
// dispatches the message, then applies the result — binding the session on the
// socket when auth is established, writing responses back, and force-closing
// any connections the action asked to disconnect (kick/logout).
func handleClientMessage(i *injector.Injector, c transport.Conn, msg []byte) {
	cs := connection.NewService(c, i.Transport)

	var session *models.Session

	if idPtr, err := cs.GetKeyAsString("sessionId"); err == nil {
		if resolved, err := i.SessionService.Get(*idPtr); err == nil {
			session = resolved
		}
	}

	// The transport's message callback carries no per-message context, so this
	// dispatch is scoped to the process: shutdown cancels it, nothing else does.
	ctx := i.GlobalContext
	if ctx == nil {
		ctx = context.Background()
	}

	result, dispatchErr := router.DispatchMessage(ctx, session, msg)

	if result.Session != nil {
		cs.SetKey("sessionId", result.Session.ID)
	}

	for _, response := range result.Responses {
		if err := cs.Write(response); err != nil {
			log.Printf("error writing response: %v", err)
		}
	}

	for _, id := range result.DisconnectIDs {
		if target, err := cs.Get(&id); err == nil {
			entrypoints.HandleDisconnect(target, i.Transport, i.GameService, i.SessionService)
		}
	}

	if dispatchErr != nil {
		log.Printf("error handling message: %v", dispatchErr)
	}
}

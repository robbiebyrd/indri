package lua

import (
	"context"
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// Invoke runs the script handler registered for action.
//
// Everything happens on one borrowed state, and in this order, because none of
// it is portable between states: the handler closure is looked up on the state
// that built it, the request is marshalled into that same state's tables, and
// only then is the call made. The state goes back to the pool when the call
// returns — or is closed instead, when the call was interrupted rather than
// unwound — so nothing produced here may outlive the borrow.
//
// ctx carries the invocation's deadline, and is the only thing that stops a
// runaway script: the VM checks it between instructions. The caller sets it per
// invocation (internal/handlers/actions/script does), because the deadline
// belongs to the request that is waiting rather than to the engine.
func (e *Engine) Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error) {
	s, err := e.pool.acquire()
	if err != nil {
		return actions.Result{}, err
	}

	defer e.pool.release(s)

	h, err := handlersFor(s.L)
	if err != nil {
		return actions.Result{}, err
	}

	fn, ok := h.lookup(action)
	if !ok {
		// The router is only ever given the actions Actions() declared, and
		// every state is held to that same manifest, so reaching this is a
		// wiring failure and not a client's mistake.
		return actions.Result{}, fmt.Errorf("no lua handler is registered for the action %q", action)
	}

	arg, err := requestToLua(s.L, action, req)
	if err != nil {
		return actions.Result{}, fmt.Errorf("preparing the %q request for lua: %w", action, err)
	}

	// Installed before the call and cleared after it, so indri.mutate can find
	// this caller's game, deadline and store, and so the next invocation on this
	// pooled state cannot find them. The game id comes from the session the
	// transport authenticated; a script never names the game it edits.
	setInvocation(s.L, &invocation{ctx: ctx, gameID: gameIDOf(req), games: e.games})
	defer clearInvocation(s.L)

	// call installs the per-invocation environment, applies ctx and marks the
	// state spoiled if the interpreter was interrupted rather than unwound.
	if _, err := s.call(ctx, fn, arg); err != nil {
		return actions.Result{}, fmt.Errorf("running the lua handler for %q: %w", action, err)
	}

	// A script has no way to answer its caller yet: replying is a host function
	// rather than a return value, so the handler's result is deliberately
	// ignored here and the dispatcher is told nothing happened.
	return actions.Result{}, nil
}

// gameIDOf is the game a script's edits land on: the one the caller's own
// session says they are in.
//
// Taken from the authenticated session and never from the payload, for the same
// reason kick resolves its caller that way — a client-supplied game id would let
// any player's script edit any game. An unauthenticated caller, or one who has
// not joined, yields "" and indri.mutate refuses.
func gameIDOf(req actions.Request) string {
	if req.Session == nil || req.Session.GameID == nil {
		return ""
	}

	return *req.Session.GameID
}

// requestToLua renders one dispatched request as the table a handler receives.
//
// It is built field by field rather than marshalled from the whole
// actions.Request, because what a script may see is a decision and not a
// serialization. The session arrives here already authenticated by the
// transport, and a script reads it: it never supplies one.
func requestToLua(L *lua.LState, action string, req actions.Request) (lua.LValue, error) {
	payload, err := toLua(L, req.Payload)
	if err != nil {
		return nil, err
	}

	tbl := L.NewTable()
	tbl.RawSetString("action", lua.LString(action))
	tbl.RawSetString("payload", payload)

	// Absent rather than empty when the caller is unauthenticated, so a script
	// asking who called has to say what it means to have no answer.
	if req.Session != nil {
		tbl.RawSetString("session", sessionToLua(L, req.Session))
	}

	return tbl, nil
}

// sessionToLua renders the caller's identity, and only that.
//
// Two fields of models.Session are deliberately missing. Token is the bearer
// token a client resumes with, and ID is the key every targeted broadcast
// filters connections on; neither is a script's to read, and neither is needed
// to decide whether the caller may make a move.
func sessionToLua(L *lua.LState, session *models.Session) *lua.LTable {
	tbl := L.NewTable()

	for key, value := range map[string]*string{
		"userId": session.UserID,
		"gameId": session.GameID,
		"teamId": session.TeamID,
	} {
		if value != nil {
			tbl.RawSetString(key, lua.LString(*value))
		}
	}

	return tbl
}

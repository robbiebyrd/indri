package lua

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"

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
//
// # Which channel carries which failure
//
// The error return is for a *host* failure: no state to run on, no handler
// registered for the action, a payload with no Lua form. Those mean the server
// is misconfigured or the router and the manifest have drifted apart, and the
// operator is who needs to hear about them.
//
// A *script's own* failure does not go there. It is packed into Responses as a
// models.WSError and the error return stays nil, because the three transports do
// not agree about a Go error: boot.handleClientMessage only log.Printfs a
// dispatch error, so a WebSocket player would see nothing at all, while the REST
// and GraphQL callers of the same action would each get the raw Go error string.
// Responses is the one channel every transport writes back, which makes it the
// only place a script error reaches every player the same way.
func (e *Engine) Invoke(ctx context.Context, action string, req actions.Request) (actions.Result, error) {
	// The game id comes from the session the transport authenticated, or — for a
	// timer, which has no session — from the entry the scheduler stamped it on;
	// a script never names the game it edits.
	return e.run(ctx, actionTrigger{action: action}, req.Session, req.GameID(), func(L *lua.LState) (lua.LValue, error) {
		return requestToLua(L, action, req)
	})
}

// argument builds the single table a handler is called with, on the state that
// will run it.
//
// A closure rather than a prepared value because nothing built on one state is
// portable to another: the state is borrowed inside run, so the table can only
// be made once run knows which one it got.
type argument func(L *lua.LState) (lua.LValue, error)

// run is one call to a script handler, whatever kind of call it is.
//
// Everything that differs between an action and a lifecycle event is asked of
// the trigger — which registry holds the handler, what to call this in a
// message, who is owed a script's failure — so that adding a kind is a type
// here rather than a condition in the middle of this function. See trigger.
func (e *Engine) run(
	ctx context.Context,
	t trigger,
	session *models.Session,
	gameID string,
	arg argument,
) (actions.Result, error) {
	s, err := e.pool.acquire()
	if err != nil {
		return actions.Result{}, err
	}

	defer e.pool.release(s)

	h, err := handlersFor(s.L)
	if err != nil {
		return actions.Result{}, err
	}

	fn, ok := t.lookup(h)
	if !ok {
		// The router is only ever given the actions Actions() declared, every
		// state is held to that same manifest, and an event is only raised when
		// the manifest says something subscribed — so reaching this is a wiring
		// failure and not a client's mistake.
		return actions.Result{}, fmt.Errorf("no lua handler is registered for %s", t.describe())
	}

	value, err := arg(s.L)
	if err != nil {
		return actions.Result{}, fmt.Errorf("preparing %s for lua: %w", t.describe(), err)
	}

	// Installed before the call and cleared after it, so indri.mutate can find
	// this caller's game, deadline and store, and so the next invocation on this
	// pooled state cannot find them.
	inv := &invocation{
		ctx:      ctx,
		trigger:  t,
		gameID:   gameID,
		games:    e.games,
		session:  session,
		dispatch: e.Dispatch,
		timers:   e.Timers,
	}

	setInvocation(s.L, inv)
	defer clearInvocation(s.L)

	// call installs the per-invocation environment, applies ctx and marks the
	// state spoiled if the interpreter was interrupted rather than unwound.
	if _, err := s.call(ctx, fn, value); err != nil {
		// The ledger is dropped with the invocation. A handler that unwound left
		// the game as it found it — indri.mutate returns its callback's error
		// rather than writing — so the effects it queued describe a move that
		// never happened, and the failure is the only honest answer to its caller.
		return t.failed(err)
	}

	// The handler has returned, so everything it queued is now owed to somebody.
	// A script cannot answer its caller by returning a value — replying is a host
	// function — so this, and not the call's return value, is the result.
	var result actions.Result

	if err := inv.effects.flush(ctx, &result); err != nil {
		// Logged rather than returned, for the reason a failed publish is: the
		// write this invocation made has already committed, and a delivery hiccup
		// must not report the move itself as failed. What the caller loses is the
		// frame, which the next refresh keyframe replaces.
		log.Printf("delivering the effects of the lua handler for %s: %v", t.describe(), err)
	}

	return result, nil
}

// scriptFailure renders a script's own failure as the frame its caller is
// answered with, and logs the whole of it for the operator.
//
// what is the trigger's own description of the call, so the log line names both
// the kind and the name.
//
// The frame carries the script's file, line and message. Leaking a server-side
// path to a client would normally be out of the question; here it is the right
// call, because the "server-side" code in question is the operator's own game
// script and its author is the only person who can act on the failure. A
// correlation id alone — the usual answer — is useless to a script author who
// cannot read the server's logs, and it is what turns a game that "just stops
// working" into a typo on a line they can open.
//
// The Lua stack traceback is not in the frame. It names the host functions the
// call passed through as well as the script's own frames, so it is detail for
// the log, which the id ties the two together with.
func scriptFailure(what string, err error) actions.Result {
	id := correlationID()

	log.Printf("lua script error [%s] running the handler for %s: %v", id, what, err)

	failure := models.ErrScriptFailed
	failure.Message = fmt.Sprintf("%s [%s]", raisedMessage(err), id)

	return actions.Result{Responses: [][]byte{failure.BytesError()}}
}

// raisedMessage is what the script raised, without the traceback gopher-lua
// appends to it.
//
// Read off the ApiError rather than by cutting err.Error() at its first
// newline: a script is free to raise a message with a newline in it, and
// truncating that would hide the half the author wrote.
func raisedMessage(err error) string {
	var apiErr *lua.ApiError
	if errors.As(err, &apiErr) && apiErr.Object != nil {
		return apiErr.Object.String()
	}

	return err.Error()
}

// correlationID ties the frame a player was sent to the log line holding the
// traceback.
//
// Sixty-four bits, because it only has to be unique among the errors in a log
// an operator is reading, and it is short enough to be quoted in a bug report.
func correlationID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail on the platforms this runs on. A frame is
		// still owed to the player if it somehow did, so fall back rather than
		// turning a script's typo into a failed request.
		return "no-id"
	}

	return hex.EncodeToString(buf)
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

	// req.gameId is the one field a handler can rely on whether or not anybody
	// is connected. A timer fires with no session, so a script reading
	// req.session.gameId would fault on exactly the dispatches that most need to
	// find their game; this is the field to read instead. See actions.Request.
	if gameID := req.GameID(); gameID != "" {
		tbl.RawSetString("gameId", lua.LString(gameID))
	}

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

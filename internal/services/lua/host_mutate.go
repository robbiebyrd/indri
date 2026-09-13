package lua

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// invocationRegistryKey is where a state keeps the context of the call it is
// currently serving.
//
// It lives in the Go-side registry for the same reason the handler map does: no
// library the sandbox opens can reach it, and debug is never opened. The host
// table is frozen when the state is built, so indri.mutate is one shared
// closure across every invocation that state goes on to serve — it cannot
// capture the caller's game or deadline. This is where it finds them instead.
const invocationRegistryKey = "indri.invocation"

// GameMutator is the single capability a script's state edits need: run apply
// against one game as a conflict-free read-modify-write, and say whether it
// committed.
//
// Narrow on purpose. A script reaches the game store through this and nothing
// else, so the surface it could reach is one method wide, and *game.Store
// satisfies it without the lua package importing the repo.
type GameMutator interface {
	MutateResult(ctx context.Context, id string, apply func(g *models.Game) error) (committed bool, err error)
}

// invocation is everything indri.mutate needs that varies per call.
//
// It is not guarded by a mutex and must not be. A *lua.LState is single
// threaded and the pool hands one out to exactly one invocation at a time, so
// the only goroutine that can read or write this is the one running the call.
type invocation struct {
	ctx    context.Context
	gameID string
	games  GameMutator

	// session is who is calling, as the transport authenticated them. It is held
	// so that an event indri.send queues is dispatched with the caller's own
	// authority and no more: a script cannot reach an action by sending it that
	// the player could not have sent themselves.
	session *models.Session

	// dispatch is how a sent event reaches the router, once the handler that
	// queued it has returned. Nil on an engine built without one, and
	// indri.send refuses rather than dropping the event.
	dispatch Dispatcher

	// effects is everything this call asked the host to do outside the game
	// document, held until it is known whether the write it belonged to
	// happened. See effects.go for why nothing is performed inline.
	effects ledger

	// inMutate is the reentrancy guard. mutation.Run takes a lock keyed on the
	// game, and lock.InProcess is a plain keyed mutex with no notion of a
	// holder, so a nested indri.mutate on the same game would wait for a lock
	// its own caller is holding and never wake up. Raising is the only outcome
	// that is not a hang.
	inMutate bool
}

// enter marks the invocation as inside a mutate, reporting whether it was
// already. The caller must pair a true return from ok with a deferred leave.
func (inv *invocation) enter() (ok bool) {
	if inv.inMutate {
		return false
	}

	inv.inMutate = true

	return true
}

func (inv *invocation) leave() {
	inv.inMutate = false
}

// setInvocation installs inv as the state's current call context.
//
// The registry is written rather than the (frozen) host table, and it is
// written on every invocation: a pooled state serves many calls and each one
// has its own game, deadline and mutator.
func setInvocation(L *lua.LState, inv *invocation) {
	ud := L.NewUserData()
	ud.Value = inv

	L.G.Registry.RawSetString(invocationRegistryKey, ud)
}

// clearInvocation removes the call context when the call ends.
//
// Not merely tidiness. The context carries the caller's deadline and their game
// id; leaving it behind would let the *next* invocation on this pooled state
// mutate the previous caller's game under an expired deadline if anything ever
// reached indri.mutate outside a call.
func clearInvocation(L *lua.LState) {
	L.G.Registry.RawSetString(invocationRegistryKey, lua.LNil)
}

// currentInvocation returns the call context, or an error describing why there
// is not one.
func currentInvocation(L *lua.LState) (*invocation, error) {
	ud, ok := L.G.Registry.RawGetString(invocationRegistryKey).(*lua.LUserData)
	if !ok {
		return nil, errors.New("there is no request in progress on this state")
	}

	inv, ok := ud.Value.(*invocation)
	if !ok {
		return nil, fmt.Errorf("the lua invocation registry holds a %T", ud.Value)
	}

	return inv, nil
}

// hostMutate is indri.mutate(fn).
//
// fn receives the game as a table and returns the game it wants stored, or nil
// to make no change. It runs *inside* the store's apply closure, which is what
// puts a script edit under the same distributed lock and the same version fence
// as every Go write — rather than inventing a second concurrency model that
// would have to agree with the first.
//
// The return value is whether the state actually changed: false for an explicit
// nil, false for a return that is deep-equal to the input, true for a committed
// write.
//
// Every refusal is L.RaiseError rather than a returned error, so a script bug
// arrives as an ordinary Lua error carrying the offending file and line, and
// unwinds the PCall the handler is running under.
func hostMutate(L *lua.LState) int {
	fn := L.CheckFunction(1)

	inv, err := currentInvocation(L)
	if err != nil {
		L.RaiseError("indri.mutate: %s", err.Error())
	}

	if inv.games == nil {
		L.RaiseError("indri.mutate: this engine was built without a game store")
	}

	if inv.gameID == "" {
		L.RaiseError("indri.mutate: the caller is not in a game")
	}

	if !inv.enter() {
		// The message names the deadlock it is standing in for, because
		// "already in a mutate" alone does not tell a script author that the
		// fix is to do all the work in one callback.
		L.RaiseError("indri.mutate: cannot be called from inside another indri.mutate on the same game")
	}

	defer inv.leave()

	committed, err := inv.games.MutateResult(inv.ctx, inv.gameID, applyThroughLua(L, inv, fn))

	// The committed flag, and not the nil error, is what settles the effects the
	// callback queued: mutation.Run returns nil for an abort as well as for a
	// write, so an error-only reading would release the effects of a mutation
	// that stored nothing. Settled before the raise below, because RaiseError
	// unwinds and never comes back.
	if committed {
		inv.effects.commitAttempt()
	} else {
		inv.effects.discardAttempt()
	}

	if err != nil {
		L.RaiseError("indri.mutate: %s", err.Error())
	}

	L.Push(lua.LBool(committed))

	return 1
}

// applyThroughLua builds the store's apply closure around a Lua callback.
//
// The closure may run up to mutation's retry budget: every version-fence miss
// reloads the game and calls fn again, against the state that actually won. fn
// therefore has to be re-runnable, and it is given a freshly converted table
// each time rather than one it saw before — a callback that kept a reference to
// the previous attempt's table would be editing a document that is no longer
// current.
func applyThroughLua(L *lua.LState, inv *invocation, fn *lua.LFunction) func(*models.Game) error {
	return func(g *models.Game) error {
		// Every run starts the ledger's attempt level over, so whatever the
		// previous, rejected attempt queued goes with the state it described.
		inv.effects.beginAttempt()

		before, err := events.ToMap(g)
		if err != nil {
			return fmt.Errorf("rendering game as a document: %w", err)
		}

		arg, err := toLua(L, before)
		if err != nil {
			return fmt.Errorf("handing the game to lua: %w", err)
		}

		ret, err := callLua(L, fn, arg)
		if err != nil {
			// Returned, not raised: this unwinds mutation.Run without writing,
			// which is what leaves the document untouched and publishes
			// nothing. hostMutate re-raises it once the store has let go.
			return err
		}

		if ret == lua.LNil {
			return mutation.ErrAbort
		}

		if err := applyLua(g, ret, defaultBudget()); err != nil {
			return err
		}

		after, err := events.ToMap(g)
		if err != nil {
			return fmt.Errorf("rendering the edited game as a document: %w", err)
		}

		// A script that reads the state, decides nothing changed and returns it
		// unaltered must not cost a write and must not publish an empty delta.
		// Compared on the document view rather than the struct because that is
		// the view the delta is diffed on: two structs differing only in a field
		// the document does not carry are not a change anyone can observe.
		if reflect.DeepEqual(before, after) {
			return mutation.ErrAbort
		}

		return nil
	}
}

// callLua invokes fn with one argument and returns its single result.
//
// Protected, which matters more here than at the top level: an unprotected error
// in gopher-lua unwinds by panicking through the Go stack, and this call is
// nested inside the store's apply closure with a distributed lock held. Letting
// that panic cross mutation.Run would skip the retry loop's own accounting and
// surface as an ApiErrorPanic that spoils the pooled state, for what is only a
// script bug.
func callLua(L *lua.LState, fn *lua.LFunction, arg lua.LValue) (lua.LValue, error) {
	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, arg); err != nil {
		return nil, err
	}

	ret := L.Get(-1)
	L.Pop(1)

	return ret, nil
}

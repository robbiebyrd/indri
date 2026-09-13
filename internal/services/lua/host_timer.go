package lua

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// maxTimerOps bounds how many timers one invocation may set or call off.
//
// The same shape as maxEventFanout and for a sharper reason: each timer op is a
// write to a shared collection that outlives the request, so a handler looping
// over its players to set one timer each is ordinary, and a handler looping a
// thousand times is filling a database on the strength of one client message.
// The deadline does not help here — queueing is cheap, and the cost lands later,
// on the scheduler.
const maxTimerOps = 32

// maxTimerDelay is the furthest ahead indri.after will set a timer.
//
// It is an overflow guard first and a unit check second. time.Duration is
// nanoseconds in an int64, so a delay of a few hundred years wraps to a negative
// one and the timer fires immediately — the exact opposite of what was asked
// for. A year is also far past any plausible party game, so the value that
// trips this is almost always milliseconds passed where seconds were meant.
//
// indri.at has no equivalent bound. An absolute timestamp is written out in
// full, so it cannot be a unit mistake, and parsing it cannot overflow.
const maxTimerDelay = 365 * 24 * time.Hour

// GameScheduler is the single capability a script's timers need.
//
// Three methods rather than one because a script is handed its timer's id
// synchronously while the entry itself is only written when the ledger flushes:
// NewTimerID mints that id up front, so a script can store it in the very game
// state whose commit releases the write, and ScheduleTimer stores the entry
// under it afterwards.
//
// Every argument is a stdlib type on purpose. It is the same arrangement as
// GameMutator — the surface a script can reach is as narrow as it can be made,
// and an adapter in internal/services/scheduler satisfies this without the lua
// package importing the schedule repo.
type GameScheduler interface {
	// NewTimerID mints the identity a timer will be stored under.
	NewTimerID() string

	// ScheduleTimer stores one deferred action, to be dispatched at fireAt.
	ScheduleTimer(id, gameID, action string, payload map[string]interface{}, fireAt time.Time) error

	// CancelTimer calls off a timer that has not been claimed yet. It takes the
	// game as well as the id because an id is not a secret: a script that passes
	// a player-supplied value straight through must not thereby be able to
	// cancel another game's timers. Cancelling is idempotent — an id that names
	// nothing is not an error.
	CancelTimer(gameID, id string) error
}

// timerEffect is one indri.after or indri.at: an action to be dispatched later,
// written to the schedule store once the handler has returned.
//
// It is queued rather than written for the reason every effect is, and here the
// reason bites hardest. A timer set inside an indri.mutate callback that loses
// the version fence describes a move the store rejected; written inline, every
// one of mutation.Run's ten attempts would have left an entry behind, and the
// game would fire the same timeout ten times.
type timerEffect struct {
	id      string
	gameID  string
	action  string
	payload map[string]interface{}
	fireAt  time.Time
	timers  GameScheduler
}

func (t timerEffect) deliver(_ context.Context, _ *actions.Result) error {
	if err := t.timers.ScheduleTimer(t.id, t.gameID, t.action, t.payload, t.fireAt); err != nil {
		return fmt.Errorf("scheduling the %q timer a script set: %w", t.action, err)
	}

	return nil
}

// cancelEffect is one indri.cancel.
//
// It is queued alongside the timers rather than performed inline so that order
// is kept: a script that sets a timer and calls it off again in the same
// handler must not have the cancel run first and the timer outlive it.
type cancelEffect struct {
	id     string
	gameID string
	timers GameScheduler
}

func (c cancelEffect) deliver(_ context.Context, _ *actions.Result) error {
	if err := c.timers.CancelTimer(c.gameID, c.id); err != nil {
		return fmt.Errorf("cancelling the timer %q a script called off: %w", c.id, err)
	}

	return nil
}

// isTimerOp is the ledger predicate maxTimerOps is measured with. Setting and
// cancelling share one budget because they cost the store the same.
func isTimerOp(e effect) bool {
	switch e.(type) {
	case timerEffect, cancelEffect:
		return true
	default:
		return false
	}
}

// hostAfter is indri.after(seconds, action[, payload]) -> id.
//
// A relative delay and an absolute time are the same thing once resolved, so
// this and indri.at differ only in how they arrive at a fire time.
func hostAfter(L *lua.LState) int {
	seconds := float64(L.CheckNumber(1))
	action := L.CheckString(2)

	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		L.RaiseError("indri.after(%q): a delay must be a finite number of seconds", action)
	}

	// Negative is refused rather than clamped: there is no reading of "fire this
	// thirty seconds ago" that a script author meant, and firing immediately
	// instead would hide the arithmetic that produced it.
	if seconds < 0 {
		L.RaiseError("indri.after(%q): a delay cannot be negative, got %v seconds", action, seconds)
	}

	// Checked before the conversion, not after: the conversion is what overflows.
	if seconds > maxTimerDelay.Seconds() {
		L.RaiseError(
			"indri.after(%q): a delay of %v seconds is further ahead than the %v limit; "+
				"indri.after takes seconds, and a value this large is usually milliseconds",
			action, seconds, maxTimerDelay,
		)
	}

	delay := time.Duration(seconds * float64(time.Second))

	return queueTimer(L, "indri.after", action, time.Now().Add(delay))
}

// hostAt is indri.at(timestamp, action[, payload]) -> id.
//
// RFC 3339 and nothing else. A timestamp reaches here as a string a script
// author typed or built, and accepting several formats would mean guessing
// which one a wrong-looking value was meant to be.
//
// A time already in the past is accepted and fires on the scheduler's next
// tick. Unlike a negative relative delay it is not obviously a mistake — a
// deadline computed from a round that has overrun is a real case — and the
// alternative, refusing it, would leave whatever the timer was for undone.
func hostAt(L *lua.LState) int {
	stamp := L.CheckString(1)
	action := L.CheckString(2)

	fireAt, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		L.RaiseError(
			"indri.at(%q): %q is not an RFC 3339 timestamp (e.g. \"2026-09-12T18:00:00Z\"): %s",
			action, stamp, err.Error(),
		)
	}

	return queueTimer(L, "indri.at", action, fireAt)
}

// hostCancel is indri.cancel(id).
//
// Cancelling something already fired is not an error: the id a script is
// holding may name an entry the scheduler claimed a moment ago, and there is no
// way for the script to have known. What it cannot do is cancel a timer
// belonging to another game — see GameScheduler.CancelTimer.
func hostCancel(L *lua.LState) int {
	id := L.CheckString(1)

	if strings.TrimSpace(id) == "" {
		L.RaiseError("indri.cancel: a timer id cannot be empty")
	}

	inv, err := timerInvocation(L, "indri.cancel")
	if err != nil {
		L.RaiseError("indri.cancel: %s", err.Error())
	}

	if queued := inv.effects.count(isTimerOp); queued >= maxTimerOps {
		L.RaiseError("indri.cancel: a handler may set or cancel at most %d timers", maxTimerOps)
	}

	if err := queueEffect(L, cancelEffect{id: id, gameID: inv.gameID, timers: inv.timers}); err != nil {
		L.RaiseError("indri.cancel: %s", err.Error())
	}

	return 0
}

// queueTimer is the half indri.after and indri.at share: validate, queue, and
// hand the script back the id its timer will be stored under.
//
// The id is minted before anything is written, which is what makes the obvious
// pattern work — `indri.mutate(function(g) g.data.timer = indri.after(30, ...)
// return g end)` stores the id in the same commit that releases the write it
// belongs to. If the attempt loses the version fence the timer goes with it and
// so does the state that named it, so the two cannot disagree.
func queueTimer(L *lua.LState, fname, action string, fireAt time.Time) int {
	// Both callers take the payload as their third argument: when to fire, what
	// to fire, and then whatever the fired handler is to be given.
	const payloadArg = 3

	inv, err := timerInvocation(L, fname)
	if err != nil {
		L.RaiseError("%s(%q): %s", fname, action, err.Error())
	}

	if err := checkScheduledAction(L, action); err != nil {
		L.RaiseError("%s(%q): %s", fname, action, err.Error())
	}

	if queued := inv.effects.count(isTimerOp); queued >= maxTimerOps {
		L.RaiseError("%s(%q): a handler may set or cancel at most %d timers", fname, action, maxTimerOps)
	}

	payload, err := eventPayload(L.OptTable(payloadArg, L.NewTable()))
	if err != nil {
		L.RaiseError("%s(%q): %s", fname, action, err.Error())
	}

	id := inv.timers.NewTimerID()

	queued := timerEffect{
		id:      id,
		gameID:  inv.gameID,
		action:  action,
		payload: payload,
		fireAt:  fireAt,
		timers:  inv.timers,
	}

	if err := queueEffect(L, queued); err != nil {
		L.RaiseError("%s(%q): %s", fname, action, err.Error())
	}

	L.Push(lua.LString(id))

	return 1
}

// timerInvocation returns the call in progress, refusing one that has nothing
// to schedule against.
//
// The game is required for all three functions, cancel included. A timer is
// stamped with the caller's own game — taken from the authenticated session,
// never from the payload — because that stamp is the only thing the fired
// action will have to find its state by, and it is what keeps one game's script
// from reaching another's timers.
func timerInvocation(L *lua.LState, fname string) (*invocation, error) {
	inv, err := currentInvocation(L)
	if err != nil {
		return nil, err
	}

	if inv.timers == nil {
		return nil, fmt.Errorf("this engine was built without a scheduler, so %s cannot store anything", fname)
	}

	if inv.gameID == "" {
		return nil, fmt.Errorf("the caller is not in a game")
	}

	return inv, nil
}

// checkScheduledAction refuses an action name a timer must not fire.
//
// This is the contract the whole feature rests on, and it is worth stating
// plainly: a timer fires with **no session**. Nobody is connected, and inventing
// a session would hand a scheduled action authority no player ever granted it.
// Every built-in framework action rejects a nil session, so scheduling one would
// store an entry that can only ever fail — and a name that matches no handler at
// all is worse, because router.Dispatch succeeds quietly when nothing matches,
// leaving a timer that fires, does nothing, and reports success.
//
// The scheduler checks the same thing again at fire time against the manifest it
// was built with, because an entry outlives the script that wrote it: an action
// removed in an upgrade is dead-lettered there rather than here.
func checkScheduledAction(L *lua.LState, action string) error {
	if strings.TrimSpace(action) == "" {
		return fmt.Errorf("an action name cannot be empty")
	}

	if reason := reservedReason(action); reason != "" {
		return fmt.Errorf(
			"the action %q cannot be scheduled because %s, and a timer fires with no session, "+
				"which every built-in action refuses", action, reason,
		)
	}

	h, err := handlersFor(L)
	if err != nil {
		return err
	}

	if _, ok := h.lookup(action); !ok {
		return fmt.Errorf(
			"no script handler is registered for the action %q, so a timer firing it would "+
				"dispatch to nothing and report success", action,
		)
	}

	return nil
}

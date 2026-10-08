package lua

import (
	"context"
	"errors"
	"expvar"
	"log"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// KillReason is why a script call was stopped.
//
// An operator who sees scripts dying needs to know which way, because the fixes
// differ: a deadline means the handler is too slow, a cap means it builds a
// value too big, and a script error means it has a bug. One "script failed"
// count would hide which.
type KillReason string

const (
	// KillDeadline is a call that ran past its context's deadline.
	KillDeadline KillReason = "deadline"
	// KillStringCap is string.rep or string.format asked to build more than
	// the string cap.
	KillStringCap KillReason = "string_cap"
	// KillDepthCap is a value handed between Lua and the host nested deeper
	// than the state depth cap.
	KillDepthCap KillReason = "depth_cap"
	// KillNodeCap is a value handed between Lua and the host with more values
	// in it than the state node cap.
	KillNodeCap KillReason = "node_cap"
	// KillScriptError is everything else a script raised, or got wrong.
	KillScriptError KillReason = "script_error"
)

// killReasons is every reason, in a stable order.
var killReasons = []KillReason{KillDeadline, KillStringCap, KillDepthCap, KillNodeCap, KillScriptError}

// killsVar is the expvar every engine in the process counts its kills into,
// keyed by reason. Nothing serves it over HTTP: the server mounts its own mux,
// and putting expvar's /debug/vars (or a metrics library and endpoint) on it is
// its own decision.
const killsVar = "indri_lua_kills"

var publishedKills = expvar.NewMap(killsVar)

// killCounts is one engine's kills by reason. The zero value is ready to use.
type killCounts struct {
	mu     sync.Mutex
	counts map[KillReason]int64
}

func (k *killCounts) add(reason KillReason) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.counts == nil {
		k.counts = make(map[KillReason]int64, len(killReasons))
	}

	k.counts[reason]++
}

// KillCounts is how many of this engine's script calls were stopped, by why.
// Every reason is present, at zero if nothing was killed for it.
func (e *Engine) KillCounts() map[KillReason]int64 {
	e.kills.mu.Lock()
	defer e.kills.mu.Unlock()

	out := make(map[KillReason]int64, len(killReasons))
	for _, reason := range killReasons {
		out[reason] = e.kills.counts[reason]
	}

	return out
}

// capError is a cap a value or a string ran into. It keeps the reason beside
// the message, so the failure can be counted under the right cap without ever
// reading the message.
type capError struct {
	reason KillReason
	msg    string
}

func (e *capError) Error() string { return e.msg }

// noteCap records on the running invocation that it hit reason's cap, before
// the host raises the failure into Lua. Raising turns the failure into a Lua
// value and loses the Go error, so the reason has to be written down first. An
// uncaught cap ends the call, so the note left standing is the one that did.
func noteCap(L *lua.LState, reason KillReason) {
	if inv, err := currentInvocation(L); err == nil {
		inv.capHit = reason
	}
}

// forgetCap drops the cap noted on the running invocation: the script caught
// the error it raised.
func forgetCap(L *lua.LState) {
	if inv, err := currentInvocation(L); err == nil {
		inv.capHit = ""
	}
}

// noteCapError is noteCap for a Go error that may be, or wrap, a cap.
func noteCapError(L *lua.LState, err error) {
	var capped *capError
	if errors.As(err, &capped) {
		noteCap(L, capped.reason)
	}
}

// classifyKill decides why a failed call was stopped. The deadline is read off
// the context after the call — never off the error, whose type and message
// depend on where in the interpreter the deadline happened to land.
func classifyKill(ctx context.Context, inv *invocation) KillReason {
	if ctx.Err() != nil {
		return KillDeadline
	}

	if inv.capHit != "" {
		return inv.capHit
	}

	return KillScriptError
}

// recordKill counts a stopped call and logs it with its reason.
func (e *Engine) recordKill(ctx context.Context, inv *invocation, took time.Duration) {
	reason := classifyKill(ctx, inv)

	e.kills.add(reason)
	publishedKills.Add(string(reason), 1)

	log.Printf("lua kill %s reason=%s after=%s", inv.trigger.describe(), reason, took)
}

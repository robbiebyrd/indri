package lua

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// The lifecycle events this server raises.
//
// They are not actions, and the colon is what says so. An action is a name a
// client can reach through the router; a lifecycle event is a name only the
// server raises, about something that has already happened to a game. Keeping
// the two in separate namespaces is what stops a player from dispatching
// "player:left" at a game they are losing, and is why Engine.Actions() — which
// the router, the REST route table and the scheduler are all built from — never
// carries one of these.
const (
	LifecycleGameCreated  = "game:created"
	LifecyclePlayerJoined = "player:joined"
	LifecyclePlayerLeft   = "player:left"
	LifecycleSceneChanged = "scene:changed"
)

// lifecycleMark is what separates the two namespaces a script registers into.
//
// A name carrying it is read as a lifecycle event and held to the list below;
// a name without it is an action. That makes an unknown lifecycle name a load
// failure rather than an action nobody will ever dispatch — which is the whole
// of "refused at load": indri.on("player:quit", ...) is a typo, and a typo that
// registers successfully is a handler that silently never runs.
const lifecycleMark = ":"

// lifecycleEvents is every event this server raises, in the order they read.
var lifecycleEvents = []string{
	LifecycleGameCreated,
	LifecyclePlayerJoined,
	LifecyclePlayerLeft,
	LifecycleSceneChanged,
}

// lifecycleTimeout bounds one lifecycle handler.
//
// It matches the deadline a dispatched script action runs under
// (script.InvocationTimeout), and for the same reason: a script that outlived
// the lease on its game's lock could run beside another instance's attempt on
// the same game. It is a constant here rather than an import because a
// lifecycle event has no caller whose context it could inherit — see emit.
const lifecycleTimeout = 100 * time.Millisecond

// maxPendingLifecycle bounds the deferred queue.
//
// The queue is drained to exhaustion by whichever goroutine opened it, so
// without a cap a script that caused an event from inside a lifecycle handler
// could grow it without bound and pin that goroutine. Dropping past the cap is
// loud: the whole point of the queue is that an event is somebody's to log.
const maxPendingLifecycle = 256

// LifecycleSubject is what one event says about the thing it happened to.
//
// A map rather than a type per event because it is converted to a Lua table
// immediately and never read by Go. The emitter fills in the event name and the
// game id itself, so neither can be forged by a subject key.
type LifecycleSubject = map[string]interface{}

// lifecycleEvent is one thing that happened to one game, waiting to be told to
// the scripts that subscribed.
type lifecycleEvent struct {
	event   string
	gameID  string
	subject LifecycleSubject
}

// lifecycleQueue is the deferred queue every lifecycle event passes through.
//
// Nothing is dispatched by the goroutine that raised it while it is still
// inside the call that raised it. That is the same rule indri.send follows, and
// it exists for the same reason: inline dispatch of an event whose handler can
// cause another event is how an event system recurses until something runs out.
// Here the queue also decouples a script's bug from the built-in action that
// committed — a join that raised player:joined has already written, and must
// not be failed by whatever the subscriber does with it.
//
// Whichever goroutine finds the queue idle owns the drain and runs it to
// exhaustion; anything enqueued meanwhile — by a handler, or by another
// goroutine's own commit — lands on that same drain instead of starting a
// second one. Events for one game are therefore handled one at a time and in
// the order they were raised, which a handler that reads the game depends on.
type lifecycleQueue struct {
	mu       sync.Mutex
	pending  []lifecycleEvent
	draining bool
}

// enqueue buffers ev, reporting whether the caller now owns the drain.
//
// A full queue drops rather than blocks, because the caller is a built-in
// action that has already committed and must not be made to wait on a script.
func (q *lifecycleQueue) enqueue(ev lifecycleEvent) (own bool, dropped bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) >= maxPendingLifecycle {
		return false, true
	}

	q.pending = append(q.pending, ev)

	if q.draining {
		return false, false
	}

	q.draining = true

	return true, false
}

// next hands the drain its next event, closing the drain when there is none
// left. Closing it here rather than in the caller is what makes the transition
// atomic: an enqueue that arrives between the last event and the close is seen
// under the same lock and keeps the drain open.
func (q *lifecycleQueue) next() (lifecycleEvent, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		q.draining = false

		return lifecycleEvent{}, false
	}

	ev := q.pending[0]
	q.pending = q.pending[1:]

	return ev, true
}

// EmitLifecycle tells the scripts that something happened to a game.
//
// It returns nothing, and that is the contract rather than an omission: the
// caller is a built-in action whose write has already committed, and a script's
// bug must not be able to fail a join, a create or a leave. Everything that can
// go wrong here is logged and goes no further.
//
// The call is cheap when nobody is listening — an event no script subscribed to
// costs a slice scan and no Lua state at all, which is what keeps this
// affordable on the path every player takes.
func (e *Engine) EmitLifecycle(event, gameID string, subject LifecycleSubject) {
	if !slices.Contains(lifecycleEvents, event) {
		// A caller inside this repo, not a script: the name is a Go constant.
		log.Printf("refusing to emit the unknown lifecycle event %q for game %q", event, gameID)

		return
	}

	if !slices.Contains(e.lifecycle, event) {
		return
	}

	// A lifecycle handler reaches its game through indri.mutate, which resolves
	// the game from the invocation and has nothing to fall back on. An event
	// without one could only ever raise.
	if strings.TrimSpace(gameID) == "" {
		log.Printf("refusing to emit the lifecycle event %q with no game", event)

		return
	}

	own, dropped := e.events.enqueue(lifecycleEvent{event: event, gameID: gameID, subject: subject})

	if dropped {
		log.Printf(
			"dropping the lifecycle event %q for game %q: more than %d events are already queued",
			event, gameID, maxPendingLifecycle,
		)

		return
	}

	if !own {
		return
	}

	e.drainLifecycle()
}

// drainLifecycle runs queued events until there are none left.
//
// Every failure is logged and the next event still runs. The events in the
// queue are independent — a subscriber that raised on player:joined says
// nothing about whether the scene:changed behind it should be told — and one
// bad handler must not stop a game hearing about everything after it.
func (e *Engine) drainLifecycle() {
	for {
		ev, ok := e.events.next()
		if !ok {
			return
		}

		if err := e.emit(ev); err != nil {
			log.Printf("the lifecycle handler for %q on game %q failed: %v", ev.event, ev.gameID, err)
		}
	}
}

// emit runs one event's handler.
//
// The deadline is its own rather than the emitting request's. A lifecycle event
// is not part of the call that raised it: that call's write has committed, and
// a client who has already gone away — cancelling their context — must not stop
// the game's script hearing what happened. It is the same reasoning
// core.publish uses for fanning a committed delta out on the store's context.
func (e *Engine) emit(ev lifecycleEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleTimeout)
	defer cancel()

	// No session, on purpose and permanently — the same contract a timer fires
	// under. Nobody is connected when a game is created or a player drops, and
	// inventing a session would hand a subscriber authority no player granted
	// it. The game therefore reaches the handler as ev.gameId, exactly as it
	// reaches a timer-fired action as req.gameId.
	_, err := e.run(ctx, lifecycleTrigger{event: ev.event}, nil, ev.gameID, func(L *lua.LState) (lua.LValue, error) {
		return lifecycleToLua(L, ev)
	})

	return err
}

// lifecycleToLua renders one event as the table its handler receives.
//
// The subject is written first and the two fields the host owns after it, so a
// subject key can never redefine which event this is or which game it belongs
// to.
//
// There is no session field. It is absent rather than nil for the reason
// requestToLua leaves it out for an unauthenticated caller: a script asking who
// caused this has to say what it means for the answer to be nobody.
func lifecycleToLua(L *lua.LState, ev lifecycleEvent) (lua.LValue, error) {
	subject, err := toLua(L, ev.subject)
	if err != nil {
		return nil, err
	}

	tbl, ok := subject.(*lua.LTable)
	if !ok {
		tbl = L.NewTable()
	}

	tbl.RawSetString("event", lua.LString(ev.event))
	tbl.RawSetString("gameId", lua.LString(ev.gameID))

	return tbl, nil
}

// trigger is why an invocation is running.
//
// It is an interface with one implementation per kind, rather than a field on
// invocation saying which kind this is, because the kinds differ in what they
// *are* and not in a setting they carry. Each one answers three questions
// differently — which registry holds its handler, what to call it in a message,
// and who is owed its failure — and every one of those answers is a method here
// instead of a condition at the call site.
//
// That shape is load-bearing beyond tidiness. What a handler may reach depends
// on why it is running: blocking I/O inside a player's move holds the game lock
// and re-runs on every version-fence miss, while the same call from a lifecycle
// handler has no lock and no caller waiting. Expressing the kinds as types
// means that difference becomes another method here, on a closed set, rather
// than a boolean that every capability has to remember to test.
type trigger interface {
	// describe names this call for an error or a log line.
	describe() string

	// lookup finds the closure a prepared state registered for it. This is
	// where the two namespaces stay apart: an action never resolves to a
	// lifecycle handler, and a lifecycle event never resolves to an action.
	lookup(h *stateHandlers) (*lua.LFunction, bool)

	// failed renders a script's own failure — as opposed to a host failure —
	// for whoever is owed it.
	failed(err error) (actions.Result, error)
}

// actionTrigger is a dispatched action: a name a client reached through the
// router, or one a timer fired.
type actionTrigger struct {
	action string
}

func (t actionTrigger) describe() string {
	return fmt.Sprintf("the action %q", t.action)
}

func (t actionTrigger) lookup(h *stateHandlers) (*lua.LFunction, bool) {
	return h.lookup(t.action)
}

// failed packs the failure into a frame the caller is answered with. There is
// somebody waiting on a dispatched action, and Responses is the one channel
// every transport writes back — see scriptFailure.
func (t actionTrigger) failed(err error) (actions.Result, error) {
	return scriptFailure(t.describe(), err), nil
}

// lifecycleTrigger is a game lifecycle event: a name only the server raises.
type lifecycleTrigger struct {
	event string
}

func (t lifecycleTrigger) describe() string {
	return fmt.Sprintf("the lifecycle event %q", t.event)
}

func (t lifecycleTrigger) lookup(h *stateHandlers) (*lua.LFunction, bool) {
	return h.lookupLifecycle(t.event)
}

// failed returns the failure rather than framing it, because nobody is waiting
// on a lifecycle event. The built-in action that raised it has already
// committed and is not allowed to fail; the drain logs this instead, with the
// traceback a frame would have dropped.
func (t lifecycleTrigger) failed(err error) (actions.Result, error) {
	return actions.Result{}, fmt.Errorf("%s raised: %w", t.describe(), err)
}

// lifecycleReason explains why name is not a lifecycle event this server
// raises, or returns an empty string when it is.
func lifecycleReason(name string) string {
	if slices.Contains(lifecycleEvents, name) {
		return ""
	}

	return fmt.Sprintf(
		"%q is not a lifecycle event this server raises; it raises %v",
		name, lifecycleEvents,
	)
}

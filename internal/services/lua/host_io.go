package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// The budgets one dispatched action's script-emitted events run under.
//
// maxEventDepth bounds a chain. A handler that sends an event whose handler
// sends another is a cycle, and an uncapped cycle is how every event system
// surveyed eventually recurses until something runs out. Ten is Roblox's
// deferred-signal depth, taken as the reference for "deeper than any honest
// game needs".
//
// maxEventFanout bounds one invocation's breadth, and is deliberately not
// claimed as a bound on the whole tree: 32 events at each of 10 levels is 32^10
// dispatches in the worst case. What actually bounds the tree is the deadline,
// and it is worth knowing why, because the caps alone would be a false comfort.
// Every effect is delivered inside the Invoke that queued it — flush runs before
// Invoke returns — and script.Handler derives each invocation's context from its
// caller's, so every dispatch below one client message shares that message's
// budget and unwinds together when it expires. The caps keep an honest script
// honest; the deadline is what stops a hostile one.
const (
	maxEventDepth  = 10
	maxEventFanout = 32

	// maxReplies is one because one direct answer per dispatched action is the
	// contract, not because of anything a particular transport does with the
	// second. A handler answers the move it was given; a script that replies
	// twice has almost always reached the same line twice by accident — a branch
	// that fell through, a helper that replies as well as its caller — and
	// raising turns that into a file and a line instead of a frame a player never
	// sees.
	//
	// What made it urgent was that the transports did not agree about a second
	// frame, but the cap does not rest on that: the disagreement is being fixed
	// separately under story 068-f269, and this rule is unaffected either way.
	maxReplies = 1
)

// eventDepthKey is where one dispatch chain's depth rides.
//
// On the context rather than on the invocation because the context is the only
// thing that survives the hop. A script-sent event is dispatched as its own
// invocation, on its own pooled state, through the shared router: the two calls
// share no object except the context the effect was delivered under.
type eventDepthKey struct{}

// depthFrom reports how many script-sent events deep this dispatch is. A
// request that arrived from a client carries no depth and is zero.
func depthFrom(ctx context.Context) int {
	depth, _ := ctx.Value(eventDepthKey{}).(int)

	return depth
}

// withDepth marks ctx as being depth events below the client message that
// started the chain.
func withDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, eventDepthKey{}, depth)
}

// Dispatcher runs one action through the shared handler registry, the way a
// transport does.
//
// It is a function type, and a field on the engine rather than a constructor
// argument, for the same reasons rest.Transport.Dispatch is both: the router is
// a package-level registry in the handler layer, which a service has no business
// importing, and a test wants to drive indri.send without standing up one.
type Dispatcher func(
	ctx context.Context,
	session *models.Session,
	action string,
	payload map[string]interface{},
) (actions.Result, error)

// replyEffect is one indri.reply: a document written straight back to the
// caller, and the only thing a script can say to the player who moved that is
// not a state change.
type replyEffect struct {
	document []byte
}

func (r replyEffect) deliver(_ context.Context, res *actions.Result) error {
	res.Responses = append(res.Responses, r.document)

	return nil
}

// sendEffect is one indri.send: an action dispatched after the handler that
// asked for it has returned.
//
// It carries the caller's own session, so a sent event runs with exactly the
// authority the player who triggered it already had — a script cannot reach an
// action by sending it that the player could not have sent themselves.
type sendEffect struct {
	action   string
	payload  map[string]interface{}
	session  *models.Session
	depth    int
	dispatch Dispatcher
}

// deliver dispatches the event, one level deeper than the invocation that
// queued it.
//
// Two halves of the sub-dispatch's result are treated differently, and
// deliberately.
//
// DisconnectIDs are merged: they name sessions whose connections must close,
// the transport applies them from whichever result it is handed, and dropping
// them would make indri.send("kick") half-work.
//
// Responses are dropped. A sent event has no caller waiting on it — the handler
// that asked for it has already returned — so there is nobody for a sub-handler's
// frame to reach, and merging one would also hand a script a second direct
// answer that hostReply had just refused it. Its own failures are not lost with
// it: the sub-invocation logs them under its own correlation id exactly as any
// other invocation does.
func (s sendEffect) deliver(ctx context.Context, res *actions.Result) error {
	sent, err := s.dispatch(withDepth(ctx, s.depth), s.session, s.action, s.payload)

	res.DisconnectIDs = append(res.DisconnectIDs, sent.DisconnectIDs...)

	if err != nil {
		return fmt.Errorf("dispatching the %q event a script sent: %w", s.action, err)
	}

	return nil
}

// isReply and isSend are the ledger predicates the two budgets are measured
// with.
//
// Counting what is on the ledger, rather than counting calls on the invocation,
// is what makes a budget survive a retry. mutation.Run re-runs its callback on
// every version-fence miss and beginAttempt clears the attempt level with it, so
// a script that replies inside indri.mutate is charged once however many
// attempts it took — which a plain call counter would get wrong in the one place
// it matters.
func isReply(e effect) bool {
	_, ok := e.(replyEffect)

	return ok
}

func isSend(e effect) bool {
	_, ok := e.(sendEffect)

	return ok
}

// hostReply is indri.reply(payload).
//
// The payload is a table, not any value: every frame this repo writes to a
// client is a JSON object, and a script answering with a bare string or number
// would produce a document no client parses. Refusing it here is a script
// author's mistake caught at the line that made it.
//
// Nothing is written now. The reply is queued on the ledger like every other
// effect, so one made inside an indri.mutate callback that loses the version
// fence is discarded with the attempt that did not happen — answering a player
// about a state the store rejected is worse than not answering at all.
func hostReply(L *lua.LState) int {
	tbl := L.CheckTable(1)

	inv, err := currentInvocation(L)
	if err != nil {
		L.RaiseError("indri.reply: %s", err.Error())
	}

	if queued := inv.effects.count(isReply); queued >= maxReplies {
		L.RaiseError(
			"indri.reply: a handler may reply at most %d time(s); "+
				"one dispatched action has one direct answer, and anything else a script "+
				"wants to say belongs in the state it writes",
			maxReplies,
		)
	}

	document, err := replyDocument(tbl)
	if err != nil {
		L.RaiseError("indri.reply: %s", err.Error())
	}

	if err := queueEffect(L, replyEffect{document: document}); err != nil {
		L.RaiseError("indri.reply: %s", err.Error())
	}

	return 0
}

// hostSend is indri.send(action[, payload]).
//
// The event is queued and dispatched after the handler returns, never from
// inside it. That is the single most important line in this file: inline
// dispatch of a script-emitted event is the largest source of unbounded
// recursion in every event system surveyed, and it would also dispatch from
// inside indri.mutate's callback, with the game's lock held and the whole
// sub-tree repeated on every retry.
//
// The depth cap is checked against the invocation this call is running in, so a
// handler that sends its own action is refused at the tenth link of the chain
// rather than at the first — the chain is allowed, the runaway is not.
func hostSend(L *lua.LState) int {
	action := L.CheckString(1)

	if strings.TrimSpace(action) == "" {
		L.RaiseError("indri.send: an action name cannot be empty")
	}

	inv, err := currentInvocation(L)
	if err != nil {
		L.RaiseError("indri.send(%q): %s", action, err.Error())
	}

	if inv.dispatch == nil {
		L.RaiseError("indri.send(%q): this engine was built without a dispatcher", action)
	}

	if depth := depthFrom(inv.ctx); depth >= maxEventDepth {
		L.RaiseError(
			"indri.send(%q): event depth %d exceeded; a handler that sends an action which reaches "+
				"it again has to stop itself, because nothing else will",
			action, maxEventDepth,
		)
	}

	if queued := inv.effects.count(isSend); queued >= maxEventFanout {
		L.RaiseError("indri.send(%q): a handler may send at most %d events", action, maxEventFanout)
	}

	payload, err := eventPayload(L.OptTable(2, L.NewTable()))
	if err != nil {
		L.RaiseError("indri.send(%q): %s", action, err.Error())
	}

	queued := sendEffect{
		action:   action,
		payload:  payload,
		session:  inv.session,
		depth:    depthFrom(inv.ctx) + 1,
		dispatch: inv.dispatch,
	}

	if err := queueEffect(L, queued); err != nil {
		L.RaiseError("indri.send(%q): %s", action, err.Error())
	}

	return 0
}

// replyDocument renders a reply table as the JSON frame a transport writes.
//
// It goes through fromLua rather than straight to json.Marshal so that a reply
// is held to exactly the same value rules as stored state: no functions, no
// cycles, no NaN, no key that could forge a delta path. A frame is not a
// document, but a script author who learns one set of rules should not discover
// a second.
func replyDocument(tbl *lua.LTable) ([]byte, error) {
	value, err := fromLua(tbl, defaultBudget())
	if err != nil {
		return nil, err
	}

	document, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("rendering the reply as json: %w", err)
	}

	return document, nil
}

// eventPayload renders a sent event's arguments as the payload a handler
// receives.
//
// An array is refused rather than coerced. actions.Request.Payload is the
// decoded message object every handler reads by name, and a script that passed a
// list would otherwise reach a handler as an empty payload with no sign that
// anything was dropped.
func eventPayload(tbl *lua.LTable) (map[string]interface{}, error) {
	value, err := fromLua(tbl, defaultBudget())
	if err != nil {
		return nil, err
	}

	payload, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("a payload must be a table of named arguments, got %T", value)
	}

	return payload, nil
}

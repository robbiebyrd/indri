package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// ioName is the capability these tests inject alongside the effects one.
//
// It exists for a single function, indri.io.sent, and that function is what
// makes "queued, then drained" distinguishable from "dispatched inline". Both
// arrangements leave the same dispatch log behind once the handler has returned;
// only a script that can ask, while it is still running, can tell them apart.
const ioName = "io"

// dispatchCall is one action a script sent, as the dispatcher received it.
type dispatchCall struct {
	action  string
	payload map[string]interface{}
	session *models.Session
	depth   int
	result  actions.Result
}

// dispatchLog stands in for the router: it records every event a script sent
// and, optionally, goes on to handle it.
//
// handle is what makes the recursion tests real rather than simulated. Pointed
// back at the engine, a sent event runs the same script again on its own pooled
// state, through the same host function, under the context the effect was
// delivered with — which is the whole mechanism the depth cap has to bound.
type dispatchLog struct {
	mu    sync.Mutex
	calls []dispatchCall

	handle Dispatcher
}

// dispatcher returns the Dispatcher an engine is wired to.
func (d *dispatchLog) dispatcher() Dispatcher {
	return func(
		ctx context.Context,
		session *models.Session,
		action string,
		payload map[string]interface{},
	) (actions.Result, error) {
		// Recorded before the handler runs, and the slot kept, so that a chain
		// reads in the order it happened: a nested dispatch appends after the
		// call that caused it rather than before.
		d.mu.Lock()
		at := len(d.calls)
		d.calls = append(d.calls, dispatchCall{
			action:  action,
			payload: payload,
			session: session,
			depth:   depthFrom(ctx),
		})
		d.mu.Unlock()

		var (
			res actions.Result
			err error
		)

		if d.handle != nil {
			res, err = d.handle(ctx, session, action, payload)
		}

		d.mu.Lock()
		d.calls[at].result = res
		d.mu.Unlock()

		return res, err
	}
}

func (d *dispatchLog) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.calls)
}

func (d *dispatchLog) all() []dispatchCall {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.calls)
}

// depths lists how deep each dispatch ran, in order, so a chain can be compared
// against the ladder it is supposed to be.
func (d *dispatchLog) depths() []int {
	depths := make([]int, 0, d.count())

	for _, call := range d.all() {
		depths = append(depths, call.depth)
	}

	return depths
}

// ioCapability installs indri.io.sent, which reports how many events have
// actually been dispatched so far.
func ioCapability(log *dispatchLog) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		tbl := L.NewTable()

		tbl.RawSetString("sent", L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LNumber(log.count()))

			return 1
		}))

		return tbl, nil
	}
}

// newIOEngine builds an engine over src and returns it with the path it was
// written to, which is what a script error names.
//
// Two capabilities are granted. effects is how a test interleaves a reply with
// other queued effects, so ordering can be asserted across the ledger's two
// levels; io is how a script observes dispatch while it is still running.
// indri.reply and indri.send themselves are not capabilities — they are on the
// shared host table — so granting these does not affect what is under test.
func newIOEngine(
	t *testing.T,
	src string,
	games GameMutator,
	effects *deliveryLog,
	sent *dispatchLog,
) (*Engine, string) {
	t.Helper()

	path := writeScript(t, "game.lua", src)

	e, err := newEngine(
		[]models.ScriptFile{{Path: path, Grants: []string{effectsName, ioName}}},
		games,
		capabilitySet{effectsName: effectsCapability(effects), ioName: ioCapability(sent)},
	)
	if err != nil {
		t.Fatalf("building an engine over %v: %v", path, err)
	}

	t.Cleanup(e.Close)

	e.Dispatch = sent.dispatcher()

	return e, path
}

// invokeIO dispatches "move" for a player in gameID, folding a script error
// back into an ordinary Go error the way splitScriptError describes.
func invokeIO(t *testing.T, e *Engine, gameID string) (actions.Result, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	return splitScriptError(e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &gameID},
	}))
}

// reInvoke is the dispatcher half of a recursion test: a sent event runs the
// same engine again, on its own pooled state, under the context the effect was
// delivered with.
func reInvoke(e **Engine) Dispatcher {
	return func(
		ctx context.Context,
		session *models.Session,
		action string,
		payload map[string]interface{},
	) (actions.Result, error) {
		return (*e).Invoke(ctx, action, actions.Request{
			Context: ctx,
			Action:  action,
			Session: session,
			Payload: payload,
		})
	}
}

// --- indri.reply ---------------------------------------------------------------

// A reply is a frame in Result.Responses, and its place in that slice is the
// place it was queued from — including across the ledger's two levels, because
// a script that answers from inside indri.mutate has not thereby jumped the
// queue.
func TestReply_LandsInResponsesInOrder(t *testing.T) {
	tests := map[string]struct {
		body string
		want []string
	}{
		"a reply on its own": {
			body: `indri.reply({ok = true})`,
			want: []string{`{"ok":true}`},
		},
		"a reply keeps its place among the effects around it": {
			body: `
  indri.effects.queue("before")
  indri.reply({ok = true})
  indri.effects.queue("after")`,
			want: []string{"before", `{"ok":true}`, "after"},
		},
		"a reply queued inside a mutate keeps its place too": {
			body: `
  indri.effects.queue("before")
  indri.mutate(function(state)
    indri.effects.queue("inside")
    indri.reply({ok = true})
    state.data = state.data or {}
    state.data.round = 7
    return state
  end)
  indri.effects.queue("after")`,
			want: []string{"before", "inside", `{"ok":true}`, "after"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, id := newTestGame(t)
			effects := newDeliveryLog()

			e, _ := newIOEngine(t, `
indri.on("move", function(req)
  `+test.body+`
end)
`, store, effects, &dispatchLog{})

			res, err := invokeIO(t, e, id)
			if err != nil {
				t.Fatalf("the script failed: %v", err)
			}

			if got := responseMarks(res); !slices.Equal(got, test.want) {
				t.Fatalf("the caller was answered with %v, want %v in that order", got, test.want)
			}
		})
	}
}

// A reply is nothing the handler sees happen. It is queued like every other
// effect and released only once the handler has returned, so a script that
// replies and then fails answers with its failure and not with both.
func TestReply_IsNotDeliveredUntilTheHandlerReturns(t *testing.T) {
	store, _, id := newTestGame(t)
	effects := newDeliveryLog()

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.reply({ok = true})
  error("the script gave up after replying", 0)
end)
`, store, effects, &dispatchLog{})

	res, err := invokeIO(t, e, id)
	if err == nil {
		t.Fatal("the failing script reported success")
	}

	if !strings.Contains(err.Error(), "gave up after replying") {
		t.Fatalf("the error %q does not carry the script's own message", err.Error())
	}

	if got := responseMarks(res); len(got) != 0 {
		t.Fatalf("the caller was answered with %v besides the failure, want nothing", got)
	}
}

// One dispatched action has one direct answer, and the second call raises
// rather than being queued or dropped.
//
// It is a contract rather than a resource budget. A handler answers the move it
// was given, and a script that replies twice has nearly always reached the same
// line twice by accident; raising turns that into a file and a line its author
// can open.
func TestReply_IsCappedAtOnePerInvocation(t *testing.T) {
	store, _, id := newTestGame(t)
	effects := newDeliveryLog()

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.reply({first = true})
  indri.reply({second = true})
end)
`, store, effects, &dispatchLog{})

	res, err := invokeIO(t, e, id)
	if err == nil {
		t.Fatal("a handler replied twice")
	}

	if !strings.Contains(err.Error(), "at most 1 time") {
		t.Fatalf("the error %q does not explain the cap that was hit", err.Error())
	}

	// The handler unwound, so even the reply it had already queued is not owed:
	// the failure is the whole answer.
	if got := responseMarks(res); len(got) != 0 {
		t.Fatalf("the caller was answered with %v as well as the failure, want nothing", got)
	}
}

// The cap counts what is on the ledger, not how many times the host function
// was called, and a retry is where the difference shows.
//
// mutation.Run re-runs its callback on every version-fence miss, so a script
// that replies inside indri.mutate calls indri.reply once per attempt. Charging
// it per call would refuse the second attempt — the one that actually commits —
// and turn an ordinary write conflict into a script error the author cannot
// reproduce.
func TestReply_SurvivesAMutateRetryAndIsDeliveredOnce(t *testing.T) {
	store, _, id := newTestGame(t)
	games := &conflictOnce{inner: store}
	effects := newDeliveryLog()

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.reply({ok = true})
    state.data = state.data or {}
    state.data.round = (state.data.round or 0) + 1
    return state
  end)
end)
`, games, effects, &dispatchLog{})

	res, err := invokeIO(t, e, id)
	if err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	// One frame, not two: the losing attempt's reply went with the state the
	// store rejected.
	if got := responseMarks(res); !slices.Equal(got, []string{`{"ok":true}`}) {
		t.Fatalf("the caller was answered with %v, want exactly one reply", got)
	}
}

// A mutate that stores nothing owes the caller nothing. Both flavours of no-op
// return a nil error — an abort is not a failure — so a reply released on the
// strength of the error alone would tell a player about a move that never
// happened.
func TestReply_InsideAMutateThatStoredNothingIsDiscarded(t *testing.T) {
	tests := map[string]string{
		"returning nil aborts":                `return nil`,
		"returning an unchanged state aborts": `return state`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			store, publisher, id := newTestGame(t)
			effects := newDeliveryLog()

			e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.reply({ok = true})
    `+body+`
  end)
end)
`, store, effects, &dispatchLog{})

			res, err := invokeIO(t, e, id)
			if err != nil {
				t.Fatalf("the script failed: %v", err)
			}

			if got := responseMarks(res); len(got) != 0 {
				t.Fatalf("the caller was answered with %v for a mutation that stored nothing, want nothing", got)
			}

			// The discarded reply only makes sense if the write really was
			// discarded too.
			if got := publisher.count(); got != 0 {
				t.Fatalf("published %d events for a mutation that stored nothing, want 0", got)
			}
		})
	}
}

// A reply is held to the same value rules as stored state, and to the object
// shape every frame this repo writes already has.
func TestReply_RefusesAPayloadItCannotRender(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"a function has no json form": {
			body: `indri.reply({fn = function() end})`,
			want: "cannot convert Lua value of type function",
		},
		"a key that would forge a delta path": {
			body: `indri.reply({["players.p1.privateData"] = "forged"})`,
			want: "forge a delta path segment",
		},
		"a bare string is not a document": {
			body: `indri.reply("nope")`,
			want: "table expected",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, id := newTestGame(t)

			e, _ := newIOEngine(t, `
indri.on("move", function(req)
  `+test.body+`
end)
`, store, newDeliveryLog(), &dispatchLog{})

			res, err := invokeIO(t, e, id)
			if err == nil {
				t.Fatal("a reply that cannot be rendered was accepted")
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("the error %q does not explain the refusal, want it to mention %q", err.Error(), test.want)
			}

			if got := responseMarks(res); len(got) != 0 {
				t.Fatalf("the caller was answered with %v, want nothing", got)
			}
		})
	}
}

// --- indri.send ----------------------------------------------------------------

// The rule the whole file exists for: a sent event is queued and dispatched
// after the handler returns, never from inside it.
//
// The script asserts it itself, because that is the only place the difference is
// visible. Once the handler has returned, "queued then drained" and "dispatched
// inline" leave exactly the same dispatch log behind.
func TestSend_DispatchesOnlyAfterTheHandlerReturns(t *testing.T) {
	store, _, id := newTestGame(t)
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.send("tick", {n = 1})

  if indri.io.sent() ~= 0 then
    error("the event was dispatched inline, from inside the handler", 0)
  end
end)
`, store, newDeliveryLog(), sent)

	if _, err := invokeIO(t, e, id); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	calls := sent.all()
	if len(calls) != 1 {
		t.Fatalf("dispatched %d events, want exactly 1", len(calls))
	}

	if calls[0].action != "tick" {
		t.Fatalf("dispatched %q, want %q", calls[0].action, "tick")
	}

	if got := calls[0].payload["n"]; got != float64(1) {
		t.Fatalf("the event carried n = %v (%T), want 1", got, got)
	}

	// The caller's own session, so a sent event runs with exactly the authority
	// the player who triggered it already had.
	if calls[0].session == nil || calls[0].session.UserID == nil || *calls[0].session.UserID != "player-1" {
		t.Fatalf("the event was dispatched as %+v, want the caller's own session", calls[0].session)
	}

	// A client message is depth zero, so the event it caused is depth one.
	if calls[0].depth != 1 {
		t.Fatalf("the event ran at depth %d, want 1", calls[0].depth)
	}
}

// The same rule inside indri.mutate, which is where inline dispatch would do
// real damage rather than merely recurse: the callback runs with the game's
// distributed lock held and is re-run on every version-fence miss, so an event
// dispatched from inside it would fire once per attempt, under the lock, and
// the attempts that were thrown away would have sent events about a state
// nobody stored.
func TestSend_InsideARetriedMutateDispatchesOncePerCommit(t *testing.T) {
	store, _, id := newTestGame(t)
	games := &conflictOnce{inner: store}
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.send("tick")

    if indri.io.sent() ~= 0 then
      error("the event was dispatched from inside the mutate callback", 0)
    end

    state.data = state.data or {}
    state.data.round = (state.data.round or 0) + 1
    return state
  end)
end)
`, games, newDeliveryLog(), sent)

	if _, err := invokeIO(t, e, id); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	// The callback ran twice; the event was dispatched once.
	if got := sent.count(); got != 1 {
		t.Fatalf("dispatched %d events across a retried mutate, want 1", got)
	}
}

// An event queued by a mutate that stored nothing describes a move that did not
// happen and must go with it.
func TestSend_InsideAMutateThatStoredNothingIsDiscarded(t *testing.T) {
	store, _, id := newTestGame(t)
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.send("tick")
    return nil
  end)
end)
`, store, newDeliveryLog(), sent)

	if _, err := invokeIO(t, e, id); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if got := sent.count(); got != 0 {
		t.Fatalf("dispatched %d events for a mutation that stored nothing, want 0", got)
	}
}

// A handler that sends its own action is a cycle, and the cycle is allowed —
// what is not allowed is a cycle with no end. It must stop itself at the depth
// cap, because the chain runs through the router and nothing else in it is
// counting.
//
// The dispatcher here is the engine, so every link is a real invocation on its
// own pooled state, with its own ledger, reached through the context the
// previous link's effect was delivered under. That context is the only thing the
// two calls share, which is why the depth rides on it.
func TestSend_AHandlerSendingItsOwnActionStopsAtTheDepthCap(t *testing.T) {
	store, _, id := newTestGame(t)
	sent := &dispatchLog{}

	var e *Engine

	e, _ = newIOEngine(t, `
indri.on("move", function(req)
  indri.send("move")
end)
`, store, newDeliveryLog(), sent)

	// Recorded by the log, then handled by the engine: the chain is real and the
	// recursion is the production one.
	sent.handle = reInvoke(&e)

	res, err := invokeIO(t, e, id)
	if err != nil {
		t.Fatalf("the handler that started the chain failed: %v", err)
	}

	if got := responseMarks(res); len(got) != 0 {
		t.Fatalf("the caller was answered with %v, want nothing — the chain answers nobody", got)
	}

	// The client message is depth zero and sends at depth 0; the event it causes
	// runs at depth 1 and sends again; the one at maxEventDepth is refused before
	// it can queue. So the chain is exactly maxEventDepth dispatches long.
	calls := sent.all()
	if len(calls) != maxEventDepth {
		t.Fatalf("the chain ran %d dispatches, want exactly %d", len(calls), maxEventDepth)
	}

	want := make([]int, 0, maxEventDepth)
	for depth := 1; depth <= maxEventDepth; depth++ {
		want = append(want, depth)
	}

	if got := sent.depths(); !slices.Equal(got, want) {
		t.Fatalf("the chain ran at depths %v, want the ladder %v", got, want)
	}

	// And it ended with a script error rather than by quietly stopping, so the
	// author of a runaway handler is told why it stopped.
	last := calls[len(calls)-1].result
	requireScriptErrorMentions(t, last, fmt.Sprintf("event depth %d exceeded", maxEventDepth))
}

// The breadth cap, which bounds one invocation rather than a chain. The
// thirty-third send raises, and because the handler then unwinds, none of the
// thirty-two it had already queued is dispatched either.
func TestSend_FanOutBeyondTheCapErrors(t *testing.T) {
	tests := map[string]struct {
		count          int
		wantErr        string
		wantDispatches int
	}{
		"exactly the cap is allowed": {
			count:          maxEventFanout,
			wantDispatches: maxEventFanout,
		},
		"one over the cap is refused": {
			count:          maxEventFanout + 1,
			wantErr:        fmt.Sprintf("at most %d events", maxEventFanout),
			wantDispatches: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, id := newTestGame(t)
			sent := &dispatchLog{}

			e, _ := newIOEngine(t, fmt.Sprintf(`
indri.on("move", function(req)
  for i = 1, %d do
    indri.send("tick", {i = i})
  end
end)
`, test.count), store, newDeliveryLog(), sent)

			_, err := invokeIO(t, e, id)

			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("the script failed: %v", err)
			case test.wantErr != "" && err == nil:
				t.Fatal("a handler sent more events than the cap allows")
			case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("the error %q does not explain the cap that was hit", err.Error())
			}

			if got := sent.count(); got != test.wantDispatches {
				t.Fatalf("dispatched %d events, want %d", got, test.wantDispatches)
			}
		})
	}
}

// The fan-out budget is measured on the ledger for the same reason the reply
// budget is, and a retry is where the difference shows: a callback re-run on a
// version-fence miss calls indri.send again, and charging it per call would
// refuse the attempt that actually commits.
func TestSend_FanOutBudgetSurvivesAMutateRetry(t *testing.T) {
	store, _, id := newTestGame(t)
	games := &conflictOnce{inner: store}
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, fmt.Sprintf(`
indri.on("move", function(req)
  indri.mutate(function(state)
    for i = 1, %d do
      indri.send("tick", {i = i})
    end

    state.data = state.data or {}
    state.data.round = (state.data.round or 0) + 1
    return state
  end)
end)
`, maxEventFanout), games, newDeliveryLog(), sent)

	_, err := invokeIO(t, e, id)
	if err != nil {
		t.Fatalf("a retried mutate ran out of fan-out budget: %v", err)
	}

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	if got := sent.count(); got != maxEventFanout {
		t.Fatalf("dispatched %d events, want %d — the retry was double-charged or double-delivered", got, maxEventFanout)
	}
}

// What a sent event's own result is worth to the caller, and what it is not.
//
// DisconnectIDs are merged: they name connections the transport must close, and
// dropping them would make indri.send("kick") appear to work and do nothing.
// Responses are not: the handler that sent the event has already returned, so a
// sub-handler's frame has no caller to reach, and merging it would hand the
// script a second direct answer by the back door.
func TestSend_MergesDisconnectIDsButNotResponses(t *testing.T) {
	store, _, id := newTestGame(t)

	sent := &dispatchLog{
		handle: func(
			context.Context, *models.Session, string, map[string]interface{},
		) (actions.Result, error) {
			return actions.Result{
				Responses:     [][]byte{[]byte(`{"from":"the event"}`)},
				DisconnectIDs: []string{"session-1"},
			}, nil
		},
	}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.send("kick", {userId = "someone"})
end)
`, store, newDeliveryLog(), sent)

	res, err := invokeIO(t, e, id)
	if err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if got := responseMarks(res); len(got) != 0 {
		t.Fatalf("the caller was answered with %v, want nothing — a sent event answers nobody", got)
	}

	if got := res.DisconnectIDs; !slices.Equal(got, []string{"session-1"}) {
		t.Fatalf("the result carries DisconnectIDs %v, want [session-1]", got)
	}
}

// An engine with no dispatcher cannot deliver an event, and must say so at the
// line that asked rather than accept it and drop it. A silently discarded event
// is a game that stops advancing for no visible reason.
func TestSend_RefusesWhenTheEngineHasNoDispatcher(t *testing.T) {
	store, _, id := newTestGame(t)
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.send("tick")
end)
`, store, newDeliveryLog(), sent)

	// Undo the wiring newIOEngine did, which is the state an engine built by
	// NewEngine is in until boot sets it.
	e.Dispatch = nil

	_, err := invokeIO(t, e, id)
	if err == nil {
		t.Fatal("an event was sent with no dispatcher wired up")
	}

	if !strings.Contains(err.Error(), "without a dispatcher") {
		t.Fatalf("the error %q does not name the missing dispatcher", err.Error())
	}

	if got := sent.count(); got != 0 {
		t.Fatalf("dispatched %d events with no dispatcher, want 0", got)
	}
}

// The arguments a sent event carries have to be the named-argument object every
// handler reads by key. A list would arrive as an empty payload with nothing to
// say that anything had been dropped.
func TestSend_RefusesArgumentsThatAreNotNamed(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"a list is not a payload": {
			body: `indri.send("tick", {1, 2, 3})`,
			want: "table of named arguments",
		},
		"an empty action name": {
			body: `indri.send("  ")`,
			want: "cannot be empty",
		},
		"a value with no go form": {
			body: `indri.send("tick", {fn = function() end})`,
			want: "cannot convert Lua value of type function",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, id := newTestGame(t)
			sent := &dispatchLog{}

			e, _ := newIOEngine(t, `
indri.on("move", function(req)
  `+test.body+`
end)
`, store, newDeliveryLog(), sent)

			_, err := invokeIO(t, e, id)
			if err == nil {
				t.Fatal("a malformed event was accepted")
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("the error %q does not explain the refusal, want it to mention %q", err.Error(), test.want)
			}

			if got := sent.count(); got != 0 {
				t.Fatalf("dispatched %d events for a refused send, want 0", got)
			}
		})
	}
}

// A handler that sends no events must carry no depth of its own into anything
// it does later, and a fresh client message must start from zero however deep
// the last chain went. The pooled state is shared; the context is not.
func TestSend_DepthDoesNotLeakBetweenInvocations(t *testing.T) {
	store, _, id := newTestGame(t)
	sent := &dispatchLog{}

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  indri.send("tick")
end)
`, store, newDeliveryLog(), sent)

	for call := 1; call <= 2; call++ {
		if _, err := invokeIO(t, e, id); err != nil {
			t.Fatalf("invocation %d failed: %v", call, err)
		}
	}

	// Both client messages are depth zero, so both events are depth one. A depth
	// carried on anything the pool reuses would make the second a two.
	if got := sent.depths(); !slices.Equal(got, []int{1, 1}) {
		t.Fatalf("the two invocations sent at depths %v, want [1 1]", got)
	}
}

// --- how a script's failure reaches its caller -----------------------------------

// A script's own failure is a frame in Responses and not the Go error return.
//
// The three transports do not agree about a Go error — boot.handleClientMessage
// only logs one, so a WebSocket player would see nothing while a REST caller saw
// the raw Go string — and Responses is the one channel all of them write back.
// The three-transport half of this is in internal/services/boot/handlers_test.go.
func TestScriptError_ReachesTheCallerAsAWSError(t *testing.T) {
	store, _, id := newTestGame(t)

	e, path := newIOEngine(t, `
indri.on("move", function(req)
  error("the script gave up")
end)
`, store, newDeliveryLog(), &dispatchLog{})

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	res, err := e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
	})
	if err != nil {
		t.Fatalf("a script error was returned as a Go error, where WebSocket would drop it: %v", err)
	}

	if len(res.Responses) != 1 {
		t.Fatalf("the caller was answered with %d frames, want exactly 1", len(res.Responses))
	}

	var frame models.WSError
	if err := json.Unmarshal(res.Responses[0], &frame); err != nil {
		t.Fatalf("the frame %q is not a models.WSError: %v", res.Responses[0], err)
	}

	if frame.ErrorCode != models.ErrScriptFailed.ErrorCode {
		t.Errorf("the frame carries code %d, want %d", frame.ErrorCode, models.ErrScriptFailed.ErrorCode)
	}

	if frame.OperationType != "error" {
		t.Errorf("the frame carries op %q, want %q", frame.OperationType, "error")
	}

	// The script's own message, and the file and line that raised it. Scripts
	// here are operator-authored, and an author who cannot read the server's
	// logs has nothing else to go on.
	if !strings.Contains(frame.Message, "the script gave up") {
		t.Errorf("the message %q does not carry the script's own words", frame.Message)
	}

	if !strings.Contains(frame.Message, path+":3:") {
		t.Errorf("the message %q does not name the script and line that raised", frame.Message)
	}

	// And a correlation id, which is what ties the frame to the log line
	// carrying the traceback.
	if id := correlationIDOf(frame.Message); id == "" {
		t.Errorf("the message %q carries no correlation id", frame.Message)
	}
}

// Two failures must not be told apart by their correlation ids being the same.
func TestScriptError_CarriesADistinctCorrelationIDPerFailure(t *testing.T) {
	store, _, id := newTestGame(t)

	e, _ := newIOEngine(t, `
indri.on("move", function(req)
  error("the script gave up", 0)
end)
`, store, newDeliveryLog(), &dispatchLog{})

	seen := make(map[string]struct{}, 2)

	for call := 1; call <= 2; call++ {
		res, err := e.Invoke(context.Background(), "move", actions.Request{
			Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
		})
		if err != nil {
			t.Fatalf("invocation %d returned a Go error: %v", call, err)
		}

		var frame models.WSError
		if err := json.Unmarshal(res.Responses[0], &frame); err != nil {
			t.Fatalf("the frame %q is not a models.WSError: %v", res.Responses[0], err)
		}

		got := correlationIDOf(frame.Message)
		if got == "" {
			t.Fatalf("invocation %d answered with %q, which carries no correlation id", call, frame.Message)
		}

		if _, dup := seen[got]; dup {
			t.Fatalf("both failures were logged under the correlation id %q", got)
		}

		seen[got] = struct{}{}
	}
}

// A host failure still goes through the error return. It means the engine is
// misconfigured or the router and the manifest have drifted apart, and the
// operator — not the player — is who needs to hear about it.
func TestInvoke_HostFailuresStillReachTheErrorReturn(t *testing.T) {
	store, _, id := newTestGame(t)

	e, _ := newIOEngine(t, `indri.on("move", function(req) end)`, store, newDeliveryLog(), &dispatchLog{})

	res, err := e.Invoke(context.Background(), "nosuchaction", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
	})
	if err == nil {
		t.Fatal("an unregistered action was answered as though a script had failed")
	}

	if len(res.Responses) != 0 {
		t.Fatalf("a host failure answered the caller with %q, want nothing", res.Responses)
	}
}

// --- helpers ---------------------------------------------------------------------

// correlationIDOf pulls the bracketed id off the end of a script error message,
// or returns an empty string when there is not one.
func correlationIDOf(message string) string {
	at := strings.LastIndex(message, " [")
	if at < 0 || !strings.HasSuffix(message, "]") {
		return ""
	}

	return message[at+2 : len(message)-1]
}

// requireScriptErrorMentions fails unless res is a single models.WSError frame
// naming every want.
func requireScriptErrorMentions(t *testing.T, res actions.Result, wants ...string) {
	t.Helper()

	if len(res.Responses) != 1 {
		t.Fatalf("the result carries %d frames, want exactly one script error", len(res.Responses))
	}

	var frame models.WSError
	if err := json.Unmarshal(res.Responses[0], &frame); err != nil {
		t.Fatalf("the frame %q is not a models.WSError: %v", res.Responses[0], err)
	}

	if frame.ErrorCode != models.ErrScriptFailed.ErrorCode {
		t.Fatalf("the frame carries code %d, want a script failure (%d)", frame.ErrorCode, models.ErrScriptFailed.ErrorCode)
	}

	for _, want := range wants {
		if !strings.Contains(frame.Message, want) {
			t.Fatalf("the message %q does not mention %q", frame.Message, want)
		}
	}
}

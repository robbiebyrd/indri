package lua

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// effectsName is the capability these tests inject.
//
// A test capability rather than reply or send, because neither exists yet
// (stories 054 and 057) and because what is under test is the ledger, not any
// particular effect: the thing being proved is *when* a queued effect is
// delivered and when it is thrown away. indri.effects.queue goes through
// queueEffect — the same seam reply, send and the timers will use — so these
// tests exercise the production path and not a parallel one.
const effectsName = "effects"

// deliveryLog is the difference between what a script asked for and what
// actually happened.
//
// Both halves matter and neither alone would do. queued proves the callback
// really ran more than once on a retry, which is the situation the ledger
// exists for; delivered proves that the extra runs cost nothing. A test that
// recorded only deliveries would pass just as well against a store that never
// retried at all.
type deliveryLog struct {
	mu        sync.Mutex
	queued    []string
	delivered []string

	// fails makes one mark's delivery fail, so the ledger's behaviour around a
	// broken effect can be asserted rather than assumed.
	fails map[string]error
}

func newDeliveryLog() *deliveryLog {
	return &deliveryLog{fails: map[string]error{}}
}

func (d *deliveryLog) queue(mark string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.queued = append(d.queued, mark)
}

func (d *deliveryLog) deliver(mark string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.delivered = append(d.delivered, mark)

	return d.fails[mark]
}

func (d *deliveryLog) queuedMarks() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.queued)
}

func (d *deliveryLog) deliveredMarks() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.delivered)
}

func (d *deliveryLog) deliveredCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.delivered)
}

// markEffect is one buffered effect that records its own delivery and answers
// the caller with its mark, the way a reply will.
type markEffect struct {
	mark string
	log  *deliveryLog
}

func (m markEffect) deliver(_ context.Context, res *actions.Result) error {
	if err := m.log.deliver(m.mark); err != nil {
		return err
	}

	res.Responses = append(res.Responses, []byte(m.mark))

	return nil
}

// queueingEffect is an effect that queues another one as it is delivered, which
// the ledger is required to refuse rather than swallow.
type queueingEffect struct {
	ledger *ledger
	log    *deliveryLog
}

func (q queueingEffect) deliver(_ context.Context, _ *actions.Result) error {
	q.ledger.queue(markEffect{mark: "too late", log: q.log})

	return nil
}

// effectsCapability installs indri.effects: a queue function that buffers a
// mark, and a delivered function reporting how many effects have actually been
// released so far.
//
// delivered is what lets a *script* assert that nothing was delivered while it
// was still running. Without it, "queued then flushed" and "performed inline"
// produce the same log at the end of the call.
func effectsCapability(log *deliveryLog) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		tbl := L.NewTable()

		tbl.RawSetString("queue", L.NewFunction(func(L *lua.LState) int {
			mark := L.CheckString(1)

			log.queue(mark)

			if err := queueEffect(L, markEffect{mark: mark, log: log}); err != nil {
				L.RaiseError("indri.effects.queue: %s", err.Error())
			}

			return 0
		}))

		tbl.RawSetString("delivered", L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LNumber(log.deliveredCount()))

			return 1
		}))

		return tbl, nil
	}
}

// invokeWithEffects loads src as a script granted the effects capability,
// dispatches "move" against gameID and returns what the invocation produced.
func invokeWithEffects(
	t *testing.T,
	log *deliveryLog,
	games GameMutator,
	gameID, src string,
) (actions.Result, error) {
	t.Helper()

	path := writeScript(t, "game.lua", src)

	e, err := newEngine(
		[]models.ScriptFile{{Path: path, Grants: []string{effectsName}}},
		games,
		capabilitySet{effectsName: effectsCapability(log)},
	)
	if err != nil {
		t.Fatalf("building an engine over %v: %v", path, err)
	}

	t.Cleanup(e.Close)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	return e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &gameID},
	})
}

// responseMarks renders the frames an invocation answered with, so the order
// they were delivered in can be compared directly.
func responseMarks(res actions.Result) []string {
	marks := make([]string, 0, len(res.Responses))

	for _, response := range res.Responses {
		marks = append(marks, string(response))
	}

	return marks
}

// --- through a real mutation --------------------------------------------------

// The whole reason the ledger exists: mutation.Run re-runs the apply closure on
// every version-fence miss, so an effect performed inline would fire once per
// attempt. It must instead be emitted once per *commit* — and the effects queued
// around the mutate must keep their places in the queue.
//
// Emitted once per commit, not exactly once: the flush happens after the write
// commits and a standalone mongod cannot write the game and an outbox in one
// act, so a duplicate delivery is possible and is absorbed downstream by the
// scheduled entry's idempotency key. See the guarantee documented on effect.
func TestEffects_QueuedInARetriedMutateAreEmittedOncePerCommit(t *testing.T) {
	store, _, id := newTestGame(t)
	games := &conflictOnce{inner: store}
	log := newDeliveryLog()

	res, err := invokeWithEffects(t, log, games, id, `
indri.on("move", function(req)
  indri.effects.queue("before")
  indri.mutate(function(state)
    indri.effects.queue("inside")
    state.data = state.data or {}
    state.data.round = (state.data.round or 0) + 1
    return state
  end)
  indri.effects.queue("after")
end)
`)
	if err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	// Two "inside" marks: the callback really did run twice, once for the
	// attempt that lost the fence and once for the one that won it.
	wantQueued := []string{"before", "inside", "inside", "after"}
	if got := log.queuedMarks(); !slices.Equal(got, wantQueued) {
		t.Fatalf("the script queued %v, want %v — the retry did not happen as expected", got, wantQueued)
	}

	// One "inside" mark, in its original place. The lost attempt's copy went with
	// the state the store rejected.
	want := []string{"before", "inside", "after"}
	if got := log.deliveredMarks(); !slices.Equal(got, want) {
		t.Fatalf("delivered %v, want %v", got, want)
	}

	if got := responseMarks(res); !slices.Equal(got, want) {
		t.Fatalf("the caller was answered with %v, want %v in that order", got, want)
	}
}

// A mutation that stores nothing must cost nothing. Two of these three cases
// return a nil error — an abort is not a failure — which is exactly why the
// ledger is settled by the committed flag MutateResult reports and not by the
// error: an error-only reading would release the effects of a mutation that
// wrote nothing at all.
func TestEffects_AnAbortedOrErroredMutateDropsItsEffects(t *testing.T) {
	tests := map[string]struct {
		body    string
		wantErr string
		want    []string
	}{
		"returning nil aborts": {
			body: `indri.effects.queue("inside") return nil`,
			want: []string{"before", "after"},
		},
		"returning an unchanged state aborts": {
			body: `indri.effects.queue("inside") return state`,
			want: []string{"before", "after"},
		},
		// The handler unwinds here, so not even the effects queued outside the
		// mutate are owed: the move never happened and the error is the answer.
		"an error inside the callback unwinds the handler": {
			body:    `indri.effects.queue("inside") error("the script gave up", 0)`,
			wantErr: "the script gave up",
			want:    nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store, publisher, id := newTestGame(t)
			log := newDeliveryLog()

			res, err := invokeWithEffects(t, log, store, id, `
indri.on("move", function(req)
  indri.effects.queue("before")
  indri.mutate(function(state)
    `+test.body+`
  end)
  indri.effects.queue("after")
end)
`)

			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("the script failed: %v", err)
			case test.wantErr != "" && err == nil:
				t.Fatal("the failing script reported success")
			case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("the error %q does not carry the script's own message", err.Error())
			}

			if got := log.deliveredMarks(); !slices.Equal(got, test.want) {
				t.Fatalf("delivered %v, want %v — the mutation stored nothing", got, test.want)
			}

			if got := responseMarks(res); !slices.Equal(got, test.want) {
				t.Fatalf("the caller was answered with %v, want %v", got, test.want)
			}

			// The ledger's business is effects, but a dropped effect only makes
			// sense if the write really was dropped too.
			if got := publisher.count(); got != 0 {
				t.Fatalf("published %d events for a mutation that stored nothing, want 0", got)
			}
		})
	}
}

// --- outside a mutation --------------------------------------------------------

// A handler that never mutates still queues: it has nothing to lose a race to,
// so its effects go straight to the committed level. They are still deferred —
// delivery happens when the handler returns, never inline — because a script
// that has not finished may yet error, and because inline delivery of a
// script-emitted event is how an event system recurses without bound.
func TestEffects_QueuedOutsideAMutateFlushWhenTheHandlerReturns(t *testing.T) {
	store, _, id := newTestGame(t)
	log := newDeliveryLog()

	res, err := invokeWithEffects(t, log, store, id, `
indri.on("move", function(req)
  indri.effects.queue("first")
  indri.effects.queue("second")
  if indri.effects.delivered() ~= 0 then
    error("an effect was delivered while the handler was still running", 0)
  end
end)
`)
	if err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	want := []string{"first", "second"}
	if got := log.deliveredMarks(); !slices.Equal(got, want) {
		t.Fatalf("delivered %v, want %v", got, want)
	}

	if got := responseMarks(res); !slices.Equal(got, want) {
		t.Fatalf("the caller was answered with %v, want %v in that order", got, want)
	}
}

// A handler that unwinds is answered with its error, not with the frames it had
// lined up on the way to failing. Nothing it queued is delivered.
func TestEffects_AFailedHandlerDeliversNothing(t *testing.T) {
	store, _, id := newTestGame(t)
	log := newDeliveryLog()

	res, err := invokeWithEffects(t, log, store, id, `
indri.on("move", function(req)
  indri.effects.queue("first")
  error("the script gave up", 0)
end)
`)

	if err == nil {
		t.Fatal("the failing script reported success")
	}

	if got := log.queuedMarks(); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("the script queued %v, want it to have queued exactly [first]", got)
	}

	if got := log.deliveredMarks(); got != nil {
		t.Fatalf("delivered %v for a handler that failed, want nothing", got)
	}

	if got := responseMarks(res); len(got) != 0 {
		t.Fatalf("the caller was answered with %v, want nothing", got)
	}
}

// --- the ledger itself ---------------------------------------------------------

// ledgerOp is one step in a ledger sequence, so the orderings below read as the
// sequence of events they describe rather than as a wall of method calls.
type ledgerOp func(*ledger, *deliveryLog)

func queueMark(mark string) ledgerOp {
	return func(l *ledger, log *deliveryLog) {
		l.queue(markEffect{mark: mark, log: log})
	}
}

func beginAttempt(l *ledger, _ *deliveryLog) { l.beginAttempt() }

func commitAttempt(l *ledger, _ *deliveryLog) { l.commitAttempt() }

func discardAttempt(l *ledger, _ *deliveryLog) { l.discardAttempt() }

// Order is the property that survives both levels: an effect queued before a
// mutation, one queued inside it and one queued after it are delivered in that
// order, whatever the mutation did. These are the level transitions the mutate
// bracket drives, asserted directly so a failure names which transition broke.
func TestLedger_PreservesOrderAcrossBothLevels(t *testing.T) {
	tests := map[string]struct {
		ops  []ledgerOp
		want []string
	}{
		"a committed attempt keeps its place in the queue": {
			ops:  []ledgerOp{queueMark("before"), beginAttempt, queueMark("inside"), commitAttempt, queueMark("after")},
			want: []string{"before", "inside", "after"},
		},
		"a discarded attempt leaves the order around it intact": {
			ops:  []ledgerOp{queueMark("before"), beginAttempt, queueMark("inside"), discardAttempt, queueMark("after")},
			want: []string{"before", "after"},
		},
		"each attempt starts from empty": {
			ops: []ledgerOp{
				beginAttempt, queueMark("first try"),
				beginAttempt, queueMark("second try"),
				commitAttempt,
			},
			want: []string{"second try"},
		},
		"two committed attempts stay in the order they ran": {
			ops: []ledgerOp{
				beginAttempt, queueMark("one"), commitAttempt,
				beginAttempt, queueMark("two"), commitAttempt,
			},
			want: []string{"one", "two"},
		},
		"an attempt still open at flush is dropped": {
			ops:  []ledgerOp{queueMark("outside"), beginAttempt, queueMark("inside")},
			want: []string{"outside"},
		},
		"a ledger nothing was queued on delivers nothing": {
			ops:  nil,
			want: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var l ledger

			log := newDeliveryLog()

			for _, op := range test.ops {
				op(&l, log)
			}

			var res actions.Result

			if err := l.flush(context.Background(), &res); err != nil {
				t.Fatalf("flushing: %v", err)
			}

			if got := log.deliveredMarks(); !slices.Equal(got, test.want) {
				t.Fatalf("delivered %v, want %v", got, test.want)
			}

			if got := responseMarks(res); !slices.Equal(got, test.want) {
				t.Fatalf("answered with %v, want %v", got, test.want)
			}
		})
	}
}

// The ledger hands each effect over once. It cannot make the transport deliver
// it once — that guarantee is best-effort for a frame and at-least-once for a
// timer — but a second flush must not replay what the first one already
// released, or a retry anywhere above this would double every reply in the
// queue.
func TestLedger_ASecondFlushReleasesNothingAgain(t *testing.T) {
	var l ledger

	log := newDeliveryLog()
	l.queue(markEffect{mark: "once", log: log})

	var first, second actions.Result

	if err := l.flush(context.Background(), &first); err != nil {
		t.Fatalf("the first flush: %v", err)
	}

	if err := l.flush(context.Background(), &second); err != nil {
		t.Fatalf("the second flush: %v", err)
	}

	if got := log.deliveredMarks(); !slices.Equal(got, []string{"once"}) {
		t.Fatalf("delivered %v, want [once]", got)
	}

	if got := responseMarks(second); len(got) != 0 {
		t.Fatalf("the second flush answered with %v, want nothing", got)
	}
}

// One effect that cannot be delivered must not silence the ones behind it. The
// effects in an invocation are independent — a broadcast that failed says
// nothing about whether a timer can be scheduled — and a timer dropped because
// an earlier reply failed would hang the game.
func TestLedger_AFailedDeliveryDoesNotStopTheRest(t *testing.T) {
	var l ledger

	log := newDeliveryLog()
	broken := errors.New("the socket is gone")
	log.fails["second"] = broken

	for _, mark := range []string{"first", "second", "third"} {
		l.queue(markEffect{mark: mark, log: log})
	}

	var res actions.Result

	err := l.flush(context.Background(), &res)
	if err == nil {
		t.Fatal("a failed delivery was reported as a success")
	}

	if !errors.Is(err, broken) {
		t.Fatalf("the error %q does not carry the delivery's own failure", err.Error())
	}

	want := []string{"first", "second", "third"}
	if got := log.deliveredMarks(); !slices.Equal(got, want) {
		t.Fatalf("delivered %v, want every effect attempted: %v", got, want)
	}

	// The failed one answered with nothing; the other two still answered.
	if got := responseMarks(res); !slices.Equal(got, []string{"first", "third"}) {
		t.Fatalf("answered with %v, want [first third]", got)
	}
}

// An effect that queues while the ledger is flushing has nowhere to go: the
// invocation is over, and what it queued would otherwise sit in the buffer
// until the state served somebody else. Saying so is the difference between a
// bug in a future host function and a frame delivered to the wrong player.
func TestLedger_AnEffectQueuedDuringAFlushIsReported(t *testing.T) {
	var l ledger

	log := newDeliveryLog()
	l.queue(queueingEffect{ledger: &l, log: log})

	var res actions.Result

	err := l.flush(context.Background(), &res)
	if err == nil {
		t.Fatal("an effect queued during the flush was accepted silently")
	}

	if !strings.Contains(err.Error(), "must not queue another") {
		t.Fatalf("the error %q does not explain the rule that was broken", err.Error())
	}

	if got := log.deliveredMarks(); got != nil {
		t.Fatalf("delivered %v, want nothing — the late effect must not be released", got)
	}

	// And it is gone, rather than left behind for the next flush to find.
	var again actions.Result

	if err := l.flush(context.Background(), &again); err != nil {
		t.Fatalf("the second flush: %v", err)
	}

	if got := responseMarks(again); len(got) != 0 {
		t.Fatalf("the second flush answered with %v, want nothing", got)
	}
}

// queueEffect is the seam every host function with a side effect goes through.
// Reached outside a call it must say so, rather than dereferencing a nil
// invocation or quietly buffering onto a ledger nobody will flush.
func TestQueueEffect_RefusesWhenNoCallIsInProgress(t *testing.T) {
	L := lua.NewState()
	t.Cleanup(L.Close)

	log := newDeliveryLog()

	err := queueEffect(L, markEffect{mark: "orphan", log: log})
	if err == nil {
		t.Fatal("an effect was queued with no request in progress")
	}

	if !strings.Contains(err.Error(), "no request in progress") {
		t.Fatalf("the error %q does not say there is no request in progress", err.Error())
	}

	if got := log.queuedMarks(); got != nil {
		t.Fatalf("the log recorded %v for a refused queue, want nothing", got)
	}
}

// A pooled state serves one invocation after another, and each gets its own
// ledger. Without that, a reply queued by one player's move could be delivered
// to the next caller the state happens to serve.
func TestEffects_EachInvocationGetsItsOwnLedger(t *testing.T) {
	store, _, id := newTestGame(t)
	log := newDeliveryLog()

	path := writeScript(t, "game.lua", `
indri.on("move", function(req)
  indri.effects.queue(req.payload.mark)
end)
`)

	e, err := newEngine(
		[]models.ScriptFile{{Path: path, Grants: []string{effectsName}}},
		store,
		capabilitySet{effectsName: effectsCapability(log)},
	)
	if err != nil {
		t.Fatalf("building an engine over %v: %v", path, err)
	}

	t.Cleanup(e.Close)

	for _, mark := range []string{"first call", "second call"} {
		res, invokeErr := e.Invoke(context.Background(), "move", actions.Request{
			Session: &models.Session{UserID: stringPtr("player-1"), GameID: &id},
			Payload: map[string]interface{}{"mark": mark},
		})
		if invokeErr != nil {
			t.Fatalf("invoking with %q: %v", mark, invokeErr)
		}

		if got := responseMarks(res); !slices.Equal(got, []string{mark}) {
			t.Fatalf("the %q invocation was answered with %v, want exactly [%s]", mark, got, mark)
		}
	}

	want := []string{"first call", "second call"}
	if got := log.deliveredMarks(); !slices.Equal(got, want) {
		t.Fatalf("delivered %v across the two calls, want %v", got, want)
	}
}

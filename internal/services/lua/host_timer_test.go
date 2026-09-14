package lua

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// timerLogName is the capability these tests inject alongside the effects one.
//
// It exists for a single function, indri.timerlog.stored, and that function is
// what makes "queued, then written" distinguishable from "written inline". Both
// arrangements leave the same store behind once the handler has returned; only a
// script that can ask while it is still running can tell them apart.
const timerLogName = "timerlog"

// storedTimer is one timer as the schedule store received it.
type storedTimer struct {
	id      string
	gameID  string
	action  string
	payload map[string]interface{}
	fireAt  time.Time
}

// timerLog stands in for the schedule store behind indri.after and indri.cancel.
//
// It records every operation in one ordered list as well as separately, because
// the order between a schedule and a cancel is itself a guarantee: a script that
// sets a timer and calls it off again in the same handler must not have the
// cancel run first and leave the timer behind.
//
// Ids are minted as timer-1, timer-2, … rather than as ObjectIDs. Nothing above
// the schedule repo knows what an id is made of, and a readable one makes "the
// committing attempt's id, not the losing attempt's" an assertion a reader can
// check by eye.
type timerLog struct {
	mu sync.Mutex

	minted    []string
	scheduled []storedTimer
	cancelled []storedTimer
	ops       []string

	// failSchedule makes the store refuse, so what a script sees when a timer
	// cannot be written can be asserted rather than assumed.
	failSchedule error
}

func newTimerLog() *timerLog {
	return &timerLog{}
}

func (l *timerLog) NewTimerID() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := fmt.Sprintf("timer-%d", len(l.minted)+1)
	l.minted = append(l.minted, id)

	return id
}

func (l *timerLog) ScheduleTimer(
	id, gameID, action string,
	payload map[string]interface{},
	fireAt time.Time,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.failSchedule != nil {
		return l.failSchedule
	}

	l.scheduled = append(l.scheduled, storedTimer{
		id:      id,
		gameID:  gameID,
		action:  action,
		payload: payload,
		fireAt:  fireAt,
	})
	l.ops = append(l.ops, "schedule:"+id)

	return nil
}

func (l *timerLog) CancelTimer(gameID, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cancelled = append(l.cancelled, storedTimer{id: id, gameID: gameID})
	l.ops = append(l.ops, "cancel:"+id)

	return nil
}

func (l *timerLog) mintedIDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.minted)
}

func (l *timerLog) storedTimers() []storedTimer {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.scheduled)
}

func (l *timerLog) cancelledTimers() []storedTimer {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.cancelled)
}

func (l *timerLog) operations() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.ops)
}

func (l *timerLog) storedCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.scheduled)
}

// timerLogCapability installs indri.timerlog.stored, which reports how many
// timers have actually reached the store so far.
func timerLogCapability(log *timerLog) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		tbl := L.NewTable()

		tbl.RawSetString("stored", L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LNumber(log.storedCount()))

			return 1
		}))

		return tbl, nil
	}
}

// newTimerEngine builds an engine over src, wired to log as its scheduler.
//
// Two capabilities are granted: effects, so a timer can be interleaved with
// other queued effects and the ledger's ordering asserted, and timerlog, so a
// script can observe the store while it is still running. indri.after, at and
// cancel are not capabilities — they are on the shared host table — so granting
// these does not affect what is under test.
func newTimerEngine(t *testing.T, src string, games GameMutator, log *timerLog, effects *deliveryLog) *Engine {
	t.Helper()

	if effects == nil {
		effects = newDeliveryLog()
	}

	path := writeScript(t, "game.lua", src)

	e, err := newEngine(
		[]models.ScriptFile{{Path: path, Grants: []string{effectsName, timerLogName}}},
		games,
		capabilitySet{effectsName: onEveryView(effectsCapability(effects)), timerLogName: onEveryView(timerLogCapability(log))},
	)
	if err != nil {
		t.Fatalf("building an engine over %v: %v", path, err)
	}

	t.Cleanup(e.Close)

	if log != nil {
		e.Timers = log
	}

	return e
}

// invokeTimerMove dispatches "move" for a player in gameID.
func invokeTimerMove(t *testing.T, e *Engine, gameID string) (actions.Result, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	req := actions.Request{Session: &models.Session{UserID: stringPtr("player-1")}}
	if gameID != "" {
		req.Session.GameID = &gameID
	}

	return splitScriptError(e.Invoke(ctx, "move", req))
}

// timerHandler wraps a move body in the two registrations every test here
// needs: the move itself, and a "turn_timeout" script action for a timer to be
// allowed to name.
func timerHandler(body string) string {
	return `
indri.on("turn_timeout", function(req) end)

indri.on("move", function(req)
` + body + `
end)
`
}

// --- what reaches the store ----------------------------------------------------

// Nothing is written while the handler is running. This is the property the
// ledger exists for, and it is the only one a script can observe directly: once
// the call has returned, an inline write and a queued one leave the same store
// behind.
func TestAfter_IsNotWrittenUntilTheHandlerReturns(t *testing.T) {
	log := newTimerLog()

	e := newTimerEngine(t, timerHandler(`
  indri.after(30, "turn_timeout", {})

  if indri.timerlog.stored() ~= 0 then
    error("the timer was written inline, while the handler was still running")
  end
`), nil, log, nil)

	if _, err := invokeTimerMove(t, e, "game-1"); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if got := log.storedCount(); got != 1 {
		t.Fatalf("%d timers were stored after the handler returned, want 1", got)
	}
}

// A timer is stamped with the caller's own game, carries the payload the script
// gave it, and fires at the moment the script asked for — whichever of the two
// ways it asked.
func TestTimers_StampTheCallersGameAndTheFireTimeAskedFor(t *testing.T) {
	absolute, err := time.Parse(time.RFC3339, "2026-09-12T18:00:00Z")
	if err != nil {
		t.Fatalf("parsing the fixture timestamp: %v", err)
	}

	tests := map[string]struct {
		body string
		// wantFireAt is checked against the window the call ran in when relative
		// is true, and exactly otherwise.
		relative   bool
		relativeBy time.Duration
		wantFireAt time.Time
	}{
		"after, relative seconds": {
			body:       `indri.after(30, "turn_timeout", { player = "p1" })`,
			relative:   true,
			relativeBy: 30 * time.Second,
		},
		"after, fractional seconds": {
			body:       `indri.after(0.5, "turn_timeout", { player = "p1" })`,
			relative:   true,
			relativeBy: 500 * time.Millisecond,
		},
		"at, absolute timestamp": {
			body:       `indri.at("2026-09-12T18:00:00Z", "turn_timeout", { player = "p1" })`,
			wantFireAt: absolute,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			log := newTimerLog()
			e := newTimerEngine(t, timerHandler("  "+test.body), nil, log, nil)

			before := time.Now()

			if _, err := invokeTimerMove(t, e, "game-1"); err != nil {
				t.Fatalf("the script failed: %v", err)
			}

			after := time.Now()

			stored := log.storedTimers()
			if len(stored) != 1 {
				t.Fatalf("stored %d timers, want 1", len(stored))
			}

			timer := stored[0]

			if timer.gameID != "game-1" {
				t.Errorf("timer game = %q, want %q — a fired timer has no session and this stamp is all it has", timer.gameID, "game-1")
			}

			if timer.action != "turn_timeout" {
				t.Errorf("timer action = %q, want %q", timer.action, "turn_timeout")
			}

			if got := timer.payload["player"]; got != "p1" {
				t.Errorf("timer payload player = %v, want p1", got)
			}

			if test.relative {
				earliest, latest := before.Add(test.relativeBy), after.Add(test.relativeBy)
				if timer.fireAt.Before(earliest) || timer.fireAt.After(latest) {
					t.Errorf("fire time %v is outside [%v, %v]", timer.fireAt, earliest, latest)
				}

				return
			}

			if !timer.fireAt.Equal(test.wantFireAt) {
				t.Errorf("fire time = %v, want %v", timer.fireAt, test.wantFireAt)
			}
		})
	}
}

// The id a script is handed is the id the entry is stored under, and it is
// handed over early enough to be written into the very state whose commit
// releases the write.
//
// This is what the whole minted-id arrangement is for. If indri.after returned
// anything the store then ignored, a script could keep a handle in game state
// and indri.cancel(that handle) would silently cancel nothing.
func TestAfter_ReturnsTheIdTheEntryIsStoredUnder(t *testing.T) {
	store, _, gameID := newTestGame(t)
	log := newTimerLog()

	e := newTimerEngine(t, timerHandler(`
  indri.mutate(function(g)
    g.data.timer = indri.after(30, "turn_timeout", {})
    return g
  end)
`), store, log, nil)

	if _, err := invokeTimerMove(t, e, gameID); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	stored := log.storedTimers()
	if len(stored) != 1 {
		t.Fatalf("stored %d timers, want 1", len(stored))
	}

	game, err := store.Get(gameID)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	held, _ := game.PublicData["timer"].(string)

	if held == "" {
		t.Fatal("the script stored no timer id, so this test proved nothing")
	}

	if held != stored[0].id {
		t.Fatalf("the game holds timer id %q but the entry was stored under %q", held, stored[0].id)
	}
}

// --- the ledger, through the timers -------------------------------------------

// A timer set inside a mutate that stored nothing describes a move that did not
// happen. Writing it anyway would leave a game counting down to the consequence
// of a move the store rejected.
func TestTimer_InsideAMutateThatStoredNothingIsDiscarded(t *testing.T) {
	tests := map[string]string{
		"the callback returned nil": `
  indri.mutate(function(g)
    indri.after(30, "turn_timeout", {})
    return nil
  end)
`,
		"the callback changed nothing": `
  indri.mutate(function(g)
    indri.after(30, "turn_timeout", {})
    return g
  end)
`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			store, _, gameID := newTestGame(t)
			log := newTimerLog()

			e := newTimerEngine(t, timerHandler(body), store, log, nil)

			if _, err := invokeTimerMove(t, e, gameID); err != nil {
				t.Fatalf("the script failed: %v", err)
			}

			if len(log.mintedIDs()) == 0 {
				t.Fatal("indri.after was never reached, so this test proved nothing")
			}

			if got := log.storedTimers(); len(got) != 0 {
				t.Fatalf("stored %d timers for a mutation that wrote nothing, want 0: %+v", len(got), got)
			}
		})
	}
}

// mutation.Run re-runs its callback on every version-fence miss, so a timer set
// inside one would be stored once per attempt — the same turn timeout queued
// twice, firing twice.
//
// The assertion is not merely "one timer". It is that the surviving timer is the
// one the *committing* attempt minted: the losing attempt's id was thrown away
// with the state it described.
func TestTimer_InsideARetriedMutateIsStoredOncePerCommit(t *testing.T) {
	store, _, gameID := newTestGame(t)
	games := &conflictOnce{inner: store}
	log := newTimerLog()

	e := newTimerEngine(t, timerHandler(`
  indri.mutate(function(g)
    g.data.round = (g.data.round or 0) + 1
    indri.after(30, "turn_timeout", {})
    return g
  end)
`), games, log, nil)

	if _, err := invokeTimerMove(t, e, gameID); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	if !games.fired {
		t.Fatal("the test never forced a conflict, so it proved nothing")
	}

	minted := log.mintedIDs()
	if len(minted) != 2 {
		t.Fatalf("indri.after ran %d times, want 2 — the callback was not retried", len(minted))
	}

	stored := log.storedTimers()
	if len(stored) != 1 {
		t.Fatalf("stored %d timers across %d attempts, want 1", len(stored), len(minted))
	}

	if stored[0].id != minted[1] {
		t.Fatalf(
			"the stored timer is %q, the losing attempt's id; want %q, the committing attempt's",
			stored[0].id, minted[1],
		)
	}
}

// A cancel is queued alongside the timers rather than performed inline, so a
// handler that sets a timer and calls it off again leaves the store in the order
// the script wrote, not the reverse.
func TestCancel_RunsAfterTheTimerItCallsOff(t *testing.T) {
	log := newTimerLog()

	e := newTimerEngine(t, timerHandler(`
  local id = indri.after(30, "turn_timeout", {})
  indri.cancel(id)
`), nil, log, nil)

	if _, err := invokeTimerMove(t, e, "game-1"); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	want := []string{"schedule:timer-1", "cancel:timer-1"}

	if got := log.operations(); !slices.Equal(got, want) {
		t.Fatalf("store operations = %v, want %v", got, want)
	}
}

// A cancel carries the caller's own game, which is what stops a script passing
// a player-supplied id straight through from reaching another game's timers.
func TestCancel_IsStampedWithTheCallersGame(t *testing.T) {
	log := newTimerLog()

	e := newTimerEngine(t, timerHandler(`indri.cancel(req.payload.id or "timer-9")`), nil, log, nil)

	if _, err := invokeTimerMove(t, e, "game-1"); err != nil {
		t.Fatalf("the script failed: %v", err)
	}

	cancelled := log.cancelledTimers()
	if len(cancelled) != 1 {
		t.Fatalf("cancelled %d timers, want 1", len(cancelled))
	}

	if cancelled[0].gameID != "game-1" {
		t.Fatalf("cancel game = %q, want %q", cancelled[0].gameID, "game-1")
	}
}

// --- refusals ------------------------------------------------------------------

// The contract the whole feature rests on: a timer fires with no session, every
// built-in action rejects a nil session, and an action nobody registered is
// worse still — router.Dispatch succeeds quietly when nothing matches, so the
// timer would fire, do nothing, and be marked as delivered.
func TestTimers_RefuseAnActionATimerCouldNotFire(t *testing.T) {
	tests := map[string]struct {
		action string
		wants  []string
	}{
		"a built-in action":  {action: "login", wants: []string{"built-in", "no session"}},
		"kick":               {action: "kick", wants: []string{"built-in", "no session"}},
		"a dispatch phase":   {action: "received", wants: []string{"every message"}},
		"nothing registered": {action: "no_such_action", wants: []string{"no script handler is registered"}},
		"an empty name":      {action: "", wants: []string{"cannot be empty"}},
	}

	for name, test := range tests {
		for _, fn := range []string{"after", "at"} {
			t.Run(name+"/"+fn, func(t *testing.T) {
				log := newTimerLog()

				call := fmt.Sprintf("indri.after(30, %q, {})", test.action)
				if fn == "at" {
					call = fmt.Sprintf("indri.at(%q, %q, {})", "2026-09-12T18:00:00Z", test.action)
				}

				e := newTimerEngine(t, timerHandler("  "+call), nil, log, nil)

				_, err := invokeTimerMove(t, e, "game-1")
				if err == nil {
					t.Fatalf("scheduling the action %q was accepted", test.action)
				}

				requireErrorMentions(t, err, test.wants...)

				if got := log.storedCount(); got != 0 {
					t.Fatalf("stored %d timers for a refused action, want 0", got)
				}
			})
		}
	}

	// The control: the same call against an action a script did register is
	// accepted, so the rows above are failing on the name and not on the shape
	// of the call.
	t.Run("a script action is accepted", func(t *testing.T) {
		log := newTimerLog()
		e := newTimerEngine(t, timerHandler(`indri.after(30, "turn_timeout", {})`), nil, log, nil)

		if _, err := invokeTimerMove(t, e, "game-1"); err != nil {
			t.Fatalf("scheduling a script action was refused: %v", err)
		}

		if got := log.storedCount(); got != 1 {
			t.Fatalf("stored %d timers, want 1", got)
		}
	})
}

// A delay that cannot mean what it says is a script bug, and it is caught at the
// line that made it rather than turning into a timer that fires at the wrong
// moment — or, for a value large enough to wrap time.Duration, immediately.
func TestAfter_RefusesADelayItCannotMean(t *testing.T) {
	tests := map[string]struct {
		call  string
		wants []string
	}{
		"negative": {call: `indri.after(-30, "turn_timeout", {})`, wants: []string{"cannot be negative"}},
		"nan":      {call: `indri.after(0/0, "turn_timeout", {})`, wants: []string{"finite"}},
		"infinite": {call: `indri.after(1/0, "turn_timeout", {})`, wants: []string{"finite"}},
		"past a year": {
			call:  `indri.after(60 * 60 * 24 * 400, "turn_timeout", {})`,
			wants: []string{"further ahead", "milliseconds"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			log := newTimerLog()
			e := newTimerEngine(t, timerHandler("  "+test.call), nil, log, nil)

			_, err := invokeTimerMove(t, e, "game-1")
			if err == nil {
				t.Fatal("the delay was accepted")
			}

			requireErrorMentions(t, err, test.wants...)

			if got := log.storedCount(); got != 0 {
				t.Fatalf("stored %d timers for a refused delay, want 0", got)
			}
		})
	}
}

// indri.at takes RFC 3339 and nothing else, so a value in another format is a
// named error rather than a timer at an hour nobody chose.
func TestAt_RefusesATimestampItCannotParse(t *testing.T) {
	for _, stamp := range []string{"tomorrow", "2026-09-12", "12/09/2026 18:00", ""} {
		t.Run(stamp, func(t *testing.T) {
			log := newTimerLog()
			e := newTimerEngine(t, timerHandler(fmt.Sprintf("  indri.at(%q, %q, {})", stamp, "turn_timeout")), nil, log, nil)

			_, err := invokeTimerMove(t, e, "game-1")
			if err == nil {
				t.Fatalf("the timestamp %q was accepted", stamp)
			}

			requireErrorMentions(t, err, "RFC 3339")

			if got := log.storedCount(); got != 0 {
				t.Fatalf("stored %d timers for an unparseable timestamp, want 0", got)
			}
		})
	}
}

// Queueing is cheap and the cost lands later, on a shared collection the
// scheduler has to drain. A handler looping over a list it got from a client
// must not be able to fill it.
func TestTimers_AreCappedPerInvocation(t *testing.T) {
	tests := map[string]string{
		"setting":    fmt.Sprintf("for i = 1, %d do indri.after(30, \"turn_timeout\", {}) end", maxTimerOps+1),
		"cancelling": fmt.Sprintf("for i = 1, %d do indri.cancel(\"timer-\" .. i) end", maxTimerOps+1),
		"a mixture": fmt.Sprintf(
			"for i = 1, %d do indri.after(30, \"turn_timeout\", {}) end\n  for i = 1, %d do indri.cancel(\"timer-\" .. i) end",
			maxTimerOps, maxTimerOps,
		),
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			log := newTimerLog()
			e := newTimerEngine(t, timerHandler("  "+body), nil, log, nil)

			_, err := invokeTimerMove(t, e, "game-1")
			if err == nil {
				t.Fatalf("a handler was allowed more than %d timer operations", maxTimerOps)
			}

			requireErrorMentions(t, err, fmt.Sprintf("%d", maxTimerOps))

			// The handler unwound, so nothing it queued is owed to anybody.
			if got := log.storedCount(); got != 0 {
				t.Fatalf("stored %d timers for a handler that failed, want 0", got)
			}
		})
	}
}

// An engine with no scheduler refuses rather than dropping the timer. A dropped
// one hangs the game it belonged to, with nothing in the log to say why.
func TestTimers_RefuseWhenTheEngineHasNoScheduler(t *testing.T) {
	for name, body := range map[string]string{
		"after":  `indri.after(30, "turn_timeout", {})`,
		"at":     `indri.at("2026-09-12T18:00:00Z", "turn_timeout", {})`,
		"cancel": `indri.cancel("timer-1")`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newTimerEngine(t, timerHandler("  "+body), nil, nil, nil)

			_, err := invokeTimerMove(t, e, "game-1")
			if err == nil {
				t.Fatal("the call was accepted on an engine with no scheduler")
			}

			requireErrorMentions(t, err, "without a scheduler")
		})
	}
}

// A timer is stamped with the caller's game, so a caller who is in no game has
// nothing to stamp. Storing one anyway would produce an entry whose fired
// action could never find its state.
func TestTimers_RefuseACallerWithNoGame(t *testing.T) {
	for name, body := range map[string]string{
		"after":  `indri.after(30, "turn_timeout", {})`,
		"at":     `indri.at("2026-09-12T18:00:00Z", "turn_timeout", {})`,
		"cancel": `indri.cancel("timer-1")`,
	} {
		t.Run(name, func(t *testing.T) {
			log := newTimerLog()
			e := newTimerEngine(t, timerHandler("  "+body), nil, log, nil)

			_, err := invokeTimerMove(t, e, "")
			if err == nil {
				t.Fatal("the call was accepted for a caller who is in no game")
			}

			requireErrorMentions(t, err, "not in a game")

			if got := log.storedCount(); got != 0 {
				t.Fatalf("stored %d timers for a caller with no game, want 0", got)
			}
		})
	}
}

// An id is a string a script holds, and an empty one is a script that lost track
// of its timer rather than a timer that does not exist.
func TestCancel_RefusesAnEmptyId(t *testing.T) {
	log := newTimerLog()
	e := newTimerEngine(t, timerHandler(`indri.cancel("   ")`), nil, log, nil)

	_, err := invokeTimerMove(t, e, "game-1")
	if err == nil {
		t.Fatal("an empty timer id was accepted")
	}

	requireErrorMentions(t, err, "cannot be empty")
}

// --- req.gameId ----------------------------------------------------------------

// req.gameId is the field a handler reads whether or not anybody is connected.
// A timer fires with no session, so a script reading req.session.gameId would
// fault on exactly the dispatches that most need to find their game.
func TestRequest_ExposesTheGameIdWithAndWithoutASession(t *testing.T) {
	tests := map[string]struct {
		session *models.Session
		ctx     func(context.Context) context.Context
		want    string
	}{
		"from the caller's session": {
			session: &models.Session{UserID: stringPtr("player-1"), GameID: stringPtr("game-1")},
			want:    "game-1",
		},
		"from the dispatch, with no session at all": {
			ctx:  func(ctx context.Context) context.Context { return actions.WithGameID(ctx, "game-7") },
			want: "game-7",
		},
		"the session wins over the dispatch": {
			session: &models.Session{UserID: stringPtr("player-1"), GameID: stringPtr("game-1")},
			ctx:     func(ctx context.Context) context.Context { return actions.WithGameID(ctx, "game-7") },
			want:    "game-1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var seen string

			log := newTimerLog()
			e := newTimerEngine(t, `
indri.on("turn_timeout", function(req) end)

indri.on("move", function(req)
  indri.effects.queue(req.gameId or "<absent>")
end)
`, nil, log, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if test.ctx != nil {
				ctx = test.ctx(ctx)
			}

			res, err := splitScriptError(e.Invoke(ctx, "move", actions.Request{
				Context: ctx,
				Action:  "move",
				Session: test.session,
			}))
			if err != nil {
				t.Fatalf("the script failed: %v", err)
			}

			if marks := responseMarks(res); len(marks) == 1 {
				seen = marks[0]
			}

			if seen != test.want {
				t.Fatalf("req.gameId = %q, want %q", seen, test.want)
			}
		})
	}
}

// The engine reports a store that refused rather than swallowing it. A timer
// that was never written is a game that will never be prodded again, and the
// only trace is this log line.
func TestAfter_AStoreThatRefusesIsReported(t *testing.T) {
	log := newTimerLog()
	log.failSchedule = fmt.Errorf("the collection is unavailable")

	e := newTimerEngine(t, timerHandler(`indri.after(30, "turn_timeout", {})`), nil, log, nil)

	// The handler itself succeeded — the failure is in delivery, after it
	// returned — so the invocation reports no error to the caller, exactly as a
	// failed publish does. What must not happen is the effect reporting success.
	res, err := invokeTimerMove(t, e, "game-1")
	if err != nil {
		t.Fatalf("a delivery failure was reported as a handler failure: %v", err)
	}

	if len(res.Responses) != 0 {
		t.Fatalf("the caller was answered with %d frames, want 0", len(res.Responses))
	}

	// Proved directly against the effect, since flush only logs: the error it
	// produces has to name the action so an operator can find the script.
	effect := timerEffect{id: "timer-1", gameID: "game-1", action: "turn_timeout", timers: log}

	deliverErr := effect.deliver(context.Background(), &actions.Result{})
	if deliverErr == nil {
		t.Fatal("a store that refused was reported as a success")
	}

	if !strings.Contains(deliverErr.Error(), "turn_timeout") {
		t.Fatalf("the delivery error does not name the action: %v", deliverErr)
	}
}

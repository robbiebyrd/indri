package scheduler

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/schedule"
)

// testTimeout bounds anything that could hang rather than fail. Run is the
// reason it exists: a loop that ignored its context would otherwise leave a test
// that never returns, which tells a reader nothing.
const testTimeout = 5 * time.Second

// No test here sleeps for a fire time. The scheduler reads one clock — Config.Now
// — and tick takes the moment to evaluate against as an argument, so a test
// advances time by passing a later value rather than by waiting for one. A test
// that slept for a thirty second timer would take thirty seconds; one that slept
// for fifty milliseconds and hoped would fail on a loaded machine.

// firedAction is one dispatch as the router received it.
//
// gameID is read the way a handler reads it — through actions.Request, from the
// context — rather than from anything the scheduler handed over directly, so
// this records what a real handler would actually see.
type firedAction struct {
	action  string
	payload map[string]interface{}
	session *models.Session
	gameID  string
}

// dispatchLog stands in for the router.
type dispatchLog struct {
	mu    sync.Mutex
	calls []firedAction

	// fail, when set, is what every dispatch returns, so the failure path can be
	// driven without a handler.
	fail error

	// panics makes the dispatch panic, standing in for a received/processed hook
	// that dereferences the nil session a timer fires with.
	panics bool

	// fired is signalled after every dispatch, so a test driving Run can wait for
	// an event instead of sleeping.
	fired chan struct{}
}

func newDispatchLog() *dispatchLog {
	return &dispatchLog{fired: make(chan struct{}, 64)}
}

func (d *dispatchLog) dispatcher() Dispatcher {
	return func(
		ctx context.Context,
		session *models.Session,
		action string,
		payload map[string]interface{},
	) (actions.Result, error) {
		req := actions.Request{Context: ctx, Action: action, Session: session, Payload: payload}

		d.mu.Lock()
		d.calls = append(d.calls, firedAction{
			action:  action,
			payload: payload,
			session: session,
			gameID:  req.GameID(),
		})
		panics, fail := d.panics, d.fail
		d.mu.Unlock()

		select {
		case d.fired <- struct{}{}:
		default:
		}

		if panics {
			// Exactly what a hook written against a client message does when it
			// meets a request that has no session.
			_ = *session.UserID
		}

		return actions.Result{}, fail
	}
}

func (d *dispatchLog) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.calls)
}

func (d *dispatchLog) all() []firedAction {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.calls)
}

func (d *dispatchLog) actionNames() []string {
	names := make([]string, 0, d.count())

	for _, call := range d.all() {
		names = append(names, call.action)
	}

	return names
}

// newScheduler builds a scheduler over a fresh in-memory store.
//
// A MemoryStore rather than a stub: claiming, the lease fence, the attempt count
// and the retry backoff are the production ones, shared with the MongoDB store
// through the same pure functions. A stub would have proved only that the test
// and the test agree.
func newScheduler(t *testing.T, log *dispatchLog, cfg Config, actionNames ...string) (*Scheduler, *schedule.MemoryStore) {
	t.Helper()

	store := schedule.NewMemoryStore(schedule.Config{InstanceID: "instance-a"})

	s, err := New(store, log.dispatcher(), actionNames, cfg)
	if err != nil {
		t.Fatalf("building a scheduler: %v", err)
	}

	return s, store
}

func mustSchedule(t *testing.T, store schedule.Storer, gameID, action string, fireAt time.Time) *schedule.Entry {
	t.Helper()

	entry, err := store.Schedule(schedule.CreateEntry{
		GameID:  gameID,
		Action:  action,
		Payload: map[string]interface{}{"player": "p1"},
		FireAt:  fireAt,
	})
	if err != nil {
		t.Fatalf("scheduling %q: %v", action, err)
	}

	return entry
}

func mustGet(t *testing.T, store schedule.Storer, id string) *schedule.Entry {
	t.Helper()

	entry, err := store.Get(id)
	if err != nil {
		t.Fatalf("reloading schedule entry %q: %v", id, err)
	}

	return entry
}

// --- firing --------------------------------------------------------------------

// A timer fires after its fire time and not before, carrying the payload the
// script gave it, and it fires once.
func TestTick_DispatchesADueActionWithItsPayload(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{}, "turn_timeout")

	now := time.Now()
	entry := mustSchedule(t, store, "game-1", "turn_timeout", now.Add(30*time.Second))

	for _, early := range []time.Duration{0, 29 * time.Second} {
		if fired := s.tick(context.Background(), now.Add(early)); fired != 0 {
			t.Fatalf("%d actions fired %v before the fire time, want 0", fired, 30*time.Second-early)
		}
	}

	if fired := s.tick(context.Background(), now.Add(30*time.Second)); fired != 1 {
		t.Fatalf("%d actions fired at the fire time, want 1", fired)
	}

	calls := log.all()
	if len(calls) != 1 {
		t.Fatalf("dispatched %d actions, want 1", len(calls))
	}

	if calls[0].action != "turn_timeout" {
		t.Errorf("dispatched %q, want turn_timeout", calls[0].action)
	}

	if got := calls[0].payload["player"]; got != "p1" {
		t.Errorf("payload player = %v, want p1 — the entry's payload did not reach the handler", got)
	}

	if got := mustGet(t, store, entry.ID.Hex()).State; got != schedule.StateDone {
		t.Errorf("entry state = %q after firing, want %q", got, schedule.StateDone)
	}

	// An acknowledged entry is finished. A second tick long after must not find
	// it again, or every timer in the system would fire on a loop.
	if fired := s.tick(context.Background(), now.Add(time.Hour)); fired != 0 {
		t.Fatalf("%d actions fired again after completion, want 0", fired)
	}
}

// The contract the whole feature rests on: a fired action has no session, and
// finds its game through the stamp the scheduler puts on the dispatch instead.
//
// Both halves are asserted. A nil session alone would pass against a scheduler
// that forgot the game entirely, and a game id alone would pass against one that
// forged a session to carry it.
func TestTick_FiresWithNoSessionAndTheGameStampedOnTheDispatch(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{}, "turn_timeout")

	now := time.Now()
	mustSchedule(t, store, "game-7", "turn_timeout", now)

	if fired := s.tick(context.Background(), now); fired != 1 {
		t.Fatalf("%d actions fired, want 1", fired)
	}

	call := log.all()[0]

	if call.session != nil {
		t.Errorf("a timer fired with a session (%+v); nobody is connected, and forging one would grant authority no player gave", call.session)
	}

	if call.gameID != "game-7" {
		t.Errorf("Request.GameID() = %q, want game-7 — a fired action cannot find its state without it", call.gameID)
	}
}

// --- cancelling ----------------------------------------------------------------

// A cancelled timer does not fire. The uncancelled one beside it does, which is
// what makes this test fail if cancelling were to stop *everything* — or if the
// tick under it had simply done nothing at all.
func TestTick_ACancelledTimerDoesNotFireAndItsNeighbourStillDoes(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{}, "turn_timeout", "round_start")

	now := time.Now()
	cancelled := mustSchedule(t, store, "game-1", "turn_timeout", now.Add(30*time.Second))
	kept := mustSchedule(t, store, "game-1", "round_start", now.Add(30*time.Second))

	timers, err := NewTimers(store)
	if err != nil {
		t.Fatalf("building the timer capability: %v", err)
	}

	if err := timers.CancelTimer("game-1", cancelled.ID.Hex()); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if fired := s.tick(context.Background(), now.Add(time.Hour)); fired != 1 {
		t.Fatalf("%d actions fired, want 1 — only the uncancelled timer", fired)
	}

	if got := log.actionNames(); !slices.Equal(got, []string{"round_start"}) {
		t.Fatalf("dispatched %v, want only round_start", got)
	}

	// The cancelled entry is gone rather than merely skipped: a skipped one would
	// be claimed on some later tick.
	if _, err := store.Get(cancelled.ID.Hex()); err == nil {
		t.Fatal("the cancelled entry is still in the store")
	}

	if got := mustGet(t, store, kept.ID.Hex()).State; got != schedule.StateDone {
		t.Errorf("the uncancelled entry is %q, want %q", got, schedule.StateDone)
	}
}

// --- failure -------------------------------------------------------------------

// A handler that always fails must cost a bounded amount of work and then stop.
//
// "It retried" is not the property under test — a scheduler that retried ten
// thousand times a second would pass that. The three phases below are the
// property: a failure is not re-claimed within its own backoff however hard the
// loop is driven, the retries are bounded by the attempt budget, and a
// dead-lettered entry is never claimed again however far the clock moves.
func TestTick_AFailingActionBacksOffAndThenStopsBeingRetried(t *testing.T) {
	log := newDispatchLog()
	log.fail = fmt.Errorf("the handler refused the move")

	const maxAttempts = 3

	store := schedule.NewMemoryStore(schedule.Config{
		InstanceID:   "instance-a",
		MaxAttempts:  maxAttempts,
		RetryBackoff: 10 * time.Second,
	})

	s, err := New(store, log.dispatcher(), []string{"turn_timeout"}, Config{})
	if err != nil {
		t.Fatalf("building a scheduler: %v", err)
	}

	now := time.Now()
	entry := mustSchedule(t, store, "game-1", "turn_timeout", now)
	id := entry.ID.Hex()

	// Phase 1: hammer the loop at a fixed moment. The backoff pushed fireAt past
	// it, so exactly one dispatch may come of a hundred ticks.
	for range 100 {
		s.tick(context.Background(), now)
	}

	if got := log.count(); got != 1 {
		t.Fatalf("100 ticks at one moment produced %d dispatches, want 1 — the failure is hot-looping", got)
	}

	if got := mustGet(t, store, id).State; got != schedule.StatePending {
		t.Fatalf("entry state = %q after one failure, want %q", got, schedule.StatePending)
	}

	// Phase 2: let the clock run past every backoff. The attempt budget is what
	// bounds the total, so a tenfold-longer run must not raise it.
	for i := range 10 {
		s.tick(context.Background(), now.Add(time.Duration(i+1)*time.Hour))
	}

	if got := log.count(); got != maxAttempts {
		t.Fatalf("%d dispatches over ten backoff windows, want %d — the attempt budget did not hold", got, maxAttempts)
	}

	dead := mustGet(t, store, id)
	if dead.State != schedule.StateDead {
		t.Fatalf("entry state = %q after exhausting its attempts, want %q", dead.State, schedule.StateDead)
	}

	if !strings.Contains(dead.LastError, "refused the move") {
		t.Errorf("the dead entry does not record why it failed: %q", dead.LastError)
	}

	// Phase 3: a dead entry is terminal. A year of ticking adds nothing.
	for i := range 100 {
		s.tick(context.Background(), now.Add(time.Duration(i)*24*time.Hour))
	}

	if got := log.count(); got != maxAttempts {
		t.Fatalf("%d dispatches after the entry was dead-lettered, want %d", got, maxAttempts)
	}
}

// A hook written against a client message dereferences req.Session; a timer
// fires with none. That is the hook's bug, and it must cost the game one timer
// and a log line rather than the scheduler goroutine — which would silently stop
// every timer in the process.
//
// The two entries are the point: the second one proves the loop carried on
// through the first one's panic rather than unwinding out of the tick.
func TestTick_ASessionDereferencingHookDoesNotStopTheLoop(t *testing.T) {
	log := newDispatchLog()
	log.panics = true

	s, store := newScheduler(t, log, Config{}, "turn_timeout")

	now := time.Now()
	first := mustSchedule(t, store, "game-1", "turn_timeout", now)
	second := mustSchedule(t, store, "game-2", "turn_timeout", now)

	if fired := s.tick(context.Background(), now); fired != 2 {
		t.Fatalf("%d entries were attempted, want 2 — the loop unwound on the first panic", fired)
	}

	for _, entry := range []*schedule.Entry{first, second} {
		reloaded := mustGet(t, store, entry.ID.Hex())

		if reloaded.State != schedule.StatePending {
			t.Errorf("entry %s is %q after a panicking handler, want %q", entry.ID.Hex(), reloaded.State, schedule.StatePending)
		}

		if reloaded.Attempts != 1 {
			t.Errorf("entry %s has %d attempts, want 1", entry.ID.Hex(), reloaded.Attempts)
		}

		if !strings.Contains(reloaded.LastError, "panicked") {
			t.Errorf("entry %s does not record the panic: %q", entry.ID.Hex(), reloaded.LastError)
		}
	}

	// And the scheduler is still usable afterwards, which is the property a
	// crashed goroutine would fail.
	log.panics = false

	if fired := s.tick(context.Background(), now.Add(time.Hour)); fired != 2 {
		t.Fatalf("%d entries fired on the next tick, want 2 — the scheduler did not survive", fired)
	}
}

// The same thing through the real router, which is where the received phase
// actually lives. router.invokeHandler recovers a panicking handler into an
// error; this proves the scheduler's dispatch is subject to that and that a
// hook sees the game even when it cannot see a session.
func TestTick_RunsTheDispatchPhasesWithNoSession(t *testing.T) {
	router.Reset()
	t.Cleanup(router.Reset)

	hook := &recordingHook{}
	router.RegisterHandler("hook", "received", hook)
	router.RegisterHandler("timeout", "turn_timeout", &recordingHook{})

	store := schedule.NewMemoryStore(schedule.Config{InstanceID: "instance-a"})

	s, err := New(store, router.Dispatch, []string{"turn_timeout"}, Config{})
	if err != nil {
		t.Fatalf("building a scheduler: %v", err)
	}

	now := time.Now()
	mustSchedule(t, store, "game-7", "turn_timeout", now)

	if fired := s.tick(context.Background(), now); fired != 1 {
		t.Fatalf("%d actions fired, want 1", fired)
	}

	if hook.calls != 1 {
		t.Fatalf("the received hook ran %d times on a timer fire, want 1", hook.calls)
	}

	if hook.sawSession {
		t.Error("the received hook was handed a session on a timer fire")
	}

	if hook.sawGameID != "game-7" {
		t.Errorf("the received hook read req.GameID() = %q, want game-7", hook.sawGameID)
	}
}

// recordingHook is a handler registered under the received phase.
type recordingHook struct {
	calls      int
	sawSession bool
	sawGameID  string
}

func (h *recordingHook) Handle(req actions.Request) (actions.Result, error) {
	h.calls++
	h.sawSession = req.Session != nil
	h.sawGameID = req.GameID()

	return actions.Result{}, nil
}

// --- what a timer may name -----------------------------------------------------

// An entry outlives the script that wrote it, so the name is checked again here.
// A built-in cannot be dispatched — it would reject the nil session — and an
// action nobody registered is worse, because router.Dispatch succeeds quietly
// when nothing matches and the timer would be marked delivered having done
// nothing.
func TestTick_DeadLettersAnActionNoScriptCanHandle(t *testing.T) {
	tests := map[string]string{
		"a built-in action":                  "kick",
		"another built-in action":            "login",
		"a dispatch phase":                   "received",
		"an action a script upgrade removed": "retired_action",
	}

	for name, action := range tests {
		t.Run(name, func(t *testing.T) {
			log := newDispatchLog()
			s, store := newScheduler(t, log, Config{}, "turn_timeout")

			now := time.Now()
			entry := mustSchedule(t, store, "game-1", action, now)

			s.tick(context.Background(), now)

			if got := log.count(); got != 0 {
				t.Fatalf("the action %q was dispatched %d times, want 0", action, got)
			}

			reloaded := mustGet(t, store, entry.ID.Hex())

			if reloaded.State != schedule.StateDead {
				t.Fatalf("entry state = %q, want %q — a name that cannot work must not be retried", reloaded.State, schedule.StateDead)
			}

			if !strings.Contains(reloaded.LastError, action) {
				t.Errorf("the dead entry does not name the refused action: %q", reloaded.LastError)
			}

			// Terminal: no later tick picks it up again.
			for i := range 10 {
				s.tick(context.Background(), now.Add(time.Duration(i+1)*time.Hour))
			}

			if got := log.count(); got != 0 {
				t.Fatalf("the dead-lettered action was dispatched %d times later, want 0", got)
			}
		})
	}

	// The control: a script action on the same store is dispatched, so the rows
	// above fail on the name and not because nothing ever fires.
	t.Run("a script action is dispatched", func(t *testing.T) {
		log := newDispatchLog()
		s, store := newScheduler(t, log, Config{}, "turn_timeout")

		now := time.Now()
		mustSchedule(t, store, "game-1", "turn_timeout", now)

		if fired := s.tick(context.Background(), now); fired != 1 {
			t.Fatalf("%d script actions fired, want 1", fired)
		}
	})
}

// --- the loop ------------------------------------------------------------------

// A backlog is drained across ticks rather than in one unbroken loop, so a
// shutdown does not have to wait for it.
func TestTick_StopsAtThePerTickBudget(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{MaxPerTick: 3}, "turn_timeout")

	now := time.Now()

	for range 7 {
		mustSchedule(t, store, "game-1", "turn_timeout", now)
	}

	if fired := s.tick(context.Background(), now); fired != 3 {
		t.Fatalf("one tick fired %d entries, want 3", fired)
	}

	if fired := s.tick(context.Background(), now); fired != 3 {
		t.Fatalf("the second tick fired %d entries, want 3", fired)
	}

	if fired := s.tick(context.Background(), now); fired != 1 {
		t.Fatalf("the third tick fired %d entries, want the 1 that was left", fired)
	}
}

// A cancelled context stops a tick part-way through a backlog rather than after
// it.
func TestTick_AbandonsABacklogOnCancellation(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{}, "turn_timeout")

	now := time.Now()

	for range 5 {
		mustSchedule(t, store, "game-1", "turn_timeout", now)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if fired := s.tick(ctx, now); fired != 0 {
		t.Fatalf("a cancelled tick fired %d entries, want 0", fired)
	}

	if got := log.count(); got != 0 {
		t.Fatalf("a cancelled tick dispatched %d actions, want 0", got)
	}
}

// Run polls until its context is cancelled and then returns nil, so a clean
// shutdown does not look like a failure to the errgroup that is waiting on it.
//
// It is proved to have been running first. "Returned on cancellation" is
// trivially true of a Run that returned immediately and fired nothing.
func TestRun_FiresUntilItsContextIsCancelled(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{Interval: time.Millisecond}, "turn_timeout")

	mustSchedule(t, store, "game-1", "turn_timeout", time.Now().Add(-time.Minute))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- s.Run(ctx) }()

	select {
	case <-log.fired:
	case <-time.After(testTimeout):
		cancel()
		t.Fatal("Run never dispatched the entry that was already due")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run reported a clean shutdown as an error: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("Run did not return after its context was cancelled")
	}

	// Nothing is dispatched after Run has returned. A goroutine still polling
	// would keep firing whatever was left.
	settled := log.count()

	mustSchedule(t, store, "game-1", "round_start", time.Now().Add(-time.Minute))

	select {
	case <-log.fired:
		t.Fatal("an action fired after Run returned")
	case <-time.After(50 * time.Millisecond):
	}

	if got := log.count(); got != settled {
		t.Fatalf("%d actions dispatched after Run returned, want 0", got-settled)
	}
}

// --- construction --------------------------------------------------------------

func TestNew_RefusesMissingDependencies(t *testing.T) {
	store := schedule.NewMemoryStore(schedule.Config{})
	log := newDispatchLog()

	tests := map[string]struct {
		entries  schedule.Storer
		dispatch Dispatcher
	}{
		"no store":      {dispatch: log.dispatcher()},
		"no dispatcher": {entries: store},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(test.entries, test.dispatch, nil, Config{}); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

// A server running no scripts schedules nothing, so an empty manifest is not an
// error — but any entry that somehow exists must still be refused rather than
// dispatched into silence.
func TestNew_AnEmptyManifestFiresNothing(t *testing.T) {
	log := newDispatchLog()
	s, store := newScheduler(t, log, Config{})

	now := time.Now()
	entry := mustSchedule(t, store, "game-1", "turn_timeout", now)

	s.tick(context.Background(), now)

	if got := log.count(); got != 0 {
		t.Fatalf("dispatched %d actions with no script actions registered, want 0", got)
	}

	if got := mustGet(t, store, entry.ID.Hex()).State; got != schedule.StateDead {
		t.Fatalf("entry state = %q, want %q", got, schedule.StateDead)
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	filled := Config{}.withDefaults()

	if filled.Interval != defaultInterval {
		t.Errorf("interval = %v, want %v", filled.Interval, defaultInterval)
	}

	if filled.MaxPerTick != defaultMaxPerTick {
		t.Errorf("max per tick = %d, want %d", filled.MaxPerTick, defaultMaxPerTick)
	}

	if filled.Now == nil {
		t.Error("no clock was filled in")
	}

	custom := Config{Interval: time.Minute, MaxPerTick: 5, Now: func() time.Time { return time.Time{} }}.withDefaults()

	if custom.Interval != time.Minute || custom.MaxPerTick != 5 {
		t.Errorf("a configured value was overwritten: %+v", custom)
	}

	if !custom.Now().IsZero() {
		t.Error("a configured clock was overwritten")
	}
}

package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/schedule"
	"github.com/robbiebyrd/indri/internal/services/lua"
)

// The whole chain, end to end: a script sets a timer, the ledger writes it after
// the mutation commits, the scheduler claims it and dispatches it back into the
// same engine with no session at all.
//
// Everything below the test is production code — a real Lua engine, the real
// effect ledger, the real game store with its lock and version fence, the real
// schedule store. Only the clock and the router are stood in for, and the router
// stand-in does exactly what internal/handlers/actions/script does.

// scriptFixture is an engine, the two stores behind it and the scheduler that
// fires what it schedules.
type scriptFixture struct {
	engine    *lua.Engine
	games     *game.MemoryStore
	entries   *schedule.MemoryStore
	scheduler *Scheduler
	gameID    string
}

func newScriptFixture(t *testing.T, src string, actionNames ...string) *scriptFixture {
	t.Helper()

	path := filepath.Join(t.TempDir(), "game.lua")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("writing the script: %v", err)
	}

	games := game.NewMemoryStore(context.Background(), nil, nil)

	// A data map rather than none: a nil map is not a document the store's
	// dotted-path writes can descend into, so a fixture without it would
	// exercise "create the map" rather than the ordinary edit under test.
	g, err := games.New("ABCD", &models.Script{PublicData: map[string]interface{}{"round": 0}}, false)
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}

	engine, err := lua.NewEngine([]string{path}, games)
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	t.Cleanup(engine.Close)

	entries := schedule.NewMemoryStore(schedule.Config{InstanceID: "instance-a"})

	timers, err := NewTimers(entries)
	if err != nil {
		t.Fatalf("building the timer capability: %v", err)
	}

	engine.Timers = timers

	fixture := &scriptFixture{
		engine:  engine,
		games:   games,
		entries: entries,
		gameID:  g.ID.Hex(),
	}

	// The stand-in for router.Dispatch: it runs the script action the way
	// handlers/actions/script does, threading the context — and therefore the
	// game the scheduler stamped on it — straight through.
	dispatch := func(
		ctx context.Context,
		session *models.Session,
		action string,
		payload map[string]interface{},
	) (actions.Result, error) {
		return engine.Invoke(ctx, action, actions.Request{
			Context: ctx,
			Action:  action,
			Session: session,
			Payload: payload,
		})
	}

	engine.Dispatch = dispatch

	fixture.scheduler, err = New(entries, dispatch, actionNames, Config{})
	if err != nil {
		t.Fatalf("building the scheduler: %v", err)
	}

	return fixture
}

// play dispatches one action as a player who is in the fixture's game.
func (f *scriptFixture) play(t *testing.T, action string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	res, err := f.engine.Invoke(ctx, action, actions.Request{
		Context: ctx,
		Action:  action,
		Session: &models.Session{UserID: strPtr("player-1"), GameID: &f.gameID},
	})
	if err != nil {
		t.Fatalf("dispatching %q: %v", action, err)
	}

	// A script failure is packed into Responses rather than returned, so a test
	// that only checked err would report a broken script as a pass.
	if len(res.Responses) != 0 {
		t.Fatalf("dispatching %q produced %d frames, the first being %s", action, len(res.Responses), res.Responses[0])
	}
}

func (f *scriptFixture) data(t *testing.T) map[string]interface{} {
	t.Helper()

	stored, err := f.games.Get(f.gameID)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	return stored.PublicData
}

func strPtr(s string) *string { return &s }

// A fired timer reads the state as it is at fire time, through indri.mutate,
// and never a snapshot frozen when the timer was set.
//
// The two are told apart by moving the state in between. If the scheduler
// carried a snapshot, the fired handler would close the round at 1 — the value
// as it stood when the timer was set — and every move made while the clock ran
// would be lost.
func TestScheduledAction_ReadsStateAtFireTimeNotAtScheduleTime(t *testing.T) {
	f := newScriptFixture(t, `
indri.on("open", function(req)
  indri.mutate(function(g)
    g.data.round = 1
    return g
  end)

  indri.after(30, "close_round", {})
end)

indri.on("bump", function(req)
  indri.mutate(function(g)
    g.data.round = (g.data.round or 0) + 1
    return g
  end)
end)

indri.on("close_round", function(req)
  indri.mutate(function(g)
    g.data.closedAt = g.data.round
    g.data.closedGame = req.gameId
    g.data.hadSession = req.session ~= nil
    return g
  end)
end)
`, "close_round")

	f.play(t, "open")

	if got := f.data(t)["round"]; got != float64(1) {
		t.Fatalf("data.round = %v after open, want 1", got)
	}

	// Two more moves land while the timer is still counting down.
	f.play(t, "bump")
	f.play(t, "bump")

	if got := f.data(t)["round"]; got != float64(3) {
		t.Fatalf("data.round = %v before the timer fired, want 3", got)
	}

	if fired := f.scheduler.tick(context.Background(), time.Now().Add(30*time.Second)); fired != 1 {
		t.Fatalf("%d timers fired, want 1", fired)
	}

	data := f.data(t)

	if got := data["closedAt"]; got != float64(3) {
		t.Fatalf("the timer closed the round at %v, want 3 — it read a snapshot frozen at schedule time", got)
	}

	if got := data["closedGame"]; got != f.gameID {
		t.Errorf("req.gameId = %v, want %v — the fired handler could not find its game", got, f.gameID)
	}

	if got := data["hadSession"]; got != false {
		t.Errorf("req.session was %v on a timer fire, want absent", got)
	}
}

// A timer set inside a mutation is written only if that mutation commits, and a
// script that calls it off before the fire time stops it — both through the real
// stores rather than a stand-in for either.
func TestScheduledAction_CancelledBeforeItFires(t *testing.T) {
	f := newScriptFixture(t, `
indri.on("open", function(req)
  indri.mutate(function(g)
    g.data.timer = indri.after(30, "close_round", {})
    return g
  end)
end)

indri.on("moved", function(req)
  indri.mutate(function(g)
    indri.cancel(g.data.timer)
    g.data.timer = nil
    return g
  end)
end)

indri.on("close_round", function(req)
  indri.mutate(function(g)
    g.data.closed = true
    return g
  end)
end)
`, "close_round")

	f.play(t, "open")

	id, _ := f.data(t)["timer"].(string)
	if id == "" {
		t.Fatal("the script stored no timer id, so this test proved nothing")
	}

	if _, err := f.entries.Get(id); err != nil {
		t.Fatalf("the timer the script set was never written: %v", err)
	}

	f.play(t, "moved")

	if fired := f.scheduler.tick(context.Background(), time.Now().Add(time.Hour)); fired != 0 {
		t.Fatalf("%d timers fired after being cancelled, want 0", fired)
	}

	if got := f.data(t)["closed"]; got != nil {
		t.Fatalf("data.closed = %v, want absent — the cancelled timer fired anyway", got)
	}

	// The control: without the cancel, the same timer does fire. Without this,
	// the assertion above would pass against a scheduler that fires nothing.
	f.play(t, "open")

	if fired := f.scheduler.tick(context.Background(), time.Now().Add(time.Hour)); fired != 1 {
		t.Fatalf("%d timers fired for an uncancelled timer, want 1", fired)
	}

	if got := f.data(t)["closed"]; got != true {
		t.Fatalf("data.closed = %v, want true", got)
	}
}

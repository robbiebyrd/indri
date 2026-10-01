package scripttest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scheduleRepo "github.com/robbiebyrd/indri/internal/repo/schedule"
	"github.com/robbiebyrd/indri/internal/services/events"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
	schedulerService "github.com/robbiebyrd/indri/internal/services/scheduler"
)

// DefaultTimeout bounds one case's invocation.
//
// It is deliberately not the server's own budget (100ms — see
// internal/handlers/actions/script). This harness answers "is the script
// correct", and a CI runner under load that reported a correct script as broken
// because a pooled Lua state took 120ms to build would be worse than no harness
// at all. What it does bound is a script that never returns, which would
// otherwise hang the whole run.
const DefaultTimeout = 5 * time.Second

// Result is one case's verdict. Failures is empty when the case passed, and
// every entry is one line a reader can act on.
type Result struct {
	// Name is "<fixture file>/<case>", the name the report prints.
	Name string

	Failures []string
}

// Passed reports whether the case met every one of its expectations.
func (r Result) Passed() bool {
	return len(r.Failures) == 0
}

// Runner is one `indri-script test` run: the compiled engine, the store every
// case stamps its game into, and the deltas that store published.
//
// One engine and one store for the whole run, not one per case. Building an
// engine compiles every script and prepares a pooled Lua state, and a run of a
// hundred cases that did that a hundred times would be slow enough that authors
// stopped running it. Cases stay isolated because each one plays its own game:
// a fresh document, a fresh version, and deltas the recorder keys by game id.
type Runner struct {
	suite  *Suite
	engine *luaService.Engine

	games     *gameRepo.MemoryStore
	published *recorder

	timeout time.Duration

	// codes numbers the game each case is stamped into. The store rejects a
	// duplicate code exactly as the MongoDB unique index does, so they have to
	// differ.
	codes int
}

// NewRunner compiles the suite's scripts and prepares the store its cases play
// in. The caller owns the runner and must Close it.
//
// A compile failure, a grant naming a capability this build does not have, or a
// script whose registrations differ between states all surface here — the same
// three failures `indri-script check` reports, because this is the same
// construction the server performs at boot.
func NewRunner(suite *Suite, timeout time.Duration) (*Runner, error) {
	if suite == nil {
		return nil, fmt.Errorf("a runner needs a loaded suite")
	}

	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	published := &recorder{byGame: map[string][]events.ChangeEvent{}}

	// context.Background rather than a per-run context: this is the store's own
	// process-lifetime context, which bounds the fan-out of a change event
	// rather than any one case. Each case's deadline rides on the invocation.
	games := gameRepo.NewMemoryStore(context.Background(), nil, published)

	// A config declaring no scripts cannot fail a fixture usefully: every case
	// is answered "no lua handler is registered", which reads as "your script is
	// wrong" when the truth is that no script was loaded. That is the harness
	// failing to run, not the game failing its tests, and the two want different
	// exit codes — so it is refused here rather than reported N times.
	//
	// The likeliest way to arrive here is a config resolved by fallback: naming
	// a directory that holds fixtures but no config.json resolves to the one in
	// the working directory, which on this repo is the framework's own and
	// declares nothing.
	if len(suite.Script.Scripts) == 0 {
		return nil, fmt.Errorf(
			"the config %q declares no lua scripts, so no fixture can pass; name the game's config with -script",
			suite.ConfigPath,
		)
	}

	engine, err := luaService.NewEngineWithGrants(suite.Script.Scripts, games)
	if err != nil {
		return nil, fmt.Errorf("loading the game scripts declared by %q: %w", suite.ConfigPath, err)
	}

	r := &Runner{suite: suite, engine: engine, games: games, published: published, timeout: timeout}

	// Both halves of what a script can reach outside its own game document, so a
	// script using either is testable rather than refused by a harness the
	// server would not have refused.
	//
	// The engine dispatches its own sent events, which is what the router does
	// on a live server for a script-declared action. Timers are written to an
	// in-memory schedule store and never fire: no scheduler is polling it, and a
	// fixture format that asserts on one dispatched action has nothing to say
	// about an action dispatched an hour later.
	engine.Dispatch = r.dispatch

	timers, err := schedulerService.NewTimers(scheduleRepo.NewMemoryStore(scheduleRepo.Config{}))
	if err != nil {
		engine.Close()

		return nil, fmt.Errorf("preparing the timer capability: %w", err)
	}

	engine.Timers = timers

	return r, nil
}

// Close releases the Lua states the runner is holding.
func (r *Runner) Close() {
	r.engine.Close()
}

// Actions is what the loaded scripts declare, which is every action a fixture
// may dispatch.
func (r *Runner) Actions() []string {
	return r.engine.Actions()
}

// dispatch is where an indri.send lands.
//
// It reaches the engine directly rather than a router, so a script can send
// another of its own actions and the harness runs it for real. A built-in
// framework action is refused instead of silently doing nothing: running one
// would need the session, user and game services a server boots, and a harness
// that pretended to have dispatched it would report a passing case for a script
// whose effect never happened.
func (r *Runner) dispatch(
	ctx context.Context,
	session *models.Session,
	action string,
	payload map[string]interface{},
) (actions.Result, error) {
	if !slices.Contains(r.engine.Actions(), action) {
		return actions.Result{}, fmt.Errorf(
			"indri.send(%q): the harness can only dispatch actions a game script declares, and %q is not one of them",
			action, action)
	}

	return r.engine.Invoke(ctx, action, actions.Request{
		Context: ctx,
		Action:  action,
		Session: session,
		Payload: payload,
	})
}

// Run plays every case in the suite, in file and then declaration order, and
// returns one Result per case.
//
// Sequentially and deliberately. The cases share one pool of Lua states, and a
// report whose lines arrived in a different order on every run is one an author
// cannot diff against the last.
func (r *Runner) Run(ctx context.Context) []Result {
	results := make([]Result, 0, r.suite.Cases())

	for _, fixture := range r.suite.Fixtures {
		for _, c := range fixture.Cases {
			results = append(results, Result{
				Name:     fixture.Name + "/" + c.Name,
				Failures: r.runCase(ctx, r.suite.state(fixture, c), c),
			})
		}
	}

	return results
}

// runCase plays one case and returns every way it fell short.
//
// Every expectation is checked rather than stopping at the first, so one run
// tells an author everything that is wrong with a case instead of one thing at
// a time.
func (r *Runner) runCase(ctx context.Context, state *models.Script, c Case) []string {
	r.codes++

	g, err := r.games.New(fmt.Sprintf("FIXTURE-%d", r.codes), state, false)
	if err != nil {
		return []string{fmt.Sprintf("the harness could not create the fixture game: %v", err)}
	}

	id := g.ID

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	result, err := r.engine.Invoke(ctx, c.Event.Action, actions.Request{
		Context: ctx,
		Action:  c.Event.Action,
		Session: c.Event.Session.model(id),
		Payload: c.Event.Payload,
	})
	if err != nil {
		// A host failure, not the script's: no handler for the action, or a
		// payload with no Lua form. Naming the declared actions turns the
		// commonest of them — a fixture dispatching an action the script does
		// not register — into a one-line fix.
		return []string{fmt.Sprintf("the harness could not run the action %q: %v (this game declares %v)",
			c.Event.Action, err, r.engine.Actions())}
	}

	refusal, replies := splitResponses(result.Responses)
	published := r.published.forGame(id)

	if c.Expect.refusal() {
		return r.checkRefused(c, refusal, published, id, g.Version)
	}

	return checkAccepted(c, refusal, replies, published)
}

// checkRefused holds a case that expected the script to refuse.
//
// All three halves matter, and they are the three the tic-tac-toe suite this
// harness replaces asserts. The message separates "refused this move" from
// "never ran"; no delta proves nothing was broadcast; and the unchanged version
// proves the refusal unwound the store rather than committing and then
// complaining.
func (r *Runner) checkRefused(
	c Case,
	refusal string,
	published []events.ChangeEvent,
	gameID string,
	version int64,
) []string {
	failures := []string{}

	switch {
	case refusal == "":
		failures = append(failures,
			fmt.Sprintf("the action was accepted, want it refused with a message containing %q", c.Expect.Error))
	case !strings.Contains(refusal, c.Expect.Error):
		failures = append(failures,
			fmt.Sprintf("refused with %q, want a message containing %q", refusal, c.Expect.Error))
	}

	if len(published) != 0 {
		failures = append(failures,
			fmt.Sprintf("a refused action published %d deltas, want none: %s", len(published), render(published[0].UpdatedFields)))
	}

	stored, err := r.games.Get(gameID)
	if err != nil {
		return append(failures, fmt.Sprintf("the harness could not reload the fixture game: %v", err))
	}

	if stored.Version != version {
		failures = append(failures,
			fmt.Sprintf("a refused action moved the game from version %d to %d", version, stored.Version))
	}

	return failures
}

// checkAccepted holds a case that expected the script to write.
func checkAccepted(
	c Case,
	refusal string,
	replies []map[string]interface{},
	published []events.ChangeEvent,
) []string {
	if refusal != "" {
		// The frame carries the script's own message, and its file and line
		// whenever the script faulted rather than refused deliberately — which
		// is the whole of what an author needs to open the right line.
		return []string{fmt.Sprintf("the action was refused: %s", refusal)}
	}

	failures := checkDelta(*c.Expect.Published, c.Expect.Removed, published)

	return append(failures, checkReplies(c.Expect.Replies, replies)...)
}

// checkDelta compares what the action published against what the case says it
// must.
func checkDelta(want map[string]interface{}, wantRemoved []string, published []events.ChangeEvent) []string {
	if len(want) == 0 && len(wantRemoved) == 0 {
		if len(published) == 0 {
			return nil
		}

		return []string{fmt.Sprintf("the action published %d deltas, want none: %s",
			len(published), render(published[0].UpdatedFields))}
	}

	// One action is one write. Two deltas would mean a player saw the game move
	// in two steps, which is a different game from the one this case describes.
	if len(published) != 1 {
		return []string{fmt.Sprintf("the action published %d deltas, want exactly 1", len(published))}
	}

	event := published[0]
	failures := []string{}

	got := map[string]interface{}{}

	for path, value := range event.UpdatedFields {
		got[path] = value
	}

	// Asserted as present rather than compared: it is a clock reading no fixture
	// could predict, and a delta without it did not come from a save.
	if _, ok := got["updatedAt"]; !ok {
		failures = append(failures, "the delta carries no updatedAt, so no save produced it")
	}

	delete(got, "updatedAt")

	for _, path := range union(want, got) {
		wantValue, wanted := want[path]
		gotValue, gotIt := got[path]

		if wanted == gotIt && render(wantValue) == render(gotValue) {
			continue
		}

		failures = append(failures, fmt.Sprintf("%s: want %s, got %s",
			path, absentOr(wantValue, wanted), absentOr(gotValue, gotIt)))
	}

	if render(normaliseRemoved(wantRemoved)) != render(normaliseRemoved(event.RemovedFields)) {
		failures = append(failures, fmt.Sprintf("removed: want %s, got %s",
			render(normaliseRemoved(wantRemoved)), render(normaliseRemoved(event.RemovedFields))))
	}

	return failures
}

// checkReplies compares the documents indri.reply wrote back to the caller.
//
// An absent "replies" is the assertion that there were none, rather than a case
// that does not care: a script that answered the player something it should not
// have would otherwise pass.
func checkReplies(want, got []map[string]interface{}) []string {
	if len(want) != len(got) {
		return []string{fmt.Sprintf("the action sent %d replies, want %d; it sent %s", len(got), len(want), render(got))}
	}

	failures := []string{}

	for i := range want {
		if render(want[i]) != render(got[i]) {
			failures = append(failures, fmt.Sprintf("reply %d: want %s, got %s", i+1, render(want[i]), render(got[i])))
		}
	}

	return failures
}

// splitResponses separates the script's own failure from the documents it
// replied with.
//
// Invoke does not report a script's failure through its error return — the
// transports do not agree about a Go error — so it packs it into Responses as a
// models.WSError, alongside anything indri.reply wrote. The error code is what
// tells the two apart: every JSON object unmarshals into a WSError with a zero
// code.
func splitResponses(responses [][]byte) (refusal string, replies []map[string]interface{}) {
	for _, response := range responses {
		var frame models.WSError

		if json.Unmarshal(response, &frame) == nil && frame.ErrorCode == models.ErrScriptFailed.ErrorCode {
			refusal = frame.Message

			continue
		}

		var reply map[string]interface{}

		if err := json.Unmarshal(response, &reply); err != nil {
			// A reply that is not a JSON object cannot be compared against a
			// fixture's, so it is reported as one that will never match rather
			// than dropped.
			reply = map[string]interface{}{"<not a json object>": string(response)}
		}

		replies = append(replies, reply)
	}

	return refusal, replies
}

// recorder records every delta the store fans out, keyed by the game it belongs
// to, so each case is held to what its own game published.
type recorder struct {
	mu     sync.Mutex
	byGame map[string][]events.ChangeEvent
}

func (p *recorder) Publish(_ context.Context, event events.ChangeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.byGame[event.ID] = append(p.byGame[event.ID], event)

	return nil
}

// Subscribe is the half of the Publisher contract nothing here uses: the
// harness has no broadcast loop and no connections to fan out to. It returns an
// error rather than a nil channel so that a caller which one day did subscribe
// would be told, instead of blocking forever on a channel nobody writes to.
func (p *recorder) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return nil, fmt.Errorf("the script test harness does not broadcast change events")
}

func (p *recorder) forGame(id string) []events.ChangeEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.byGame[id])
}

// union is every path either side carries, sorted, so a mismatch reads as a
// diff rather than as two randomly ordered maps.
func union(a, b map[string]interface{}) []string {
	seen := map[string]struct{}{}

	for path := range a {
		seen[path] = struct{}{}
	}

	for path := range b {
		seen[path] = struct{}{}
	}

	return slices.Sorted(maps.Keys(seen))
}

// normaliseRemoved renders a nil and an empty removed list the same, so a
// fixture that omits "removed" matches a delta that removed nothing.
func normaliseRemoved(paths []string) []string {
	if paths == nil {
		return []string{}
	}

	return paths
}

// absentOr renders a value, or says it was not there at all.
func absentOr(value interface{}, present bool) string {
	if !present {
		return "(absent)"
	}

	return render(value)
}

// render is the canonical JSON form of a value, which is both how two values
// are compared and how a mismatch is shown. encoding/json sorts map keys, so
// equal documents render identically however they were built — a fixture's
// value comes from a JSON file and a delta's from events.ToMap, and comparing
// the two with reflect.DeepEqual would turn every whole number into a
// float64-versus-int failure.
func render(value interface{}) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<unrenderable %T: %v>", value, err)
	}

	return string(raw)
}

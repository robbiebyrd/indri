// Package scheduler fires the deferred actions a game script asked for.
//
// It is the other half of internal/repo/schedule: the store owns what an entry
// is and who may claim it, and this owns when to look and what to do with what
// comes back. One goroutine in boot.Serve's errgroup polls, claims whatever is
// due, dispatches it through the shared router and reports the outcome back to
// the store.
//
// # A fired action has no session, and that is the whole contract
//
// Nobody is connected when a timer fires. There is no authenticated caller to
// resolve, and inventing one would hand a scheduled action authority no player
// ever granted it, so the dispatch carries a nil session — deliberately, and
// permanently. Two things follow, and both are enforced here rather than
// documented and hoped for:
//
//   - The game cannot come from the session, so it rides on the context
//     (actions.WithGameID) and reaches a handler as Request.GameID and a script
//     as req.gameId. It was stamped on the entry from the *scheduling* caller's
//     authenticated session, never from a payload.
//   - Every built-in framework action rejects a nil session, so a scheduled
//     action name must be one a game script registered. The name is checked
//     against the engine's manifest at fire time as well as at schedule time,
//     because an entry outlives the script that wrote it and an action removed
//     in an upgrade must be dead-lettered rather than dispatched into silence.
//
// # Delivery is at-least-once
//
// An instance can crash after dispatching and before marking the entry done,
// and the entry is re-claimed once its lease expires. Nothing here is
// exactly-once and nothing built on it may be described as such.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/schedule"
)

// Defaults applied to a zero Config.
const (
	// defaultInterval is how often the store is asked what is due. It bounds how
	// late a timer can be, so it is the one number a game designer feels: a
	// second is invisible beside a countdown a player is watching, and rarer
	// polling would show.
	defaultInterval = time.Second

	// defaultMaxPerTick bounds the work one tick does.
	//
	// Without it a backlog — an instance restarting after an outage, a hundred
	// games whose rounds all expired while it was down — would be drained in one
	// unbroken loop, and the loop only looks at ctx.Done between ticks. The
	// backlog is still drained, just across ticks, and shutdown stays prompt.
	defaultMaxPerTick = 64
)

// Dispatcher runs one action through the shared handler registry, the way a
// transport does.
//
// Declared here rather than imported so this package does not depend on the
// handler layer it feeds; boot supplies router.Dispatch. It is the same shape
// as lua.Dispatcher and rest.Transport.Dispatch, for the same reason.
type Dispatcher func(
	ctx context.Context,
	session *models.Session,
	action string,
	payload map[string]interface{},
) (actions.Result, error)

// Config tunes the loop. The zero value is valid.
type Config struct {
	// Interval is how often the store is polled.
	Interval time.Duration

	// MaxPerTick is how many entries one tick may dispatch.
	MaxPerTick int

	// Now is the clock. It is a field so a test can drive the loop through a
	// year of backoff without waiting for one, and it is the only clock this
	// package reads — a test that advances it advances everything.
	Now func() time.Time
}

func (c Config) withDefaults() Config {
	if c.Interval <= 0 {
		c.Interval = defaultInterval
	}

	if c.MaxPerTick <= 0 {
		c.MaxPerTick = defaultMaxPerTick
	}

	if c.Now == nil {
		c.Now = time.Now
	}

	return c
}

// Scheduler polls one schedule store and dispatches what is due.
type Scheduler struct {
	entries  schedule.Storer
	dispatch Dispatcher

	// dispatchable is the set of action names a timer may fire: the script
	// manifest, frozen at boot. A name outside it is dead-lettered rather than
	// dispatched — see the package comment.
	dispatchable []string

	cfg Config
}

// New returns a scheduler over entries, dispatching through dispatch.
//
// actionNames is Engine.Actions(): every action a game script registered. An
// empty list is allowed and means no entry can ever fire, which is the honest
// behaviour for a server running no scripts — it is the scripts that schedule.
func New(entries schedule.Storer, dispatch Dispatcher, actionNames []string, cfg Config) (*Scheduler, error) {
	if entries == nil {
		return nil, fmt.Errorf("the scheduler requires a schedule store")
	}

	if dispatch == nil {
		return nil, fmt.Errorf("the scheduler requires a dispatcher")
	}

	return &Scheduler{
		entries:      entries,
		dispatch:     dispatch,
		dispatchable: slices.Clone(actionNames),
		cfg:          cfg.withDefaults(),
	}, nil
}

// Run polls until ctx is cancelled, then returns nil.
//
// Nil rather than ctx.Err(): a cancelled root context is how this server shuts
// down, and reporting it as a failure would make every clean stop look like
// one. It is the same contract monitorGameChanges has, and errgroup treats the
// first non-nil error as the reason the whole group unwound.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.tick(ctx, s.cfg.Now())
		}
	}
}

// tick claims and fires everything due at now, up to the per-tick budget, and
// reports how many it dispatched.
//
// This is the unit Run repeats, and it is separate from Run so the loop can be
// driven one step at a time against a clock a test controls. A test that waited
// for a real ticker would be measuring the machine it runs on.
func (s *Scheduler) tick(ctx context.Context, now time.Time) int {
	fired := 0

	for ; fired < s.cfg.MaxPerTick; fired++ {
		// Checked inside the loop as well as in Run, so a shutdown reaches a
		// tick that is working through a backlog rather than waiting for it to
		// finish.
		if ctx.Err() != nil {
			return fired
		}

		entry, err := s.entries.ClaimDue(now)
		if err != nil {
			// Logged, not returned. A store that cannot answer this second is a
			// reason to try again next second, not to bring down the server the
			// games are still being played on.
			log.Printf("claiming a due scheduled action: %v", err)

			return fired
		}

		if entry == nil {
			return fired
		}

		s.fire(ctx, entry)
	}

	return fired
}

// fire dispatches one claimed entry and records what happened to it.
//
// Every path ends in a store call — Complete, Fail or MarkDead — because an
// entry left leased is invisible until its lease expires and is then simply
// tried again. Saying nothing is the one outcome that hides a bug.
func (s *Scheduler) fire(ctx context.Context, entry *schedule.Entry) {
	// schedule.Entry keeps a Mongo ObjectID of its own; only the game, user and
	// session models moved to string ids.
	id := entry.ID.Hex()

	if !slices.Contains(s.dispatchable, entry.Action) {
		// Terminal, and deliberately not a retry: the action name will not come
		// back on its own, so retrying it would only burn the attempt budget
		// before dead-lettering it anyway, several minutes later and with a
		// backoff in between that tells an operator nothing.
		reason := fmt.Sprintf(
			"the action %q is not a game script action on this server (a timer fires with no session, "+
				"so it can only reach a script action); known: %v", entry.Action, s.dispatchable,
		)

		log.Printf("dead-lettering scheduled entry %s for game %s: %s", id, entry.GameID, reason)

		if err := s.entries.MarkDead(id, reason); err != nil {
			log.Printf("dead-lettering scheduled entry %s: %v", id, err)
		}

		return
	}

	if err := s.dispatchEntry(ctx, entry); err != nil {
		log.Printf("dispatching the scheduled action %q for game %s: %v", entry.Action, entry.GameID, err)

		// Fail decides between a retry with a backoff and a dead letter, using
		// the attempt count the claim already incremented. It also pushes fireAt
		// past the backoff, which is what stops a permanently failing entry from
		// being re-claimed by the very next tick.
		failed, failErr := s.entries.Fail(id, err.Error())
		if failErr != nil {
			log.Printf("recording the failure of scheduled entry %s: %v", id, failErr)

			return
		}

		if failed.State == schedule.StateDead {
			log.Printf(
				"scheduled entry %s for game %s gave up after %d attempts: %v",
				id, entry.GameID, failed.Attempts, err,
			)
		}

		return
	}

	if err := s.entries.Complete(id); err != nil {
		// The action already ran. Losing the acknowledgement means it will be
		// dispatched again once the lease lapses, which is the at-least-once
		// guarantee working as described rather than a new failure.
		log.Printf("acknowledging scheduled entry %s after it fired: %v", id, err)
	}
}

// dispatchEntry runs one entry's action, with the game it belongs to on the
// context and no session at all.
//
// The recover is not defensive habit. router.Dispatch already turns a panicking
// handler into an error, but the received/processed phases run on a timer fire
// exactly as they do on a client message, and a hook written against a client
// message may well dereference req.Session — which is nil here. That is a hook
// bug, and it must cost the game one timer and a log line, not the scheduler
// goroutine and with it every timer in the process.
func (s *Scheduler) dispatchEntry(ctx context.Context, entry *schedule.Entry) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the handler for the scheduled action %q panicked: %v", entry.Action, r)
		}
	}()

	// Stamped from the entry, which took it from the authenticated session of
	// whoever scheduled the timer. This is the only way the fired action can
	// find its game, and Request.GameID prefers a real session wherever there is
	// one, so it can never override an authenticated caller.
	ctx = actions.WithGameID(ctx, entry.GameID)

	// Nil session, on purpose. See the package comment.
	_, err = s.dispatch(ctx, nil, entry.Action, entry.Payload)

	return err
}

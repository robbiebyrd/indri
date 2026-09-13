package scheduler

import (
	"errors"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/repo/schedule"
)

// Timers is the writing half of the scheduler: what a game script reaches when
// it calls indri.after, indri.at or indri.cancel.
//
// It exists as an adapter rather than the lua package holding a *schedule.Store
// directly, for the reason the game store is reached through an interface too —
// a script's capabilities are meant to be readable as a list of methods, not
// inferred from which repository happens to be in scope. It satisfies
// lua.GameScheduler; the injector's assignment is the compile-time check.
type Timers struct {
	entries schedule.Storer
}

// NewTimers returns the scheduling capability over entries.
func NewTimers(entries schedule.Storer) (*Timers, error) {
	if entries == nil {
		return nil, fmt.Errorf("the timer capability requires a schedule store")
	}

	return &Timers{entries: entries}, nil
}

// NewTimerID mints the identity a timer will be stored under.
//
// Minting is separate from storing because a script is handed its timer's id
// the moment it calls indri.after, while the entry itself is only written once
// the mutation that queued it has committed. Both halves have to agree on the
// id, and the script is holding it first.
func (t *Timers) NewTimerID() string {
	return schedule.NewEntryID()
}

// ScheduleTimer stores one deferred action under a previously minted id.
//
// No idempotency key is supplied, so the store generates one: each call is its
// own logical event. Deduplicating repeats is not this layer's job — the effect
// ledger already emits a timer once per *commit* of the mutation that queued it,
// however many times the version fence made that mutation re-run.
//
// ScriptVersion is left empty for now, and deliberately rather than by
// omission. This build has no per-deployment script version to put there — only
// models.ScriptSchemaVersion, which describes the config *format* and says
// nothing about which handlers a script registers — and a field filled with a
// value that does not mean what it claims is worse than an empty one. What the
// field was for is already covered: the scheduler checks every entry's action
// against the live manifest at fire time and dead-letters a name that no longer
// exists.
func (t *Timers) ScheduleTimer(
	id, gameID, action string,
	payload map[string]interface{},
	fireAt time.Time,
) error {
	if _, err := t.entries.Schedule(schedule.CreateEntry{
		ID:      id,
		GameID:  gameID,
		Action:  action,
		Payload: payload,
		FireAt:  fireAt,
	}); err != nil {
		return err
	}

	return nil
}

// CancelTimer calls off a timer belonging to gameID.
//
// The game is checked before the cancel, and that check is the point of the
// parameter. A MongoDB ObjectID is not a secret — it is a timestamp, a
// per-process value and a counter — so a script that passes a player-supplied
// field straight to indri.cancel would otherwise let that player call off
// another game's timers. The entry is read first so the wrong game is refused
// rather than obeyed.
//
// An id that names nothing is not an error. The entry may have fired a moment
// ago, and the script holding the id had no way to know.
func (t *Timers) CancelTimer(gameID, id string) error {
	entry, err := t.entries.Get(id)
	if err != nil {
		if errors.Is(err, schedule.ErrNotFound) {
			return nil
		}

		return err
	}

	if entry.GameID != gameID {
		return fmt.Errorf("the timer %q belongs to another game and cannot be cancelled from game %q", id, gameID)
	}

	// A cancel that lost the race with a claim reports ErrAlreadyClaimed. It is
	// returned rather than swallowed: the action is being dispatched right now,
	// and a script that thinks it stopped a turn timeout which is in fact about
	// to fire needs that in the log.
	return t.entries.Cancel(id)
}

package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/repo/schedule"
)

func newTestTimers(t *testing.T) (*Timers, *schedule.MemoryStore) {
	t.Helper()

	store := schedule.NewMemoryStore(schedule.Config{InstanceID: "instance-a"})

	timers, err := NewTimers(store)
	if err != nil {
		t.Fatalf("building the timer capability: %v", err)
	}

	return timers, store
}

func TestNewTimers_RefusesAMissingStore(t *testing.T) {
	if _, err := NewTimers(nil); err == nil {
		t.Fatal("a timer capability with no store was accepted")
	}
}

// Each minted id is its own. Two timers sharing one would make the second
// unstorable and the first uncancellable.
func TestNewTimerID_MintsDistinctIds(t *testing.T) {
	timers, _ := newTestTimers(t)

	seen := map[string]struct{}{}

	for range 100 {
		id := timers.NewTimerID()

		if _, dup := seen[id]; dup {
			t.Fatalf("the id %q was minted twice", id)
		}

		seen[id] = struct{}{}
	}
}

// The entry is stored under the id the caller was already holding. Anything else
// and a script that kept its timer's id in game state would be holding a handle
// that cancels nothing.
func TestScheduleTimer_StoresUnderTheMintedId(t *testing.T) {
	timers, store := newTestTimers(t)

	id := timers.NewTimerID()
	fireAt := time.Now().Add(30 * time.Second)

	if err := timers.ScheduleTimer(id, "game-1", "turn_timeout", map[string]interface{}{"player": "p1"}, fireAt); err != nil {
		t.Fatalf("scheduling: %v", err)
	}

	entry, err := store.Get(id)
	if err != nil {
		t.Fatalf("the entry is not stored under the minted id %q: %v", id, err)
	}

	if entry.GameID != "game-1" || entry.Action != "turn_timeout" {
		t.Errorf("stored entry = %+v, want game-1/turn_timeout", entry)
	}

	if got := entry.Payload["player"]; got != "p1" {
		t.Errorf("stored payload player = %v, want p1", got)
	}
}

// A timer belongs to the game that set it. An ObjectID is a timestamp, a
// per-process value and a counter — not a secret — so a script passing a
// player-supplied field to indri.cancel must not thereby reach another game's
// timers.
func TestCancelTimer_RefusesATimerBelongingToAnotherGame(t *testing.T) {
	timers, store := newTestTimers(t)

	id := timers.NewTimerID()
	if err := timers.ScheduleTimer(id, "game-1", "turn_timeout", nil, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("scheduling: %v", err)
	}

	if err := timers.CancelTimer("game-2", id); err == nil {
		t.Fatal("another game was allowed to cancel this game's timer")
	}

	// Refused *and* untouched: reporting an error while deleting it anyway would
	// be the same bug with a log line attached.
	entry, err := store.Get(id)
	if err != nil {
		t.Fatalf("the refused timer was removed anyway: %v", err)
	}

	if entry.State != schedule.StatePending {
		t.Errorf("entry state = %q, want %q", entry.State, schedule.StatePending)
	}

	// The control: its own game can still cancel it, so the refusal above is
	// about the game and not about the call being broken.
	if err := timers.CancelTimer("game-1", id); err != nil {
		t.Fatalf("the owning game could not cancel its own timer: %v", err)
	}

	if _, err := store.Get(id); err == nil {
		t.Fatal("the timer survived a cancel from its own game")
	}
}

// An id that names nothing is not an error. The entry may have fired a moment
// ago, and the script holding the id had no way to know.
func TestCancelTimer_AnUnknownIdIsNotAnError(t *testing.T) {
	timers, _ := newTestTimers(t)

	if err := timers.CancelTimer("game-1", schedule.NewEntryID()); err != nil {
		t.Fatalf("cancelling an id that names nothing: %v", err)
	}

	// Cancelling twice is the same case, and is how a defensive script behaves.
	id := timers.NewTimerID()
	if err := timers.ScheduleTimer(id, "game-1", "turn_timeout", nil, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("scheduling: %v", err)
	}

	for i := range 2 {
		if err := timers.CancelTimer("game-1", id); err != nil {
			t.Fatalf("cancel %d reported an error: %v", i+1, err)
		}
	}
}

// A cancel that lost the race with a claim is reported rather than swallowed:
// the action is being dispatched right now, and a script that believes it
// stopped a turn timeout which is about to fire needs that in the log.
func TestCancelTimer_ReportsLosingTheRaceWithAClaim(t *testing.T) {
	timers, store := newTestTimers(t)

	id := timers.NewTimerID()
	if err := timers.ScheduleTimer(id, "game-1", "turn_timeout", nil, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("scheduling: %v", err)
	}

	claimed, err := store.ClaimDue(time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claiming the entry: %v (%v)", err, claimed)
	}

	err = timers.CancelTimer("game-1", id)
	if !errors.Is(err, schedule.ErrAlreadyClaimed) {
		t.Fatalf("cancelling a claimed entry returned %v, want %v", err, schedule.ErrAlreadyClaimed)
	}
}

package schedule

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The lifecycle a deferred event goes through — claimed once, renewable while
// it is being handled, retried then dead-lettered, cancellable until it is
// claimed — is the store's contract, not MongoDB's. Every test below is
// therefore written once and run against both backends: the in-memory store,
// which always runs, and the MongoDB store, which runs wherever a database is
// reachable.
//
// Before that split, all of this needed MongoDB and skipped itself on a CI
// runner that has none, so a broken lease fence would have reported green.
// What the in-memory backend still cannot prove is that FindOneAndUpdate is
// atomic and that the BSON predicates select what the Go mirrors select; the
// MongoDB run and the mirror tests in schedule_test.go remain that layer.

// storeFactory makes stores over one shared, initially empty set of entries.
// The first call is the store under test; later calls are peers, standing in
// for other server instances polling the same collection.
type storeFactory func(t *testing.T, cfg Config) Storer

// forEachScheduleBackend runs test against every available backend.
func forEachScheduleBackend(t *testing.T, test func(t *testing.T, newStore storeFactory)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		var first *MemoryStore

		test(t, func(_ *testing.T, cfg Config) Storer {
			if first == nil {
				first = NewMemoryStore(cfg)

				return first
			}

			return first.Peer(cfg)
		})
	})

	t.Run("mongodb", func(t *testing.T) {
		var first *Store

		test(t, func(t *testing.T, cfg Config) Storer {
			t.Helper()

			if first == nil {
				first = newTestStore(t, cfg)
				clearSchedule(t, first)

				return first
			}

			return peerStore(t, first, cfg)
		})
	})
}

func mustScheduleOn(t *testing.T, store Storer, gameID string, fireAt time.Time) *Entry {
	t.Helper()

	entry, err := store.Schedule(CreateEntry{
		GameID:        gameID,
		Action:        "turn_timeout",
		Payload:       map[string]interface{}{"player": "p1"},
		ScriptVersion: "v1",
		FireAt:        fireAt,
	})
	if err != nil {
		t.Fatalf("scheduling entry: %v", err)
	}

	return entry
}

// TestSchedule_FreshlyInsertedEntryIsClaimed is the regression test for the
// bug that would have stopped every timer in the system from firing: a new
// entry has never been claimed, so a naive "lease has expired" test must still
// treat it as due.
func TestSchedule_FreshlyInsertedEntryIsClaimed(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		scheduled := mustScheduleOn(t, store, "game-fresh", time.Now().Add(-time.Minute))

		claimed, err := store.ClaimDue(time.Now())
		if err != nil {
			t.Fatalf("claiming due entry: %v", err)
		}

		if claimed == nil {
			t.Fatal("a freshly inserted, due entry was not claimed; no timer would ever fire")
		}

		if claimed.ID != scheduled.ID {
			t.Fatalf("claimed %v, want %v", claimed.ID.Hex(), scheduled.ID.Hex())
		}

		if claimed.State != StateLeased || claimed.ClaimedBy != "instance-a" || claimed.Attempts != 1 {
			t.Errorf("claimed entry is not leased to this instance: %+v", claimed)
		}

		if claimed.GameID != "game-fresh" || claimed.ScriptVersion != "v1" ||
			claimed.IdempotencyKey == "" || claimed.Payload["player"] != "p1" {
			t.Errorf("claimed entry lost its scheduling detail: %+v", claimed)
		}
	})
}

// TestClaimDue_NothingDueIsNotAnError: the scheduler polls constantly, so an
// idle collection must read as "nothing to do", not as a failure.
func TestClaimDue_NothingDueIsNotAnError(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		mustScheduleOn(t, store, "game-later", time.Now().Add(time.Hour))

		claimed, err := store.ClaimDue(time.Now())
		if err != nil {
			t.Fatalf("claiming with nothing due: %v", err)
		}

		if claimed != nil {
			t.Errorf("claimed an entry that is not due yet: %+v", claimed)
		}
	})
}

// TestClaimDue_ConcurrentClaimersGetDisjointEntries: each due entry goes to
// exactly one instance, or an action fires twice. Against MongoDB this proves
// FindOneAndUpdate really is atomic; against the in-memory store it proves the
// claim predicate and the write cannot interleave there either.
func TestClaimDue_ConcurrentClaimersGetDisjointEntries(t *testing.T) {
	const (
		entryCount   = 40
		claimerCount = 4
	)

	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		claimers := make([]Storer, claimerCount)
		for i := range claimers {
			claimers[i] = newStore(t, Config{InstanceID: fmt.Sprintf("instance-%d", i)})
		}

		fireAt := time.Now().Add(-time.Minute)
		for i := 0; i < entryCount; i++ {
			mustScheduleOn(t, claimers[0], "game-concurrent", fireAt)
		}

		now := time.Now()
		claimed := make([][]*Entry, claimerCount)
		errs := make([]error, claimerCount)

		var wg sync.WaitGroup

		for i, claimer := range claimers {
			wg.Add(1)

			go func(index int, claimer Storer) {
				defer wg.Done()

				for {
					entry, err := claimer.ClaimDue(now)
					if err != nil {
						errs[index] = err
						return
					}

					if entry == nil {
						return
					}

					claimed[index] = append(claimed[index], entry)
				}
			}(i, claimer)
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("claimer %d failed: %v", i, err)
			}
		}

		seen := map[bson.ObjectID]string{}

		for index, entries := range claimed {
			owner := claimers[index].InstanceID()

			for _, entry := range entries {
				if previous, duplicate := seen[entry.ID]; duplicate {
					t.Fatalf("entry %v claimed twice: by %s and by %s", entry.ID.Hex(), previous, owner)
				}

				seen[entry.ID] = owner

				if entry.ClaimedBy != owner {
					t.Errorf("entry %v returned to %s but leased to %s", entry.ID.Hex(), owner, entry.ClaimedBy)
				}
			}
		}

		if len(seen) != entryCount {
			t.Errorf("claimed %d entries in total, want %d", len(seen), entryCount)
		}
	})
}

// TestLease_ExpiredIsReclaimableAndRenewalHoldsIt covers both halves of the
// lease: an instance that crashed mid-dispatch must not hold an entry forever,
// and an instance that is still working must be able to keep it.
func TestLease_ExpiredIsReclaimableAndRenewalHoldsIt(t *testing.T) {
	const lease = time.Minute

	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		storeA := newStore(t, Config{InstanceID: "instance-a", LeaseTTL: lease})
		storeB := newStore(t, Config{InstanceID: "instance-b", LeaseTTL: lease})

		claimedAt := time.Now()
		mustScheduleOn(t, storeA, "game-lease", claimedAt.Add(-time.Minute))

		held, err := storeA.ClaimDue(claimedAt)
		if err != nil || held == nil {
			t.Fatalf("claiming entry: entry=%v err=%v", held, err)
		}

		// While the lease is live, nobody else gets the entry.
		stolen, err := storeB.ClaimDue(claimedAt.Add(lease / 2))
		if err != nil {
			t.Fatalf("peer claim: %v", err)
		}

		if stolen != nil {
			t.Fatalf("a live lease was stolen by %s", storeB.InstanceID())
		}

		// The slow handler renews, pushing the lease past its original expiry.
		// Times are stored to the millisecond, so let the clock move on before
		// renewing: otherwise the renewed lease can land in the same
		// millisecond as the original and there is no window left to assert
		// against.
		time.Sleep(10 * time.Millisecond)

		renewed, err := storeA.RenewLease(held.ID.Hex())
		if err != nil {
			t.Fatalf("renewing lease: %v", err)
		}

		if !renewed.ClaimedUntil.After(held.ClaimedUntil) {
			t.Fatalf("renewal did not extend the lease: %v is not after %v", renewed.ClaimedUntil, held.ClaimedUntil)
		}

		if renewed.Attempts != held.Attempts {
			t.Errorf("renewal counted as another attempt: %d, want %d", renewed.Attempts, held.Attempts)
		}

		justPastOriginalLease := held.ClaimedUntil.Add(time.Millisecond)

		if !renewed.ClaimedUntil.After(justPastOriginalLease) {
			t.Fatalf("renewal to %v left no window past the original expiry %v", renewed.ClaimedUntil, held.ClaimedUntil)
		}

		stolen, err = storeB.ClaimDue(justPastOriginalLease)
		if err != nil {
			t.Fatalf("peer claim after renewal: %v", err)
		}

		if stolen != nil {
			t.Fatal("a renewed lease was stolen at the original expiry; a slow handler cannot hold its entry")
		}

		// Once the renewed lease lapses, the entry is claimable again —
		// otherwise an instance that crashed mid-dispatch strands it forever.
		takenOver, err := storeB.ClaimDue(renewed.ClaimedUntil.Add(time.Millisecond))
		if err != nil {
			t.Fatalf("peer claim after expiry: %v", err)
		}

		if takenOver == nil {
			t.Fatal("an expired lease was not re-claimable")
		}

		if takenOver.ClaimedBy != storeB.InstanceID() || takenOver.Attempts != 2 {
			t.Errorf("re-claimed entry is wrong: %+v", takenOver)
		}

		// The instance that lost the entry must find out rather than keep
		// working, and must not be able to overwrite the outcome.
		if _, err := storeA.RenewLease(held.ID.Hex()); !errors.Is(err, ErrLeaseLost) {
			t.Errorf("renewing a lost lease returned %v, want %v", err, ErrLeaseLost)
		}

		if err := storeA.Complete(held.ID.Hex()); !errors.Is(err, ErrLeaseLost) {
			t.Errorf("completing a lost lease returned %v, want %v", err, ErrLeaseLost)
		}
	})
}

func TestComplete_MarksDoneAndStopsRefiring(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})

		claimedAt := time.Now()
		mustScheduleOn(t, store, "game-complete", claimedAt.Add(-time.Minute))

		claimed, err := store.ClaimDue(claimedAt)
		if err != nil || claimed == nil {
			t.Fatalf("claiming entry: entry=%v err=%v", claimed, err)
		}

		if err := store.Complete(claimed.ID.Hex()); err != nil {
			t.Fatalf("completing entry: %v", err)
		}

		done, err := store.Get(claimed.ID.Hex())
		if err != nil {
			t.Fatalf("re-reading entry: %v", err)
		}

		if done.State != StateDone || done.FiredAt == nil {
			t.Errorf("completed entry is %+v, want state %v with a fired time", done, StateDone)
		}

		// Long past the lease, a done entry must still never be claimed again.
		again, err := store.ClaimDue(claimedAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("claiming after completion: %v", err)
		}

		if again != nil {
			t.Errorf("a completed entry was claimed again: %+v", again)
		}
	})
}

// TestFail_RetriesThenDeadLetters walks an action that keeps failing all the
// way to the dead letter, and checks it is never silently marked done.
func TestFail_RetriesThenDeadLetters(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{
			InstanceID:  "instance-a",
			LeaseTTL:    time.Minute,
			MaxAttempts: 2,
			// Keep the backoff small so the retry is due immediately.
			RetryBackoff:    time.Millisecond,
			MaxRetryBackoff: time.Millisecond,
		})

		now := time.Now()
		scheduled := mustScheduleOn(t, store, "game-fail", now.Add(-time.Minute))

		claimed, err := store.ClaimDue(now)
		if err != nil || claimed == nil {
			t.Fatalf("claiming entry: entry=%v err=%v", claimed, err)
		}

		retried, err := store.Fail(scheduled.ID.Hex(), "handler exploded")
		if err != nil {
			t.Fatalf("failing entry: %v", err)
		}

		if retried.State != StatePending {
			t.Fatalf("first failure left the entry %v, want %v", retried.State, StatePending)
		}

		if retried.LastError != "handler exploded" {
			t.Errorf("failure reason not recorded: %q", retried.LastError)
		}

		// A retry must be claimable again, or the attempt budget is meaningless.
		reclaimed, err := store.ClaimDue(time.Now().Add(time.Second))
		if err != nil || reclaimed == nil {
			t.Fatalf("re-claiming a retried entry: entry=%v err=%v", reclaimed, err)
		}

		if reclaimed.Attempts != 2 {
			t.Fatalf("re-claimed entry is on attempt %d, want 2", reclaimed.Attempts)
		}

		dead, err := store.Fail(scheduled.ID.Hex(), "handler exploded again")
		if err != nil {
			t.Fatalf("failing entry for the last time: %v", err)
		}

		if dead.State != StateDead {
			t.Errorf("exhausted entry is %v, want %v — a permanent failure must not be marked done",
				dead.State, StateDead)
		}

		if again, err := store.ClaimDue(time.Now().Add(time.Hour)); err != nil || again != nil {
			t.Errorf("a dead entry was claimed again: entry=%v err=%v", again, err)
		}
	})
}

// TestMarkDead_RetiresAnEntryThatCanNeverRun covers the script-upgrade case:
// the persisted action no longer exists, so the entry is dead lettered rather
// than dispatched into a router that would quietly match nothing.
func TestMarkDead_RetiresAnEntryThatCanNeverRun(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})

		now := time.Now()
		mustScheduleOn(t, store, "game-upgrade", now.Add(-time.Minute))

		claimed, err := store.ClaimDue(now)
		if err != nil || claimed == nil {
			t.Fatalf("claiming entry: entry=%v err=%v", claimed, err)
		}

		if err := store.MarkDead(claimed.ID.Hex(), "unknown action after script upgrade"); err != nil {
			t.Fatalf("dead-lettering entry: %v", err)
		}

		dead, err := store.Get(claimed.ID.Hex())
		if err != nil {
			t.Fatalf("re-reading entry: %v", err)
		}

		if dead.State != StateDead || dead.LastError == "" {
			t.Errorf("entry is %+v, want %v with a recorded reason", dead, StateDead)
		}
	})
}

func TestCancel_RemovesPendingAndRefusesClaimed(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})

		now := time.Now()
		pending := mustScheduleOn(t, store, "game-cancel", now.Add(time.Hour))

		if err := store.Cancel(pending.ID.Hex()); err != nil {
			t.Fatalf("cancelling a pending entry: %v", err)
		}

		if _, err := store.Get(pending.ID.Hex()); err == nil {
			t.Error("a cancelled entry is still stored")
		}

		// Cancelling twice is not an error; the timer is gone either way.
		if err := store.Cancel(pending.ID.Hex()); err != nil {
			t.Errorf("cancelling an already-cancelled entry: %v", err)
		}

		// A cancel that loses the race with a claim must say so, not pretend
		// the action was called off while it is already being dispatched.
		claimedEntry := mustScheduleOn(t, store, "game-cancel", now.Add(-time.Minute))

		if _, err := store.ClaimDue(now); err != nil {
			t.Fatalf("claiming entry: %v", err)
		}

		if err := store.Cancel(claimedEntry.ID.Hex()); !errors.Is(err, ErrAlreadyClaimed) {
			t.Errorf("cancelling a claimed entry returned %v, want %v", err, ErrAlreadyClaimed)
		}
	})
}

// TestCancelForGame_CancelsEveryPendingTimerForOneGame is the only cleanup
// path for a game nobody returns to: nothing in this repo deletes a game, so
// without this the timers would sit until their TTL.
func TestCancelForGame_CancelsEveryPendingTimerForOneGame(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})

		now := time.Now()

		for i := 0; i < 3; i++ {
			mustScheduleOn(t, store, "game-abandoned", now.Add(time.Duration(i+1)*time.Hour))
		}

		survivor := mustScheduleOn(t, store, "game-other", now.Add(time.Hour))

		// One entry for the abandoned game is mid-dispatch and must be left alone.
		inFlight := mustScheduleOn(t, store, "game-abandoned", now.Add(-time.Minute))

		if _, err := store.ClaimDue(now); err != nil {
			t.Fatalf("claiming entry: %v", err)
		}

		cancelled, err := store.CancelForGame("game-abandoned")
		if err != nil {
			t.Fatalf("cancelling timers for game: %v", err)
		}

		if cancelled != 3 {
			t.Errorf("cancelled %d timers, want 3", cancelled)
		}

		if _, err := store.Get(survivor.ID.Hex()); err != nil {
			t.Errorf("another game's timer was cancelled too: %v", err)
		}

		if _, err := store.Get(inFlight.ID.Hex()); err != nil {
			t.Errorf("a timer being dispatched was deleted under its handler: %v", err)
		}

		if _, err := store.CancelForGame(""); err == nil {
			t.Error("cancelling timers for an empty game id was accepted")
		}
	})
}

// TestSchedule_IdempotencyKeyDeduplicates: delivery is at-least-once, so the
// key has to be stable and unique. Scheduling the same logical event twice
// must not queue the action twice.
func TestSchedule_IdempotencyKeyDeduplicates(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		create := CreateEntry{
			GameID:         "game-idempotent",
			Action:         "round_start",
			ScriptVersion:  "v1",
			IdempotencyKey: "round-3-start",
			FireAt:         time.Now().Add(time.Hour),
		}

		first, err := store.Schedule(create)
		if err != nil {
			t.Fatalf("scheduling entry: %v", err)
		}

		second, err := store.Schedule(create)
		if err != nil {
			t.Fatalf("re-scheduling the same event: %v", err)
		}

		if first.ID != second.ID {
			t.Errorf("the same idempotency key produced two entries: %v and %v", first.ID.Hex(), second.ID.Hex())
		}
	})
}

// TestEntriesSurviveStoreRestart: timers outlive the process that scheduled
// them, so an instance that starts later still sees what an earlier one left.
func TestEntriesSurviveStoreRestart(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		scheduled := mustScheduleOn(t, store, "game-restart", time.Now().Add(-time.Minute))

		restarted := newStore(t, Config{InstanceID: "instance-after-restart"})

		claimed, err := restarted.ClaimDue(time.Now())
		if err != nil {
			t.Fatalf("claiming after restart: %v", err)
		}

		if claimed == nil || claimed.ID != scheduled.ID {
			t.Fatalf("a scheduled entry did not survive the restart: got %v", claimed)
		}
	})
}

// TestSchedule_RejectsAnEntryThatCouldNeverFire.
func TestSchedule_RejectsAnEntryThatCouldNeverFire(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		incomplete := map[string]CreateEntry{
			"missing game id":   {Action: "turn_timeout", FireAt: time.Now()},
			"missing action":    {GameID: "game-1", FireAt: time.Now()},
			"missing fire time": {GameID: "game-1", Action: "turn_timeout"},
			"unparseable id":    {ID: "not-an-id", GameID: "game-1", Action: "turn_timeout", FireAt: time.Now()},
		}

		for name, create := range incomplete {
			if _, err := store.Schedule(create); err == nil {
				t.Errorf("%s was accepted", name)
			}
		}
	})
}

// A caller that has to hand the id out before the entry exists — indri.after
// returns its timer's id synchronously, while the write waits for the effect
// ledger to flush — must get the entry stored under exactly that id. Storing it
// anywhere else leaves the caller holding a handle that cancels nothing.
func TestSchedule_StoresUnderACallerSuppliedId(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		id := NewEntryID()

		stored, err := store.Schedule(CreateEntry{
			ID:     id,
			GameID: "game-supplied-id",
			Action: "turn_timeout",
			FireAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("scheduling under a supplied id: %v", err)
		}

		if stored.ID.Hex() != id {
			t.Fatalf("the entry was stored under %q, not the supplied %q", stored.ID.Hex(), id)
		}

		found, err := store.Get(id)
		if err != nil {
			t.Fatalf("fetching the entry by its supplied id: %v", err)
		}

		if found.GameID != "game-supplied-id" {
			t.Errorf("fetched entry = %+v", found)
		}

		// The same id twice is refused rather than silently stored beside the
		// first, which would leave one entry no cancel could ever reach.
		if _, err := store.Schedule(CreateEntry{
			ID:     id,
			GameID: "game-supplied-id",
			Action: "turn_timeout",
			FireAt: time.Now().Add(time.Hour),
		}); err == nil {
			t.Error("the same entry id was accepted twice")
		}
	})
}

// "There is nothing here" and "the store could not answer" are different
// answers, and cancelling a timer that already fired depends on telling them
// apart: the first is the ordinary case and must not be reported as a failure.
func TestGet_ReportsAMissingEntryAsNotFound(t *testing.T) {
	forEachScheduleBackend(t, func(t *testing.T, newStore storeFactory) {
		store := newStore(t, Config{InstanceID: "instance-a"})

		_, err := store.Get(NewEntryID())
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("fetching an id that names nothing returned %v, want %v", err, ErrNotFound)
		}

		// The control: a stored entry is found, so the error above is about the
		// entry being absent and not about Get being broken.
		stored := mustScheduleOn(t, store, "game-found", time.Now().Add(time.Hour))

		if _, err := store.Get(stored.ID.Hex()); err != nil {
			t.Fatalf("fetching a stored entry: %v", err)
		}
	})
}

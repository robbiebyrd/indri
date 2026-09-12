package schedule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
)

// The tests in this file come in two halves.
//
// The first half exercises the claim/lease state machine directly. Those
// functions are pure, so those tests need no database and always run — which
// matters, because CI has no MongoDB service container and every test in the
// second half skips there.
//
// The second half is the genuinely integration-level behaviour: that the BSON
// predicates say to MongoDB what the pure functions say in Go, and that the
// FindOneAndUpdate really is atomic. Those tests need a reachable MongoDB and
// skip without one.

// --- Pure state machine: no database required ---

func dueEntry(fireAt time.Time) *Entry {
	return &Entry{
		ID:             bson.NewObjectID(),
		GameID:         "game-1",
		Action:         "turn_timeout",
		ScriptVersion:  "v1",
		IdempotencyKey: randomID(),
		State:          StatePending,
		FireAt:         fireAt,
	}
}

// TestClaimable_FreshlyInsertedEntryIsDue is the regression test for the bug
// that would have stopped every timer in the system from ever firing: a new
// entry has never been claimed, so its claimedUntil is the zero time, and a
// naive "claimedUntil < now" test must still treat it as claimable.
func TestClaimable_FreshlyInsertedEntryIsDue(t *testing.T) {
	now := time.Now()
	entry := dueEntry(now.Add(-time.Minute))

	if !entry.ClaimedUntil.IsZero() {
		t.Fatalf("a fresh entry should have a zero claimedUntil, got %v", entry.ClaimedUntil)
	}

	if !claimable(entry, now) {
		t.Fatal("a freshly inserted, due entry must be claimable; nothing would ever fire otherwise")
	}
}

func TestClaimable(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		entry func() *Entry
		want  bool
	}{
		{
			name:  "due and never claimed",
			entry: func() *Entry { return dueEntry(now.Add(-time.Minute)) },
			want:  true,
		},
		{
			name:  "not due yet",
			entry: func() *Entry { return dueEntry(now.Add(time.Minute)) },
			want:  false,
		},
		{
			name: "due exactly now",
			entry: func() *Entry {
				return dueEntry(now)
			},
			want: true,
		},
		{
			name: "leased with a live lease",
			entry: func() *Entry {
				e := dueEntry(now.Add(-time.Minute))
				applyClaim(e, "other-instance", now.Add(-time.Second), time.Minute)

				return e
			},
			want: false,
		},
		{
			name: "leased with an expired lease is re-claimable",
			entry: func() *Entry {
				e := dueEntry(now.Add(-time.Hour))
				applyClaim(e, "crashed-instance", now.Add(-time.Minute), time.Second)

				return e
			},
			want: true,
		},
		{
			name: "already done",
			entry: func() *Entry {
				e := dueEntry(now.Add(-time.Minute))
				e.State = StateDone

				return e
			},
			want: false,
		},
		{
			name: "dead lettered",
			entry: func() *Entry {
				e := dueEntry(now.Add(-time.Minute))
				e.State = StateDead

				return e
			},
			want: false,
		},
		{
			name:  "nil entry",
			entry: func() *Entry { return nil },
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimable(tt.entry(), now); got != tt.want {
				t.Errorf("claimable = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClaimFilter_MatchesDocumentsWithoutClaimedUntil guards the query half of
// the same bug. MongoDB range queries are type-bracketed, so a filter of
// {"claimedUntil": {"$lte": now}} matches no document that lacks the field. If
// the exists branch is ever dropped, an entry written by any path that forgets
// claimedUntil becomes permanently unclaimable.
func TestClaimFilter_MatchesDocumentsWithoutClaimedUntil(t *testing.T) {
	filter := claimFilter(time.Now())

	branches, ok := lookup(filter, "$or").(bson.A)
	if !ok {
		t.Fatalf("claim filter has no $or over claimedUntil: %v", filter)
	}

	var sawExistsFalse, sawRange bool

	for _, branch := range branches {
		condition, ok := lookup(branch.(bson.D), "claimedUntil").(bson.D)
		if !ok {
			t.Fatalf("$or branch does not constrain claimedUntil: %v", branch)
		}

		if lookup(condition, "$exists") == false {
			sawExistsFalse = true
		}

		if lookup(condition, "$lte") != nil {
			sawRange = true
		}
	}

	if !sawExistsFalse {
		t.Error("claim filter must match documents with no claimedUntil at all, or they never fire")
	}

	if !sawRange {
		t.Error("claim filter must match documents whose lease has expired")
	}
}

// TestEntry_AlwaysPersistsClaimedUntil is the other half of that guard: the
// stored document must carry claimedUntil even when it is the zero time. An
// omitempty here would drop the field and put every entry back in the
// unclaimable state the $exists branch exists to rescue.
func TestEntry_AlwaysPersistsClaimedUntil(t *testing.T) {
	raw, err := bson.Marshal(&Entry{ID: bson.NewObjectID()})
	if err != nil {
		t.Fatalf("marshalling entry: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshalling entry: %v", err)
	}

	for _, field := range []string{"claimedUntil", "state", "fireAt", "gameId", "attempts", "scriptVersion", "idempotencyKey"} {
		if _, present := doc[field]; !present {
			t.Errorf("stored document is missing %q: %v", field, doc)
		}
	}
}

// TestClaimUpdate_MatchesApplyClaim keeps the BSON update and its Go mirror in
// step. They are two statements of one rule, and a change to either alone is a
// silent divergence between what the store does and what the tests check.
func TestClaimUpdate_MatchesApplyClaim(t *testing.T) {
	now := mongoTime(time.Now())
	lease := 30 * time.Second

	entry := dueEntry(now.Add(-time.Minute))
	entry.Attempts = 2
	applyClaim(entry, "instance-a", now, lease)

	update := claimUpdate("instance-a", now, lease)

	set, ok := lookup(update, "$set").(bson.D)
	if !ok {
		t.Fatalf("claim update has no $set: %v", update)
	}

	if got := lookup(set, "state"); got != entry.State {
		t.Errorf("$set state = %v, applyClaim set %v", got, entry.State)
	}

	if got := lookup(set, "claimedBy"); got != entry.ClaimedBy {
		t.Errorf("$set claimedBy = %v, applyClaim set %v", got, entry.ClaimedBy)
	}

	if got := lookup(set, "claimedUntil"); got != entry.ClaimedUntil {
		t.Errorf("$set claimedUntil = %v, applyClaim set %v", got, entry.ClaimedUntil)
	}

	inc, ok := lookup(update, "$inc").(bson.D)
	if !ok {
		t.Fatalf("claim update has no $inc: %v", update)
	}

	if got := lookup(inc, "attempts"); got != 1 || entry.Attempts != 3 {
		t.Errorf("$inc attempts = %v and applyClaim reached %d, want 1 and 3", got, entry.Attempts)
	}
}

// modelStore is an in-memory stand-in for the collection. It drives the
// production claimable/applyClaim pair under a mutex, which gives the same
// all-or-nothing guarantee FindOneAndUpdate gives: the predicate and the write
// cannot interleave with another claimer. It therefore proves the state
// machine never hands one entry to two claimers, and assumes — rather than
// proves — MongoDB's atomicity. TestClaimDue_ConcurrentClaimersGetDisjointEntries
// proves that part against a real database.
type modelStore struct {
	mu      sync.Mutex
	entries []*Entry
}

func (m *modelStore) claim(owner string, now time.Time, lease time.Duration) *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range m.entries {
		if !claimable(entry, now) {
			continue
		}

		applyClaim(entry, owner, now, lease)
		claimed := *entry

		return &claimed
	}

	return nil
}

// TestClaimStateMachine_ConcurrentClaimersAreDisjoint runs many claimers at
// once over one set of due entries: every entry must be handed out exactly
// once, and no entry twice.
func TestClaimStateMachine_ConcurrentClaimersAreDisjoint(t *testing.T) {
	const (
		entryCount   = 200
		claimerCount = 8
	)

	now := time.Now()

	model := &modelStore{}
	for i := 0; i < entryCount; i++ {
		model.entries = append(model.entries, dueEntry(now.Add(-time.Duration(i)*time.Second)))
	}

	claimed := make([][]*Entry, claimerCount)

	var wg sync.WaitGroup

	for i := 0; i < claimerCount; i++ {
		wg.Add(1)

		go func(claimer int) {
			defer wg.Done()

			owner := fmt.Sprintf("instance-%d", claimer)

			for {
				entry := model.claim(owner, now, time.Minute)
				if entry == nil {
					return
				}

				claimed[claimer] = append(claimed[claimer], entry)
			}
		}(i)
	}

	wg.Wait()

	seen := map[bson.ObjectID]string{}

	for claimer, entries := range claimed {
		owner := fmt.Sprintf("instance-%d", claimer)

		for _, entry := range entries {
			if previous, duplicate := seen[entry.ID]; duplicate {
				t.Fatalf("entry %v claimed twice: by %s and by %s", entry.ID.Hex(), previous, owner)
			}

			seen[entry.ID] = owner

			if entry.State != StateLeased || entry.ClaimedBy != owner || entry.Attempts != 1 {
				t.Fatalf("claimed entry not leased to its claimer: %+v", entry)
			}
		}
	}

	if len(seen) != entryCount {
		t.Errorf("claimed %d entries, want all %d", len(seen), entryCount)
	}
}

// TestFailureOutcome_PermanentFailureIsDeadNotDone: a retry keeps the entry
// pending with a backoff, but once the attempts are spent the entry is dead
// lettered rather than marked done, so the failure stays visible.
func TestFailureOutcome_PermanentFailureIsDeadNotDone(t *testing.T) {
	cfg := Config{MaxAttempts: 3, RetryBackoff: time.Second, MaxRetryBackoff: time.Minute}.withDefaults()
	now := time.Now()

	entry := dueEntry(now.Add(-time.Minute))

	for entry.Attempts = 1; entry.Attempts < cfg.MaxAttempts; entry.Attempts++ {
		state, fireAt := failureOutcome(entry, now, cfg)

		if state != StatePending {
			t.Fatalf("attempt %d: state = %v, want %v", entry.Attempts, state, StatePending)
		}

		if !fireAt.After(now) {
			t.Fatalf("attempt %d: retry at %v is not in the future", entry.Attempts, fireAt)
		}
	}

	state, _ := failureOutcome(entry, now, cfg)
	if state != StateDead {
		t.Errorf("after %d attempts state = %v, want %v — a permanent failure must not look like a success",
			entry.Attempts, state, StateDead)
	}

	if state == StateDone {
		t.Error("a permanent failure must never be marked done")
	}
}

func TestRetryDelay_BacksOffAndCaps(t *testing.T) {
	cfg := Config{RetryBackoff: time.Second, MaxRetryBackoff: 8 * time.Second}.withDefaults()

	want := []time.Duration{time.Second, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
	for attempts, expected := range want {
		if got := retryDelay(attempts, cfg); got != expected {
			t.Errorf("retryDelay(%d) = %v, want %v", attempts, got, expected)
		}
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	cfg := Config{}.withDefaults()

	if cfg.InstanceID == "" {
		t.Error("an instance id must be generated, or every instance shares one lease identity")
	}

	if cfg.LeaseTTL != defaultLeaseTTL || cfg.MaxAttempts != defaultMaxAttempts ||
		cfg.RetryBackoff != defaultRetryBackoff || cfg.OrphanGrace != defaultOrphanGrace {
		t.Errorf("zero config did not pick up defaults: %+v", cfg)
	}

	// An orphan TTL is what stops timers for abandoned games accumulating
	// forever, since nothing in this repo ever deletes a game.
	if cfg.OrphanGrace <= 0 {
		t.Error("orphan grace must be positive or nothing expires abandoned entries")
	}

	custom := Config{InstanceID: "fixed", LeaseTTL: time.Second, RetryBackoff: time.Hour}.withDefaults()
	if custom.InstanceID != "fixed" || custom.LeaseTTL != time.Second {
		t.Errorf("explicit config was overwritten: %+v", custom)
	}

	if custom.MaxRetryBackoff < custom.RetryBackoff {
		t.Errorf("max backoff %v is below the base backoff %v", custom.MaxRetryBackoff, custom.RetryBackoff)
	}
}

func TestCreateEntry_Validate(t *testing.T) {
	valid := CreateEntry{GameID: "game-1", Action: "turn_timeout", FireAt: time.Now()}

	tests := map[string]CreateEntry{
		"missing game id":   {Action: valid.Action, FireAt: valid.FireAt},
		"missing action":    {GameID: valid.GameID, FireAt: valid.FireAt},
		"missing fire time": {GameID: valid.GameID, Action: valid.Action},
	}

	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			if err := create.validate(); err == nil {
				t.Errorf("%s was accepted", name)
			}
		})
	}

	if err := valid.validate(); err != nil {
		t.Errorf("a complete entry was rejected: %v", err)
	}
}

// lookup returns the value of key in doc, or nil when it is absent.
func lookup(doc bson.D, key string) interface{} {
	for _, element := range doc {
		if element.Key == key {
			return element.Value
		}
	}

	return nil
}

// --- Integration: these need a reachable MongoDB and skip without one ---

// newTestStore connects to a local MongoDB (a single-node replica set is not
// required) and empties the schedule collection so each test starts clean. It
// skips when no database is reachable.
func newTestStore(t *testing.T, cfg Config) *Store {
	t.Helper()

	uri := os.Getenv("INDRI_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}

	_ = os.Setenv("INDRI_MONGO_URI", uri)
	_ = os.Setenv("INDRI_MONGO_DATABASE", "indri_test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongodb.New(ctx)
	if err != nil {
		t.Skipf("skipping: MongoDB not reachable: %v", err)
	}

	store, err := NewStore(context.Background(), client, cfg)
	if err != nil {
		t.Skipf("skipping: could not create schedule store: %v", err)
	}

	return store
}

// peerStore returns a second store over the same collection, standing in for
// another server instance.
func peerStore(t *testing.T, store *Store, cfg Config) *Store {
	t.Helper()

	return &Store{
		ctx:        store.ctx,
		client:     store.client,
		collection: store.collection,
		cfg:        cfg.withDefaults(),
	}
}

func clearSchedule(t *testing.T, store *Store) {
	t.Helper()

	if _, err := store.collection.Collection().DeleteMany(*store.ctx, bson.D{}); err != nil {
		t.Fatalf("clearing schedule collection: %v", err)
	}
}

func mustSchedule(t *testing.T, store *Store, gameID string, fireAt time.Time) *Entry {
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

// TestSchedule_FreshlyInsertedEntryIsClaimed proves against a real MongoDB
// that a brand new entry is reachable by the claim predicate — the failure
// mode in which no timer in the system ever fires.
func TestSchedule_FreshlyInsertedEntryIsClaimed(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a"})
	clearSchedule(t, store)

	scheduled := mustSchedule(t, store, "game-fresh", time.Now().Add(-time.Minute))

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
}

// TestClaimDue_ConcurrentClaimersGetDisjointEntries is the real-database proof
// that FindOneAndUpdate hands each due entry to exactly one instance.
func TestClaimDue_ConcurrentClaimersGetDisjointEntries(t *testing.T) {
	const (
		entryCount   = 40
		claimerCount = 4
	)

	store := newTestStore(t, Config{InstanceID: "instance-0"})
	clearSchedule(t, store)

	fireAt := time.Now().Add(-time.Minute)
	for i := 0; i < entryCount; i++ {
		mustSchedule(t, store, "game-concurrent", fireAt)
	}

	claimers := make([]*Store, claimerCount)
	claimers[0] = store

	for i := 1; i < claimerCount; i++ {
		claimers[i] = peerStore(t, store, Config{InstanceID: fmt.Sprintf("instance-%d", i)})
	}

	now := time.Now()
	claimed := make([][]*Entry, claimerCount)
	errs := make([]error, claimerCount)

	var wg sync.WaitGroup

	for i, claimer := range claimers {
		wg.Add(1)

		go func(index int, claimer *Store) {
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
}

// TestLease_ExpiredIsReclaimableAndRenewalHoldsIt covers both halves of the
// lease: an instance that crashed mid-dispatch must not hold an entry forever,
// and an instance that is still working must be able to keep it.
func TestLease_ExpiredIsReclaimableAndRenewalHoldsIt(t *testing.T) {
	const lease = time.Minute

	storeA := newTestStore(t, Config{InstanceID: "instance-a", LeaseTTL: lease})
	clearSchedule(t, storeA)

	storeB := peerStore(t, storeA, Config{InstanceID: "instance-b", LeaseTTL: lease})

	claimedAt := time.Now()
	mustSchedule(t, storeA, "game-lease", claimedAt.Add(-time.Minute))

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
	// MongoDB stores times to the millisecond, so let the clock move on before
	// renewing: otherwise the renewed lease can land in the same millisecond
	// as the original and there is no window left to assert against.
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

	// Once the renewed lease lapses, the entry is claimable again — otherwise
	// an instance that crashed mid-dispatch would strand it forever.
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

	// The instance that lost the entry must find out rather than keep working.
	if _, err := storeA.RenewLease(held.ID.Hex()); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("renewing a lost lease returned %v, want %v", err, ErrLeaseLost)
	}

	if err := storeA.Complete(held.ID.Hex()); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("completing a lost lease returned %v, want %v", err, ErrLeaseLost)
	}
}

func TestComplete_MarksDoneAndStopsRefiring(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})
	clearSchedule(t, store)

	claimedAt := time.Now()
	mustSchedule(t, store, "game-complete", claimedAt.Add(-time.Minute))

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
}

// TestFail_RetriesThenDeadLetters walks an action that keeps failing all the
// way to the dead letter, and checks it is never silently marked done.
func TestFail_RetriesThenDeadLetters(t *testing.T) {
	cfg := Config{
		InstanceID:  "instance-a",
		LeaseTTL:    time.Minute,
		MaxAttempts: 2,
		// Keep the backoff small so the retry is due immediately.
		RetryBackoff:    time.Millisecond,
		MaxRetryBackoff: time.Millisecond,
	}

	store := newTestStore(t, cfg)
	clearSchedule(t, store)

	now := time.Now()
	scheduled := mustSchedule(t, store, "game-fail", now.Add(-time.Minute))

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
}

// TestMarkDead_RetiresAnEntryThatCanNeverRun covers the script-upgrade case:
// the persisted action no longer exists, so the entry is dead lettered rather
// than dispatched into a router that would quietly match nothing.
func TestMarkDead_RetiresAnEntryThatCanNeverRun(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})
	clearSchedule(t, store)

	now := time.Now()
	mustSchedule(t, store, "game-upgrade", now.Add(-time.Minute))

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

	if dead.State == StateDone {
		t.Error("a permanent failure must never be recorded as done")
	}
}

func TestCancel_RemovesPendingAndRefusesClaimed(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})
	clearSchedule(t, store)

	now := time.Now()
	pending := mustSchedule(t, store, "game-cancel", now.Add(time.Hour))

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

	// A cancel that loses the race with a claim must say so, not pretend the
	// action was called off while it is already being dispatched.
	claimedEntry := mustSchedule(t, store, "game-cancel", now.Add(-time.Minute))

	if _, err := store.ClaimDue(now); err != nil {
		t.Fatalf("claiming entry: %v", err)
	}

	if err := store.Cancel(claimedEntry.ID.Hex()); !errors.Is(err, ErrAlreadyClaimed) {
		t.Errorf("cancelling a claimed entry returned %v, want %v", err, ErrAlreadyClaimed)
	}
}

// TestCancelForGame_CancelsEveryPendingTimerForOneGame is the only cleanup
// path for a game nobody returns to: nothing in this repo deletes a game, so
// without this the timers would sit until their TTL.
func TestCancelForGame_CancelsEveryPendingTimerForOneGame(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a", LeaseTTL: time.Minute})
	clearSchedule(t, store)

	now := time.Now()

	for i := 0; i < 3; i++ {
		mustSchedule(t, store, "game-abandoned", now.Add(time.Duration(i+1)*time.Hour))
	}

	survivor := mustSchedule(t, store, "game-other", now.Add(time.Hour))

	// One entry for the abandoned game is mid-dispatch and must be left alone.
	inFlight := mustSchedule(t, store, "game-abandoned", now.Add(-time.Minute))

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
}

// TestSchedule_IdempotencyKeyDeduplicates: delivery is at-least-once, so the
// key has to be stable and unique. Scheduling the same logical event twice
// must not queue the action twice.
func TestSchedule_IdempotencyKeyDeduplicates(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a"})
	clearSchedule(t, store)

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
}

// TestEntriesSurviveStoreRestart: timers are durable, so a new store over the
// same collection still sees what the old one scheduled.
func TestEntriesSurviveStoreRestart(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a"})
	clearSchedule(t, store)

	scheduled := mustSchedule(t, store, "game-restart", time.Now().Add(-time.Minute))

	restarted := newTestStore(t, Config{InstanceID: "instance-after-restart"})

	claimed, err := restarted.ClaimDue(time.Now())
	if err != nil {
		t.Fatalf("claiming after restart: %v", err)
	}

	if claimed == nil || claimed.ID != scheduled.ID {
		t.Fatalf("a scheduled entry did not survive the restart: got %v", claimed)
	}
}

// TestIndexes_TTLOnFireAt: the TTL is keyed on fireAt, not on firedAt, so an
// entry for an abandoned game expires even though it never fired.
func TestIndexes_TTLOnFireAt(t *testing.T) {
	store := newTestStore(t, Config{InstanceID: "instance-a"})

	cursor, err := store.collection.Collection().Indexes().List(*store.ctx)
	if err != nil {
		t.Fatalf("listing indexes: %v", err)
	}

	var indexes []bson.M
	if err := cursor.All(*store.ctx, &indexes); err != nil {
		t.Fatalf("reading indexes: %v", err)
	}

	var ttlOnFireAt, gameIDIndex bool

	for _, index := range indexes {
		keys, ok := index["key"].(bson.M)
		if !ok {
			continue
		}

		if _, keyed := keys["fireAt"]; keyed && len(keys) == 1 {
			if _, hasTTL := index["expireAfterSeconds"]; hasTTL {
				ttlOnFireAt = true
			}
		}

		if _, keyed := keys["gameId"]; keyed {
			gameIDIndex = true
		}
	}

	if !ttlOnFireAt {
		t.Error("no TTL index on fireAt: timers for abandoned games would never expire")
	}

	if !gameIDIndex {
		t.Error("no index on gameId: CancelForGame would scan the collection")
	}
}

package schedule

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
)

// The tests in this file come in two halves. The store's behaviour as a whole
// lives in storer_contract_test.go, which runs against both backends.
//
// The first half here exercises the claim/lease state machine directly, and
// ties each BSON predicate to the Go function that mirrors it. Those mirrors
// are what let the in-memory store share the real policy rather than restate
// it, so a drift between the two would quietly make the in-memory runs
// meaningless. Every test in this half is pure and always runs.
//
// The second half is what only a real database can answer: that the indexes
// the store depends on exist. It skips when MongoDB is unreachable.

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

// TestHeldBy_MirrorsLeaseFilter keeps the lease fence and its Go mirror in
// step. leaseFilter is what stops a straggler overwriting the outcome of the
// instance that took its entry over; heldBy is the same rule for the in-memory
// store. If they drift, one backend enforces the fence and the other does not,
// and the in-memory runs stop meaning anything.
func TestHeldBy_MirrorsLeaseFilter(t *testing.T) {
	store := &Store{cfg: Config{InstanceID: "instance-a"}.withDefaults()}
	entry := dueEntry(time.Now().Add(-time.Minute))

	filter := store.leaseFilter(entry.ID)

	if lookup(filter, "_id") != entry.ID {
		t.Errorf("lease filter does not pin the entry id: %v", filter)
	}

	if lookup(filter, "state") != StateLeased {
		t.Errorf("lease filter does not require a leased entry: %v", filter)
	}

	if lookup(filter, "claimedBy") != "instance-a" {
		t.Errorf("lease filter does not require this instance to be the holder: %v", filter)
	}

	tests := []struct {
		name  string
		state State
		owner string
		want  bool
	}{
		{name: "leased to this instance", state: StateLeased, owner: "instance-a", want: true},
		{name: "leased to another instance", state: StateLeased, owner: "instance-b", want: false},
		{name: "never claimed", state: StatePending, owner: "", want: false},
		{name: "already done", state: StateDone, owner: "instance-a", want: false},
		{name: "dead lettered", state: StateDead, owner: "instance-a", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := &Entry{ID: entry.ID, State: tt.state, ClaimedBy: tt.owner}

			if got := heldBy(held, "instance-a"); got != tt.want {
				t.Errorf("heldBy = %v, want %v", got, tt.want)
			}
		})
	}

	if heldBy(nil, "instance-a") {
		t.Error("a missing entry must not read as held; nothing matches the filter either")
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

// TestIndexes_TTLOnFireAt: the TTL is keyed on fireAt, not on firedAt, so an
// entry for an abandoned game expires even though it never fired.
//
// Only MongoDB can answer this, so the body is a "mongodb" subtest like the
// ones in the contract suite. The naming is load-bearing: CI asserts that with
// no database reachable, nothing outside a "/mongodb" subtest skips.
func TestIndexes_TTLOnFireAt(t *testing.T) {
	t.Run("mongodb", func(t *testing.T) {
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
	})
}

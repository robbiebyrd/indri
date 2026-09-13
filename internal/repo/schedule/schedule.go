// Package schedule is the durable store for deferred game events.
//
// A game script asks for an action to run later; the entry is written to
// MongoDB and survives a restart. Any number of server instances poll the same
// collection, and each due entry is leased to exactly one of them by an atomic
// FindOneAndUpdate — the same compare-and-set idiom the game store uses for its
// version fence. Mongo is the only mandatory dependency in this stack, so the
// scheduler works in the default single-instance mode as well as under
// INDRI_LOCK_BACKEND=redis; nothing here needs Redis.
//
// Delivery is at-least-once, never exactly-once. An instance can crash after
// dispatching an action but before marking the entry done, and the entry is
// then re-claimed once its lease expires. Every entry therefore carries an
// idempotency key so a duplicate delivery can be absorbed downstream.
//
// Entries are also cleaned up unconditionally. There is no game deletion
// anywhere in this repo, so nothing will ever cascade-cancel the timers of an
// abandoned game; a TTL index on fireAt expires every entry a generous grace
// window after its fire time, whether or not it ever fired.
package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/chenmingyong0423/go-mongox/v2"
	"github.com/chenmingyong0423/go-mongox/v2/builder/query"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
)

var collectionName = "schedule"

// State is the lifecycle position of a scheduled entry. It is explicit on the
// document rather than inferred from timestamps, so "waiting", "running",
// "finished" and "gave up" can never be confused with one another.
type State string

const (
	// StatePending is waiting for its fire time; claimable once due.
	StatePending State = "pending"
	// StateLeased is held by one instance that is dispatching it right now.
	StateLeased State = "leased"
	// StateDone fired and was acknowledged. Terminal.
	StateDone State = "done"
	// StateDead failed permanently — retries exhausted, or an action name that
	// no longer exists after a script upgrade. Terminal, and deliberately not
	// StateDone: a permanent failure must stay visible instead of looking like
	// a success.
	StateDead State = "dead"
)

// Sentinel errors callers are expected to branch on.
var (
	// ErrLeaseLost means the entry is no longer held by this instance: the
	// lease expired and another instance claimed it, or the entry reached a
	// terminal state. The caller must stop working on it — someone else is.
	ErrLeaseLost = errors.New("schedule entry lease lost")

	// ErrAlreadyClaimed means a cancel lost the race with a claim. The action
	// is already being dispatched and can no longer be called off.
	ErrAlreadyClaimed = errors.New("schedule entry already claimed")

	// ErrNotFound means no entry is stored under this id. Both backends wrap it
	// so a caller can tell "there is nothing here" — which is the ordinary
	// outcome of cancelling a timer that already fired — apart from "the store
	// could not answer", which is not.
	ErrNotFound = errors.New("schedule entry not found")
)

// Defaults applied to a zero Config.
const (
	defaultLeaseTTL        = 30 * time.Second
	defaultMaxAttempts     = 5
	defaultRetryBackoff    = 10 * time.Second
	defaultMaxRetryBackoff = 10 * time.Minute

	// defaultOrphanGrace is how long after its fire time an entry is kept
	// before the TTL index removes it. It is generous on purpose: it is a
	// leak stop, not a retry deadline.
	defaultOrphanGrace = 7 * 24 * time.Hour
)

// Entry is one deferred event as stored in MongoDB.
type Entry struct {
	ID      bson.ObjectID          `bson:"_id"               json:"id"`
	GameID  string                 `bson:"gameId"            json:"gameId"`
	Action  string                 `bson:"action"            json:"action"`
	Payload map[string]interface{} `bson:"payload,omitempty" json:"payload,omitempty"`

	// ScriptVersion records which script authored the entry, so an action name
	// that disappeared in an upgrade can be dead-lettered instead of silently
	// doing nothing (router.Dispatch succeeds quietly when nothing matches).
	ScriptVersion string `bson:"scriptVersion" json:"scriptVersion"`

	// IdempotencyKey identifies the logical event. Delivery is at-least-once,
	// so the handler uses this to absorb a duplicate fire.
	IdempotencyKey string `bson:"idempotencyKey" json:"idempotencyKey"`

	State  State     `bson:"state"  json:"state"`
	FireAt time.Time `bson:"fireAt" json:"fireAt"`

	// ClaimedUntil is when the current lease expires. It is always written on
	// insert (as the zero time) so the claim predicate can compare it; a
	// document missing the field would never match a "$lte now" filter and
	// would therefore never fire.
	ClaimedUntil time.Time `bson:"claimedUntil"        json:"claimedUntil"`
	ClaimedBy    string    `bson:"claimedBy,omitempty" json:"claimedBy,omitempty"`

	Attempts  int        `bson:"attempts"            json:"attempts"`
	LastError string     `bson:"lastError,omitempty" json:"lastError,omitempty"`
	FiredAt   *time.Time `bson:"firedAt,omitempty"   json:"firedAt,omitempty"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// CreateEntry is the caller-supplied half of a new schedule entry; the store
// owns everything else on the document.
type CreateEntry struct {
	// ID, when set, is the identity the entry is stored under; the store mints
	// one when it is empty.
	//
	// It exists because a caller can need the id before the write happens.
	// indri.after hands a script its timer's id the moment it is called, while
	// the entry itself is only written when the effect ledger flushes — so a
	// script can store the id in the very game state whose commit releases the
	// write. See NewEntryID.
	ID string

	GameID        string
	Action        string
	Payload       map[string]interface{}
	ScriptVersion string

	// IdempotencyKey may be left empty, in which case the store generates one.
	// Supplying it makes scheduling itself idempotent: a second Schedule with
	// the same key returns the entry already stored rather than a duplicate.
	IdempotencyKey string

	FireAt time.Time
}

// validate rejects an entry that could never be dispatched.
func (c CreateEntry) validate() error {
	switch {
	case c.GameID == "":
		return fmt.Errorf("schedule entry must have a game id")
	case c.Action == "":
		return fmt.Errorf("schedule entry must have an action")
	case c.FireAt.IsZero():
		return fmt.Errorf("schedule entry must have a fire time")
	}

	// Rejected here rather than silently replaced with a fresh id: the caller
	// supplying one is already holding it, and quietly storing the entry
	// somewhere else would leave them with a handle that cancels nothing.
	if c.ID != "" {
		if _, err := bson.ObjectIDFromHex(c.ID); err != nil {
			return fmt.Errorf("parsing the supplied schedule entry id %q: %w", c.ID, err)
		}
	}

	return nil
}

// Config tunes the store. The zero value is valid; withDefaults fills it in.
type Config struct {
	// InstanceID identifies this process in a lease. Generated when empty.
	InstanceID string
	// LeaseTTL is how long a claim is held before another instance may take
	// the entry over. It must exceed the time a handler needs, or the handler
	// must renew the lease while it runs.
	LeaseTTL time.Duration
	// MaxAttempts is how many claims an entry gets before a failure is
	// permanent and the entry is dead-lettered.
	MaxAttempts int
	// RetryBackoff is the delay before the first retry; it doubles per attempt
	// up to MaxRetryBackoff.
	RetryBackoff    time.Duration
	MaxRetryBackoff time.Duration
	// OrphanGrace is the TTL applied to fireAt.
	OrphanGrace time.Duration
}

// withDefaults returns a copy of cfg with every unset field filled in. Kept
// pure so the defaulting is testable without a database.
func (c Config) withDefaults() Config {
	if c.InstanceID == "" {
		c.InstanceID = randomID()
	}

	if c.LeaseTTL <= 0 {
		c.LeaseTTL = defaultLeaseTTL
	}

	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultMaxAttempts
	}

	if c.RetryBackoff <= 0 {
		c.RetryBackoff = defaultRetryBackoff
	}

	if c.MaxRetryBackoff < c.RetryBackoff {
		c.MaxRetryBackoff = max(defaultMaxRetryBackoff, c.RetryBackoff)
	}

	if c.OrphanGrace <= 0 {
		c.OrphanGrace = defaultOrphanGrace
	}

	return c
}

type Store struct {
	ctx        *context.Context
	collection *mongox.Collection[Entry]
	client     *mongodb.Client
	cfg        Config
}

// NewStore creates a new repository for deferred game events, creating its
// indexes (including the TTL index that expires orphaned entries).
func NewStore(ctx context.Context, client *mongodb.Client, cfg Config) (*Store, error) {
	if client == nil {
		return nil, fmt.Errorf("schedule store requires a mongodb client")
	}

	cfg = cfg.withDefaults()

	scheduleColl := mongox.NewCollection[Entry](client.Database, collectionName)

	indexModels := []mongo.IndexModel{
		{
			// Serves the claim predicate: due, unleased, not yet terminal.
			Keys: bson.D{
				{Key: "state", Value: 1},
				{Key: "fireAt", Value: 1},
				{Key: "claimedUntil", Value: 1},
			},
		},
		{
			// Serves CancelForGame and any per-game lookup.
			Keys: bson.D{
				{Key: "gameId", Value: 1},
				{Key: "state", Value: 1},
			},
		},
		{
			Keys:    bson.D{{Key: "idempotencyKey", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// TTL index: MongoDB removes an entry OrphanGrace after its fire
			// time. The TTL is on fireAt, not firedAt, on purpose — an entry
			// for an abandoned game is never claimed and so never gets a
			// firedAt, and nothing in this repo deletes a game, so a TTL keyed
			// on firing would leak that entry forever.
			Keys:    bson.D{{Key: "fireAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(cfg.OrphanGrace.Seconds())),
		},
	}

	_, err := scheduleColl.Collection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return nil, fmt.Errorf("creating schedule indexes: %w", err)
	}

	return &Store{
		ctx:        &ctx,
		client:     client,
		collection: scheduleColl,
		cfg:        cfg,
	}, nil
}

// InstanceID is the identity this store writes into a lease.
func (s *Store) InstanceID() string {
	return s.cfg.InstanceID
}

// Schedule stores a new deferred event. It is idempotent on the idempotency
// key: a second call with a key that is already stored returns the stored
// entry instead of scheduling the action twice.
func (s *Store) Schedule(create CreateEntry) (*Entry, error) {
	if err := create.validate(); err != nil {
		return nil, err
	}

	entry := newEntry(create)

	if _, err := s.collection.Collection().InsertOne(*s.ctx, entry); err != nil {
		// Lost a race with a concurrent schedule of the same logical event.
		// Return the winner rather than a raw duplicate-key error.
		if mongo.IsDuplicateKeyError(err) {
			return s.findByIdempotencyKey(entry.IdempotencyKey)
		}

		return nil, fmt.Errorf("scheduling action %q for game %q: %w", create.Action, create.GameID, err)
	}

	return entry, nil
}

// newEntry builds the document a new deferred event starts from. Both backends
// create entries through it, so an entry scheduled against the in-memory store
// starts life in exactly the shape a stored one does.
func newEntry(create CreateEntry) *Entry {
	if create.IdempotencyKey == "" {
		create.IdempotencyKey = randomID()
	}

	now := mongoTime(time.Now())

	// validate has already refused an unparseable id, so the error here can
	// only mean the caller skipped validation; a minted id is still better than
	// a zero one, which every later lookup would collide on.
	id := bson.NewObjectID()

	if create.ID != "" {
		if parsed, err := bson.ObjectIDFromHex(create.ID); err == nil {
			id = parsed
		}
	}

	return &Entry{
		ID:             id,
		GameID:         create.GameID,
		Action:         create.Action,
		Payload:        create.Payload,
		ScriptVersion:  create.ScriptVersion,
		IdempotencyKey: create.IdempotencyKey,
		State:          StatePending,
		FireAt:         mongoTime(create.FireAt),
		// Written explicitly, never left absent: the claim predicate compares
		// this field, and a missing field matches no range query.
		ClaimedUntil: time.Time{}.UTC(),
		Attempts:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Get retrieves a single entry by its ID.
func (s *Store) Get(id string) (*Entry, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	entry, err := s.collection.Finder().Filter(query.Id(objectID)).FindOne(*s.ctx)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("fetching schedule entry %q: %w", id, ErrNotFound)
		}

		return nil, fmt.Errorf("fetching schedule entry %q: %w", id, err)
	}

	return entry, nil
}

// ClaimDue leases the single oldest entry that is due at now, marking it
// StateLeased and incrementing its attempt count. It returns (nil, nil) when
// nothing is due. The read and the write are one atomic FindOneAndUpdate, so
// two instances polling at the same moment always get different entries.
func (s *Store) ClaimDue(now time.Time) (*Entry, error) {
	now = mongoTime(now)

	entry, err := s.collection.Collection().FindOneAndUpdate(
		*s.ctx,
		claimFilter(now),
		claimUpdate(s.cfg.InstanceID, now, s.cfg.LeaseTTL),
		options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "fireAt", Value: 1}}).
			SetReturnDocument(options.After),
	).Raw()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, fmt.Errorf("claiming due schedule entry: %w", err)
	}

	var claimed Entry
	if err := bson.Unmarshal(entry, &claimed); err != nil {
		return nil, fmt.Errorf("decoding claimed schedule entry: %w", err)
	}

	return &claimed, nil
}

// RenewLease extends the lease on an entry this instance still holds, so a
// slow handler is not overtaken by another instance mid-flight. It returns
// ErrLeaseLost once another instance has taken the entry over — the caller
// must then abandon the work, because someone else is already doing it.
//
// Ownership, not the clock, is the fence: a lapsed lease that nobody has
// claimed yet may still be renewed by the instance that is demonstrably still
// working on it. Forcing that handler to give up would lose work for nothing.
func (s *Store) RenewLease(id string) (*Entry, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	now := mongoTime(time.Now())

	renewed, err := s.findOneAndUpdate(
		s.leaseFilter(objectID),
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "claimedUntil", Value: now.Add(s.cfg.LeaseTTL)},
			{Key: "updatedAt", Value: now},
		}}},
	)
	if err != nil {
		return nil, fmt.Errorf("renewing lease on schedule entry %q: %w", id, err)
	}

	if renewed == nil {
		return nil, ErrLeaseLost
	}

	return renewed, nil
}

// Complete marks an entry as successfully fired. It is refused with
// ErrLeaseLost if this instance no longer holds the lease, so a straggler
// cannot overwrite the outcome of the instance that took the entry over.
func (s *Store) Complete(id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	now := mongoTime(time.Now())

	updated, err := s.findOneAndUpdate(
		s.leaseFilter(objectID),
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "state", Value: StateDone},
			{Key: "firedAt", Value: now},
			{Key: "updatedAt", Value: now},
		}}},
	)
	if err != nil {
		return fmt.Errorf("completing schedule entry %q: %w", id, err)
	}

	if updated == nil {
		return ErrLeaseLost
	}

	return nil
}

// Fail records a failed attempt on an entry this instance holds. The entry
// goes back to StatePending with a backoff while attempts remain, and becomes
// StateDead once they are spent — never StateDone, so an operator can still
// see that the action never ran. The returned entry carries the new state.
func (s *Store) Fail(id string, reason string) (*Entry, error) {
	current, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	now := mongoTime(time.Now())
	state, fireAt := failureOutcome(current, now, s.cfg)

	objectID := current.ID

	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "state", Value: state},
		{Key: "fireAt", Value: mongoTime(fireAt)},
		{Key: "claimedUntil", Value: time.Time{}.UTC()},
		{Key: "lastError", Value: reason},
		{Key: "updatedAt", Value: now},
	}}}

	// Fence on the attempt count as well as the lease: if the entry was
	// re-claimed between the read above and this write, attempts has moved on
	// and the update must not land.
	filter := append(s.leaseFilter(objectID), bson.E{Key: "attempts", Value: current.Attempts})

	failed, err := s.findOneAndUpdate(filter, update)
	if err != nil {
		return nil, fmt.Errorf("failing schedule entry %q: %w", id, err)
	}

	if failed == nil {
		return nil, ErrLeaseLost
	}

	return failed, nil
}

// MarkDead retires an entry that can never succeed — an action name that no
// longer exists after a script upgrade, say. It is StateDead rather than
// StateDone so the failure is not mistaken for delivery.
func (s *Store) MarkDead(id string, reason string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	now := mongoTime(time.Now())

	updated, err := s.findOneAndUpdate(
		s.leaseFilter(objectID),
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "state", Value: StateDead},
			{Key: "lastError", Value: reason},
			{Key: "updatedAt", Value: now},
		}}},
	)
	if err != nil {
		return fmt.Errorf("dead-lettering schedule entry %q: %w", id, err)
	}

	if updated == nil {
		return ErrLeaseLost
	}

	return nil
}

// Cancel removes a timer that has not been claimed yet. Cancelling is
// idempotent — an entry that is already gone is not an error — but a cancel
// that loses the race with a claim returns ErrAlreadyClaimed, because the
// action is being dispatched and can no longer be called off.
func (s *Store) Cancel(id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	result, err := s.collection.Collection().DeleteOne(*s.ctx, bson.D{
		{Key: "_id", Value: objectID},
		{Key: "state", Value: StatePending},
	})
	if err != nil {
		return fmt.Errorf("cancelling schedule entry %q: %w", id, err)
	}

	if result.DeletedCount > 0 {
		return nil
	}

	// Nothing was deleted: either the entry never existed (fine) or it is no
	// longer pending (the cancel lost the race).
	count, err := s.collection.Finder().Filter(query.Id(objectID)).Count(*s.ctx)
	if err != nil {
		return fmt.Errorf("checking cancelled schedule entry %q: %w", id, err)
	}

	if count > 0 {
		return ErrAlreadyClaimed
	}

	return nil
}

// CancelForGame removes every timer still pending for one game and reports how
// many went. It is the only way to clean up after a game nobody will return
// to, since nothing in this repo deletes a game. Entries already leased are
// left alone — they are mid-dispatch.
func (s *Store) CancelForGame(gameID string) (int64, error) {
	if gameID == "" {
		return 0, fmt.Errorf("cancelling timers requires a game id")
	}

	result, err := s.collection.Collection().DeleteMany(*s.ctx, bson.D{
		{Key: "gameId", Value: gameID},
		{Key: "state", Value: StatePending},
	})
	if err != nil {
		return 0, fmt.Errorf("cancelling timers for game %q: %w", gameID, err)
	}

	return result.DeletedCount, nil
}

// leaseFilter matches an entry only while this instance holds its lease.
func (s *Store) leaseFilter(objectID bson.ObjectID) bson.D {
	return bson.D{
		{Key: "_id", Value: objectID},
		{Key: "state", Value: StateLeased},
		{Key: "claimedBy", Value: s.cfg.InstanceID},
	}
}

// heldBy is the Go mirror of leaseFilter: it reports whether owner still holds
// the entry. Change one and you must change the other;
// TestHeldBy_MirrorsLeaseFilter fails if they drift.
func heldBy(e *Entry, owner string) bool {
	if e == nil {
		return false
	}

	return e.State == StateLeased && e.ClaimedBy == owner
}

// findOneAndUpdate applies update to the one document matching filter and
// returns it, or (nil, nil) when nothing matched.
func (s *Store) findOneAndUpdate(filter bson.D, update bson.D) (*Entry, error) {
	raw, err := s.collection.Collection().FindOneAndUpdate(
		*s.ctx,
		filter,
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Raw()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, err
	}

	var entry Entry
	if err := bson.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("decoding schedule entry: %w", err)
	}

	return &entry, nil
}

func (s *Store) findByIdempotencyKey(key string) (*Entry, error) {
	entry, err := s.collection.Finder().Filter(query.Eq("idempotencyKey", key)).FindOne(*s.ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching schedule entry for idempotency key %q: %w", key, err)
	}

	return entry, nil
}

// --- The claim state machine, kept pure so it can be tested without Mongo ---

// claimFilter is the predicate that selects a leasable entry at now: due, not
// terminal, and not already held by a live lease.
//
// The claimedUntil branch matters. A filter of {"claimedUntil": {"$lte": now}}
// alone matches no document that lacks the field, because MongoDB range
// queries are type-bracketed — a freshly inserted entry would never be claimed
// and the timer would never fire. Schedule always writes claimedUntil, and the
// $exists branch below covers any document written by another path.
func claimFilter(now time.Time) bson.D {
	return bson.D{
		{Key: "state", Value: bson.D{{Key: "$in", Value: bson.A{StatePending, StateLeased}}}},
		{Key: "fireAt", Value: bson.D{{Key: "$lte", Value: now}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "claimedUntil", Value: bson.D{{Key: "$lte", Value: now}}}},
			bson.D{{Key: "claimedUntil", Value: bson.D{{Key: "$exists", Value: false}}}},
		}},
	}
}

// claimUpdate is the write half of a claim.
func claimUpdate(owner string, now time.Time, lease time.Duration) bson.D {
	return bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "state", Value: StateLeased},
			{Key: "claimedBy", Value: owner},
			{Key: "claimedUntil", Value: now.Add(lease)},
			{Key: "updatedAt", Value: now},
		}},
		{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}},
	}
}

// claimable is the Go mirror of claimFilter: it reports whether e may be
// leased at now. Change one and you must change the other.
func claimable(e *Entry, now time.Time) bool {
	if e == nil {
		return false
	}

	if e.State != StatePending && e.State != StateLeased {
		return false
	}

	if e.FireAt.After(now) {
		return false
	}

	// The zero time is "never claimed", and must read as claimable.
	return !e.ClaimedUntil.After(now)
}

// applyClaim is the Go mirror of claimUpdate.
func applyClaim(e *Entry, owner string, now time.Time, lease time.Duration) {
	e.State = StateLeased
	e.ClaimedBy = owner
	e.ClaimedUntil = now.Add(lease)
	e.UpdatedAt = now
	e.Attempts++
}

// failureOutcome decides what a failed attempt becomes. Attempts are counted
// at claim time, so an entry on its final attempt is dead-lettered rather than
// rescheduled — it would only be claimed to fail again.
func failureOutcome(e *Entry, now time.Time, cfg Config) (State, time.Time) {
	if e.Attempts >= cfg.MaxAttempts {
		return StateDead, e.FireAt
	}

	return StatePending, now.Add(retryDelay(e.Attempts, cfg))
}

// retryDelay doubles the backoff per attempt, capped, so a failing action
// does not hot-loop against the database.
func retryDelay(attempts int, cfg Config) time.Duration {
	delay := cfg.RetryBackoff

	for i := 1; i < attempts; i++ {
		if delay >= cfg.MaxRetryBackoff {
			break
		}

		delay *= 2
	}

	if delay > cfg.MaxRetryBackoff {
		delay = cfg.MaxRetryBackoff
	}

	return delay
}

// mongoTime normalises a time to what MongoDB can store, so a value read back
// compares equal to the value written.
func mongoTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

// NewEntryID mints an identity a schedule entry can be stored under later.
//
// It is exported so a caller that must hand the id out before the entry exists
// does not have to know that an entry is keyed by a MongoDB ObjectID. Ids are
// globally unique by construction, so two callers minting at the same moment
// cannot collide.
func NewEntryID() string {
	return bson.NewObjectID().Hex()
}

// randomID returns an unguessable identifier, used for instance identity and
// for a generated idempotency key.
func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand never fails on the platforms this runs on; fall back to
		// a time-based value rather than panicking in a constructor.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(buf)
}

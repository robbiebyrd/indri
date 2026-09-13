package schedule

import (
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// MemoryStore is a schedule store that keeps its entries in a slice instead of
// in MongoDB. The scheduler that polls it, and the scripts that schedule
// through it, must be testable on a CI runner with no database — otherwise
// those tests skip themselves and report green without ever having run.
//
// The lifecycle it enforces is not a second implementation. Claiming goes
// through claimable and applyClaim, a failure through failureOutcome, and the
// lease fence through heldBy — the same functions the MongoDB store's queries
// are written to mirror, each with a test tying the two together. What differs
// is only where the entries live and what makes an operation atomic: a mutex
// here, FindOneAndUpdate there.
//
// It therefore cannot prove that MongoDB's FindOneAndUpdate really is atomic,
// or that the BSON claim predicate selects what claimable selects. Those are
// properties of the database, and the MongoDB-backed tests remain the layer
// that proves them.
//
// Several instances may share one set of entries; see Peer.
type MemoryStore struct {
	cfg     Config
	entries *memoryEntries
}

// memoryEntries is the shared entry set, standing in for the collection.
type memoryEntries struct {
	mu   sync.Mutex
	list []*Entry
}

// NewMemoryStore builds an in-memory schedule store.
func NewMemoryStore(cfg Config) *MemoryStore {
	return &MemoryStore{
		cfg:     cfg.withDefaults(),
		entries: &memoryEntries{},
	}
}

// Peer returns a second store over the same entries, standing in for another
// server instance polling the same collection.
func (s *MemoryStore) Peer(cfg Config) *MemoryStore {
	return &MemoryStore{
		cfg:     cfg.withDefaults(),
		entries: s.entries,
	}
}

// InstanceID is the identity this store writes into a lease.
func (s *MemoryStore) InstanceID() string {
	return s.cfg.InstanceID
}

// Schedule stores a new deferred event, idempotently on the idempotency key.
func (s *MemoryStore) Schedule(create CreateEntry) (*Entry, error) {
	if err := create.validate(); err != nil {
		return nil, err
	}

	entry := newEntry(create)

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	// The unique index on idempotencyKey is what makes a second schedule of
	// the same logical event return the winner instead of queueing the action
	// twice.
	for _, stored := range s.entries.list {
		if stored.IdempotencyKey == entry.IdempotencyKey {
			return copyEntry(stored), nil
		}

		// MongoDB refuses a second document under the same _id, and a caller
		// that supplied an id it was already holding needs to hear so rather
		// than end up with two entries one cancel can never reach.
		if stored.ID == entry.ID {
			return nil, fmt.Errorf("scheduling action %q: the entry id %q is already in use", create.Action, entry.ID.Hex())
		}
	}

	s.entries.list = append(s.entries.list, entry)

	return copyEntry(entry), nil
}

// Get retrieves a single entry by its ID.
func (s *MemoryStore) Get(id string) (*Entry, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	stored := s.entries.find(objectID)
	if stored == nil {
		return nil, fmt.Errorf("fetching schedule entry %q: %w", id, ErrNotFound)
	}

	return copyEntry(stored), nil
}

// ClaimDue leases the single oldest entry that is due at now. The mutex gives
// the predicate and the write the same all-or-nothing guarantee
// FindOneAndUpdate gives, so two instances polling at once get different
// entries.
func (s *MemoryStore) ClaimDue(now time.Time) (*Entry, error) {
	now = mongoTime(now)

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	var oldest *Entry

	for _, stored := range s.entries.list {
		if !claimable(stored, now) {
			continue
		}

		if oldest == nil || stored.FireAt.Before(oldest.FireAt) {
			oldest = stored
		}
	}

	if oldest == nil {
		return nil, nil
	}

	applyClaim(oldest, s.cfg.InstanceID, now, s.cfg.LeaseTTL)

	return copyEntry(oldest), nil
}

// RenewLease extends the lease on an entry this instance still holds, and
// reports ErrLeaseLost once another instance has taken it over.
func (s *MemoryStore) RenewLease(id string) (*Entry, error) {
	return s.updateHeld(id, func(e *Entry, now time.Time) {
		e.ClaimedUntil = now.Add(s.cfg.LeaseTTL)
	})
}

// Complete marks an entry this instance holds as successfully fired.
func (s *MemoryStore) Complete(id string) error {
	_, err := s.updateHeld(id, func(e *Entry, now time.Time) {
		e.State = StateDone
		e.FiredAt = &now
	})

	return err
}

// Fail records a failed attempt: a retry while attempts remain, a dead letter
// once they are spent — never done, so the failure stays visible.
func (s *MemoryStore) Fail(id string, reason string) (*Entry, error) {
	return s.updateHeld(id, func(e *Entry, now time.Time) {
		state, fireAt := failureOutcome(e, now, s.cfg)

		e.State = state
		e.FireAt = mongoTime(fireAt)
		e.ClaimedUntil = time.Time{}.UTC()
		e.LastError = reason
	})
}

// MarkDead retires an entry that can never succeed.
func (s *MemoryStore) MarkDead(id string, reason string) error {
	_, err := s.updateHeld(id, func(e *Entry, _ time.Time) {
		e.State = StateDead
		e.LastError = reason
	})

	return err
}

// Cancel removes a timer that has not been claimed yet. It is idempotent, but
// a cancel that loses the race with a claim reports ErrAlreadyClaimed.
func (s *MemoryStore) Cancel(id string) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("cancelling schedule entry %q: %w", id, err)
	}

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	stored := s.entries.find(objectID)
	if stored == nil {
		// Already gone; the timer will not fire either way.
		return nil
	}

	if stored.State != StatePending {
		return ErrAlreadyClaimed
	}

	s.entries.remove(func(e *Entry) bool { return e.ID == objectID })

	return nil
}

// CancelForGame removes every timer still pending for one game. Entries
// already leased are left alone — they are mid-dispatch.
func (s *MemoryStore) CancelForGame(gameID string) (int64, error) {
	if gameID == "" {
		return 0, fmt.Errorf("cancelling timers requires a game id")
	}

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	return s.entries.remove(func(e *Entry) bool {
		return e.GameID == gameID && e.State == StatePending
	}), nil
}

// updateHeld applies edit to the entry, but only while this instance still
// holds its lease, and returns the result. It is the in-memory leaseFilter.
func (s *MemoryStore) updateHeld(id string, edit func(e *Entry, now time.Time)) (*Entry, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	s.entries.mu.Lock()
	defer s.entries.mu.Unlock()

	stored := s.entries.find(objectID)
	if !heldBy(stored, s.cfg.InstanceID) {
		return nil, ErrLeaseLost
	}

	now := mongoTime(time.Now())

	edit(stored, now)

	stored.UpdatedAt = now

	return copyEntry(stored), nil
}

// find returns the stored entry with this id, or nil. The caller must hold the
// mutex.
func (m *memoryEntries) find(id bson.ObjectID) *Entry {
	for _, stored := range m.list {
		if stored.ID == id {
			return stored
		}
	}

	return nil
}

// remove deletes every entry matching match and reports how many went. The
// caller must hold the mutex.
func (m *memoryEntries) remove(match func(*Entry) bool) int64 {
	kept := make([]*Entry, 0, len(m.list))

	var removed int64

	for _, stored := range m.list {
		if match(stored) {
			removed++

			continue
		}

		kept = append(kept, stored)
	}

	m.list = kept

	return removed
}

// copyEntry returns a detached copy, so a caller holding a returned entry
// cannot edit the stored one behind the store's back — the same isolation a
// round trip to MongoDB gives.
func copyEntry(e *Entry) *Entry {
	copied := *e

	if e.Payload != nil {
		copied.Payload = make(map[string]interface{}, len(e.Payload))
		for k, v := range e.Payload {
			copied.Payload[k] = v
		}
	}

	if e.FiredAt != nil {
		firedAt := *e.FiredAt
		copied.FiredAt = &firedAt
	}

	return &copied
}

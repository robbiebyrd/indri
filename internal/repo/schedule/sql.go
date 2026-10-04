package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/repo/sqldialect"
)

// scheduleSchema is the PostgreSQL store's own DDL. SQLite's lives with the
// rest of its schema in internal/clients/sqlite, in the same shape: times are
// Unix milliseconds on both — the precision an entry is stored at everywhere,
// see mongoTime — so one set of statements serves both.
const scheduleSchema = `
CREATE TABLE IF NOT EXISTS schedule (
    id              TEXT    PRIMARY KEY,
    game_id         TEXT    NOT NULL,
    action          TEXT    NOT NULL,
    payload         BYTEA,
    script_version  TEXT    NOT NULL,
    idempotency_key TEXT    UNIQUE NOT NULL,
    state           TEXT    NOT NULL,
    fire_at         BIGINT  NOT NULL,
    claimed_until   BIGINT  NOT NULL,
    claimed_by      TEXT    NOT NULL,
    attempts        INTEGER NOT NULL,
    last_error      TEXT    NOT NULL,
    fired_at        BIGINT,
    created_at      BIGINT  NOT NULL,
    updated_at      BIGINT  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_schedule_claim   ON schedule (state, fire_at, claimed_until);
CREATE INDEX IF NOT EXISTS idx_schedule_game    ON schedule (game_id, state);
CREATE INDEX IF NOT EXISTS idx_schedule_fire_at ON schedule (fire_at);
`

// SQLStore is a schedule store over SQLite or PostgreSQL; the two differ only
// in their dialect. Every field of an entry has a column, so the claim
// predicate and the per-game cancel are plain WHERE clauses; the payload is
// BSON, as a game document is, so it reads back in exactly the shape the
// MongoDB store returns.
//
// Like the in-memory store, it is not a second implementation of the
// lifecycle. A claim goes through applyClaim, a lease-held change through
// heldBy and the edits the in-memory store applies, and a failure through
// failureOutcome. What makes each one atomic is a transaction holding the row:
// PostgreSQL locks it, and SQLite's pool is one connection, so a transaction
// already holds the whole database.
type SQLStore struct {
	ctx     context.Context
	db      *sql.DB
	dialect sqldialect.Dialect
	cfg     Config
}

// NewSQLiteStore builds a schedule store over an already-open SQLite database,
// whose schema internal/clients/sqlite.Open created. The caller owns db.
func NewSQLiteStore(ctx context.Context, db *sql.DB, cfg Config) *SQLStore {
	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.SQLite, cfg: cfg.withDefaults()}
}

// NewPostgresStore builds a schedule store over an already-open PostgreSQL
// pool and creates its table. The caller owns db.
func NewPostgresStore(ctx context.Context, db *sql.DB, cfg Config) (*SQLStore, error) {
	if _, err := db.ExecContext(ctx, scheduleSchema); err != nil {
		return nil, fmt.Errorf("creating the schedule table: %w", err)
	}

	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.Postgres, cfg: cfg.withDefaults()}, nil
}

// InstanceID is the identity this store writes into a lease.
func (s *SQLStore) InstanceID() string {
	return s.cfg.InstanceID
}

const entryColumns = `id, game_id, action, payload, script_version, idempotency_key, state,
	fire_at, claimed_until, claimed_by, attempts, last_error, fired_at, created_at, updated_at`

const selectEntries = `SELECT ` + entryColumns + ` FROM schedule`

// querier is what a read or write runs on: the pool, or a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Schedule stores a new deferred event, idempotently on the idempotency key.
//
// It also removes every entry OrphanGrace past its fire time. MongoDB does
// that with a TTL index, which SQL does not have, and scheduling is where a
// store gains entries, so it is where one that keeps growing gets to shrink.
func (s *SQLStore) Schedule(create CreateEntry) (*Entry, error) {
	if err := create.validate(); err != nil {
		return nil, err
	}

	entry := newEntry(create)

	orphanedBefore := mongoTime(time.Now()).Add(-s.cfg.OrphanGrace)
	if _, err := s.db.ExecContext(s.ctx, s.dialect.Rebind(`DELETE FROM schedule WHERE fire_at <= ?`),
		orphanedBefore.UnixMilli()); err != nil {
		return nil, fmt.Errorf("pruning orphaned schedule entries: %w", err)
	}

	payload, err := encodePayload(entry.Payload)
	if err != nil {
		return nil, fmt.Errorf("scheduling action %q for game %q: %w", create.Action, create.GameID, err)
	}

	_, err = s.db.ExecContext(s.ctx,
		s.dialect.Rebind(`INSERT INTO schedule (`+entryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		entry.ID.Hex(), entry.GameID, entry.Action, payload, entry.ScriptVersion, entry.IdempotencyKey,
		string(entry.State), entry.FireAt.UnixMilli(), entry.ClaimedUntil.UnixMilli(), entry.ClaimedBy,
		entry.Attempts, entry.LastError, nil, entry.CreatedAt.UnixMilli(), entry.UpdatedAt.UnixMilli())
	if s.dialect.IsUniqueViolation(err) {
		return s.scheduledEarlier(entry)
	}

	if err != nil {
		return nil, fmt.Errorf("scheduling action %q for game %q: %w", create.Action, create.GameID, err)
	}

	return entry, nil
}

// scheduledEarlier answers a Schedule the unique indexes refused. A stored
// entry with the same idempotency key is the same logical event, scheduled
// already, and is the answer. Without one the clash was on the id, which a
// caller supplied while already holding it, and has to hear so rather than end
// up with two entries one cancel can never reach.
func (s *SQLStore) scheduledEarlier(entry *Entry) (*Entry, error) {
	stored, err := scanEntry(s.db.QueryRowContext(s.ctx,
		s.dialect.Rebind(selectEntries+` WHERE idempotency_key = ?`), entry.IdempotencyKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("scheduling action %q: the entry id %q is already in use", entry.Action, entry.ID.Hex())
	}

	if err != nil {
		return nil, fmt.Errorf("fetching schedule entry for idempotency key %q: %w", entry.IdempotencyKey, err)
	}

	return stored, nil
}

// Get retrieves a single entry by its ID.
func (s *SQLStore) Get(id string) (*Entry, error) {
	if _, err := bson.ObjectIDFromHex(id); err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	return s.get(s.db, id, "")
}

// get reads one entry on q; lock is appended to the SELECT.
func (s *SQLStore) get(q querier, id string, lock string) (*Entry, error) {
	entry, err := scanEntry(q.QueryRowContext(s.ctx, s.dialect.Rebind(selectEntries+` WHERE id = ?`+lock), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("fetching schedule entry %q: %w", id, ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("fetching schedule entry %q: %w", id, err)
	}

	return entry, nil
}

// ClaimDue leases the single oldest entry that is due at now. It returns
// (nil, nil) when nothing is due.
//
// The WHERE clause is claimable written in SQL. The row it picks stays locked
// until the claim commits, and on PostgreSQL a row another instance is
// claiming is skipped rather than waited for, so two instances polling at once
// take different entries.
func (s *SQLStore) ClaimDue(now time.Time) (*Entry, error) {
	now = mongoTime(now)

	var claimed *Entry

	err := s.inTx(func(tx *sql.Tx) error {
		entry, err := scanEntry(tx.QueryRowContext(s.ctx, s.dialect.Rebind(selectEntries+
			` WHERE state IN (?, ?) AND fire_at <= ? AND claimed_until <= ? ORDER BY fire_at LIMIT 1`+
			s.dialect.ClaimLock()),
			string(StatePending), string(StateLeased), now.UnixMilli(), now.UnixMilli()))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		if err != nil {
			return err
		}

		read := entry.Attempts

		applyClaim(entry, s.cfg.InstanceID, now, s.cfg.LeaseTTL)

		if err := s.save(tx, entry, read); err != nil {
			return err
		}

		claimed = entry

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claiming due schedule entry: %w", err)
	}

	return claimed, nil
}

// RenewLease extends the lease on an entry this instance still holds, and
// reports ErrLeaseLost once another instance has taken it over.
func (s *SQLStore) RenewLease(id string) (*Entry, error) {
	return s.updateHeld(id, renewEdit(s.cfg.LeaseTTL))
}

// Complete marks an entry this instance holds as successfully fired.
func (s *SQLStore) Complete(id string) error {
	_, err := s.updateHeld(id, completeEdit)

	return err
}

// Fail records a failed attempt: a retry while attempts remain, a dead letter
// once they are spent.
func (s *SQLStore) Fail(id string, reason string) (*Entry, error) {
	return s.updateHeld(id, failEdit(reason, s.cfg))
}

// MarkDead retires an entry that can never succeed.
func (s *SQLStore) MarkDead(id string, reason string) error {
	_, err := s.updateHeld(id, markDeadEdit(reason))

	return err
}

// Cancel removes a timer that has not been claimed yet. It is idempotent, but
// a cancel that loses the race with a claim reports ErrAlreadyClaimed.
func (s *SQLStore) Cancel(id string) error {
	if _, err := bson.ObjectIDFromHex(id); err != nil {
		return fmt.Errorf("cancelling schedule entry %q: %w", id, err)
	}

	result, err := s.db.ExecContext(s.ctx, s.dialect.Rebind(`DELETE FROM schedule WHERE id = ? AND state = ?`),
		id, string(StatePending))
	if err != nil {
		return fmt.Errorf("cancelling schedule entry %q: %w", id, err)
	}

	if deleted, err := result.RowsAffected(); err != nil || deleted > 0 {
		return err
	}

	// Nothing was deleted: either the entry is already gone (fine) or it is no
	// longer pending (the cancel lost the race).
	_, err = s.get(s.db, id, "")
	if errors.Is(err, ErrNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("checking cancelled schedule entry %q: %w", id, err)
	}

	return ErrAlreadyClaimed
}

// CancelForGame removes every timer still pending for one game. Entries
// already leased are left alone — they are mid-dispatch.
func (s *SQLStore) CancelForGame(gameID string) (int64, error) {
	if gameID == "" {
		return 0, fmt.Errorf("cancelling timers requires a game id")
	}

	result, err := s.db.ExecContext(s.ctx, s.dialect.Rebind(`DELETE FROM schedule WHERE game_id = ? AND state = ?`),
		gameID, string(StatePending))
	if err != nil {
		return 0, fmt.Errorf("cancelling timers for game %q: %w", gameID, err)
	}

	return result.RowsAffected()
}

// updateHeld applies edit to the entry, but only while this instance still
// holds its lease, and returns the result.
func (s *SQLStore) updateHeld(id string, edit heldEdit) (*Entry, error) {
	if _, err := bson.ObjectIDFromHex(id); err != nil {
		return nil, fmt.Errorf("parsing schedule entry id %q: %w", id, err)
	}

	var updated *Entry

	err := s.inTx(func(tx *sql.Tx) error {
		entry, err := s.get(tx, id, s.dialect.LockForUpdate())
		if errors.Is(err, ErrNotFound) {
			return ErrLeaseLost
		}

		if err != nil {
			return err
		}

		if !heldBy(entry, s.cfg.InstanceID) {
			return ErrLeaseLost
		}

		read := entry.Attempts
		now := mongoTime(time.Now())

		edit(entry, now)

		entry.UpdatedAt = now

		if err := s.save(tx, entry, read); err != nil {
			return err
		}

		updated = entry

		return nil
	})
	if errors.Is(err, ErrLeaseLost) {
		return nil, ErrLeaseLost
	}

	if err != nil {
		return nil, fmt.Errorf("updating schedule entry %q: %w", id, err)
	}

	return updated, nil
}

// save writes back the fields a claim or a lease-held edit can change.
//
// It is conditional on the attempt count read, which every claim moves on: the
// row lock already keeps a second writer out, and the condition is what turns a
// write that somehow raced one anyway into ErrLeaseLost instead of an
// overwrite.
func (s *SQLStore) save(tx *sql.Tx, e *Entry, attemptsRead int) error {
	var firedAt *int64
	if e.FiredAt != nil {
		ms := e.FiredAt.UnixMilli()
		firedAt = &ms
	}

	result, err := tx.ExecContext(s.ctx, s.dialect.Rebind(`UPDATE schedule SET
		state = ?, fire_at = ?, claimed_until = ?, claimed_by = ?, attempts = ?, last_error = ?,
		fired_at = ?, updated_at = ?
		WHERE id = ? AND attempts = ?`),
		string(e.State), e.FireAt.UnixMilli(), e.ClaimedUntil.UnixMilli(), e.ClaimedBy, e.Attempts, e.LastError,
		firedAt, e.UpdatedAt.UnixMilli(), e.ID.Hex(), attemptsRead)
	if err != nil {
		return err
	}

	written, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if written != 1 {
		return ErrLeaseLost
	}

	return nil
}

// inTx runs fn in a transaction, committing only if it succeeds.
func (s *SQLStore) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(s.ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

func scanEntry(row interface{ Scan(...any) error }) (*Entry, error) {
	var (
		e                                       Entry
		id, state                               string
		payload                                 []byte
		fireAt, claimedUntil, created, modified int64
		firedAt                                 sql.NullInt64
	)

	if err := row.Scan(&id, &e.GameID, &e.Action, &payload, &e.ScriptVersion, &e.IdempotencyKey, &state,
		&fireAt, &claimedUntil, &e.ClaimedBy, &e.Attempts, &e.LastError, &firedAt, &created, &modified); err != nil {
		return nil, err
	}

	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("decoding schedule entry id %q: %w", id, err)
	}

	if e.Payload, err = decodePayload(payload); err != nil {
		return nil, fmt.Errorf("decoding the payload of schedule entry %q: %w", id, err)
	}

	e.ID = objectID
	e.State = State(state)
	e.FireAt = fromMillis(fireAt)
	e.ClaimedUntil = fromMillis(claimedUntil)
	e.CreatedAt = fromMillis(created)
	e.UpdatedAt = fromMillis(modified)

	if firedAt.Valid {
		fired := fromMillis(firedAt.Int64)
		e.FiredAt = &fired
	}

	return &e, nil
}

// fromMillis reads back a time stored as Unix milliseconds. The zero time
// round-trips to itself, which is what "never claimed" is stored as.
func fromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// encodePayload stores a payload as BSON, and no payload as NULL.
func encodePayload(payload map[string]interface{}) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}

	return bson.Marshal(payload)
}

func decodePayload(raw []byte) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var payload map[string]interface{}
	if err := bson.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	return payload, nil
}

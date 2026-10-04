package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/sqldialect"
)

// sessionsSchema is the PostgreSQL store's own DDL. SQLite's lives with the
// rest of its schema in internal/clients/sqlite, in the same shape: created_at
// is Unix nanoseconds on both, so one set of statements serves both.
const sessionsSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT   PRIMARY KEY,
    token      TEXT   UNIQUE,
    user_id    TEXT   UNIQUE,
    game_id    TEXT,
    data       JSONB  NOT NULL,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions (created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_game_id    ON sessions (game_id);
`

// SQLStore is a session store over SQLite or PostgreSQL; the two differ only in
// their dialect. The session is stored as JSON with id, token, user, game and
// creation time lifted into columns: the token and user carry the unique
// indexes, the game is what every broadcast looks sessions up by, and
// created_at is what expiry filters on. Token needs its column
// for another reason too — models.Session.Token is json:"-", so the JSON form
// never carries it.
type SQLStore struct {
	ctx     context.Context
	db      *sql.DB
	dialect sqldialect.Dialect
}

// NewSQLiteStore builds a session store over an already-open SQLite database,
// whose schema internal/clients/sqlite.Open created. The caller owns db.
func NewSQLiteStore(ctx context.Context, db *sql.DB) *SQLStore {
	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.SQLite}
}

// NewPostgresStore builds a session store over an already-open PostgreSQL pool
// and creates its table. The caller owns db.
func NewPostgresStore(ctx context.Context, db *sql.DB) (*SQLStore, error) {
	if _, err := db.ExecContext(ctx, sessionsSchema); err != nil {
		return nil, fmt.Errorf("creating the sessions table: %w", err)
	}

	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.Postgres}, nil
}

// selectLive is every read: the JSON document and the token beside it, from
// sessions that have not expired. Its one argument is the expiry cutoff.
const selectLive = `SELECT data, token FROM sessions WHERE created_at > ?`

func scanSession(row interface{ Scan(...any) error }) (*models.Session, error) {
	var (
		data  []byte
		token sql.NullString
	)

	if err := row.Scan(&data, &token); err != nil {
		return nil, err
	}

	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("decoding session: %w", err)
	}

	sess.Token = token.String

	return &sess, nil
}

// querier is what a read or write runs on: the pool, or a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// getOne reads the one live session matching where ("column = ?") on q; lock
// is appended to the SELECT.
func (s *SQLStore) getOne(q querier, where string, value any, what string, lock string) (*models.Session, error) {
	sess, err := scanSession(q.QueryRowContext(s.ctx,
		s.dialect.Rebind(selectLive+` AND `+where+lock), expiryCutoff().UnixNano(), value))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s: %w", what, repoErrors.ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", what, err)
	}

	return sess, nil
}

// New returns the user's session if they have one, or creates it. Expired
// sessions are deleted first, so one can neither be handed back nor block the
// new session through the unique index on the user.
func (s *SQLStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errNoUser
	}

	if _, err := s.db.ExecContext(s.ctx, s.dialect.Rebind(`DELETE FROM sessions WHERE created_at <= ?`),
		expiryCutoff().UnixNano()); err != nil {
		return nil, fmt.Errorf("purging expired sessions: %w", err)
	}

	existing, err := s.FindFirst("userId", c.UserID)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, repoErrors.ErrNotFound) {
		return nil, err
	}

	sess := newSession(c)

	data, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("encoding session %q: %w", sess.ID, err)
	}

	_, err = s.db.ExecContext(s.ctx,
		s.dialect.Rebind(`INSERT INTO sessions (id, token, user_id, game_id, data, created_at) VALUES (?, ?, ?, ?, ?, ?)`),
		sess.ID, ptrOrNil(sess.Token), c.UserID, sess.GameID, string(data), sess.CreatedAt.UnixNano())
	if s.dialect.IsUniqueViolation(err) {
		// Lost a race with a concurrent login for the same user: one session
		// per user, so the winner's is the answer.
		return s.FindFirst("userId", c.UserID)
	}

	if err != nil {
		return nil, fmt.Errorf("inserting session %q: %w", sess.ID, err)
	}

	return sess, nil
}

func (s *SQLStore) Get(id string) (*models.Session, error) {
	return s.getOne(s.db, `id = ?`, id, fmt.Sprintf("session %q", id), "")
}

func (s *SQLStore) GetByToken(token string) (*models.Session, error) {
	if err := requireToken(token); err != nil {
		return nil, err
	}

	return s.getOne(s.db, `token = ?`, token, "session for token", "")
}

func (s *SQLStore) Exists(id string) (bool, error) {
	_, err := s.Get(id)
	if errors.Is(err, repoErrors.ErrNotFound) {
		return false, nil
	}

	return err == nil, err
}

// columns are the lookups with a column of their own, filtered in SQL. The
// rest are filtered in Go.
var columns = map[string]string{"userId": "user_id", "gameId": "game_id"}

func (s *SQLStore) Find(key string, value string) ([]*models.Session, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	query, args := selectLive, []any{expiryCutoff().UnixNano()}
	if column, ok := columns[key]; ok {
		query, args = selectLive+` AND `+column+` = ?`, append(args, value)
	}

	rows, err := s.db.QueryContext(s.ctx, s.dialect.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("finding sessions by %s: %w", key, err)
	}
	defer rows.Close()

	var found []*models.Session

	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("finding sessions by %s: %w", key, err)
		}

		if matches(sess, key, value) {
			found = append(found, sess)
		}
	}

	return found, rows.Err()
}

func (s *SQLStore) FindFirst(key string, value string) (*models.Session, error) {
	found, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}

	return first(found, key, value)
}

// Update applies the update's non-empty fields. Moving the session onto a user
// who has one is repo.ErrDuplicate.
func (s *SQLStore) Update(id string, u *models.UpdateSession) error {
	err := s.modify(id, func(sess *models.Session) bool {
		applyUpdate(sess, u)

		return true
	})
	if errors.Is(err, errUserTaken) {
		return fmt.Errorf("user %q already has a session: %w", u.UserID, repoErrors.ErrDuplicate)
	}

	return err
}

// LeaveGame takes the session out of gameId, if it is still in it.
func (s *SQLStore) LeaveGame(id string, gameId string) error {
	err := s.modify(id, func(sess *models.Session) bool {
		if sess.GameID == nil || *sess.GameID != gameId {
			return false
		}

		leaveGame(sess)

		return true
	})
	if errors.Is(err, repoErrors.ErrNotFound) {
		return nil
	}

	return err
}

// errUserTaken is modify's report that the unique index on the user refused
// the write.
var errUserTaken = errors.New("user already has a session")

// modify reads the session, lets edit change it, and writes it back if edit
// says it changed — in one transaction with the row locked, so two edits to
// different fields cannot each write back a stale copy of the other's.
func (s *SQLStore) modify(id string, edit func(*models.Session) (changed bool)) error {
	tx, err := s.db.BeginTx(s.ctx, nil)
	if err != nil {
		return fmt.Errorf("updating session %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	sess, err := s.getOne(tx, `id = ?`, id, fmt.Sprintf("session %q", id), s.dialect.LockForUpdate())
	if err != nil {
		return err
	}

	if !edit(sess) {
		return nil
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("encoding session %q: %w", id, err)
	}

	_, err = tx.ExecContext(s.ctx,
		s.dialect.Rebind(`UPDATE sessions SET user_id = ?, game_id = ?, data = ? WHERE id = ?`),
		sess.UserID, sess.GameID, string(data), id)
	if s.dialect.IsUniqueViolation(err) {
		return errUserTaken
	}

	if err != nil {
		return fmt.Errorf("updating session %q: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("updating session %q: %w", id, err)
	}

	return nil
}

// Delete removes a session, invalidating its token. Deleting one that is
// already gone is not an error.
func (s *SQLStore) Delete(id string) error {
	if _, err := s.db.ExecContext(s.ctx, s.dialect.Rebind(`DELETE FROM sessions WHERE id = ?`), id); err != nil {
		return fmt.Errorf("deleting session %q: %w", id, err)
	}

	return nil
}

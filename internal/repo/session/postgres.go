package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

const sessionSchemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_token   ON sessions (token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
`

// PostgresStore is a PostgreSQL-backed session.Storer. Each session is stored
// as a JSONB blob alongside indexed scalar columns for efficient lookups.
// token is kept in its own column because models.Session.Token is tagged
// json:"-" and would otherwise be lost on every read/write round-trip.
type PostgresStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if _, err := db.ExecContext(ctx, sessionSchemaSQL); err != nil {
		return nil, fmt.Errorf("running session schema DDL: %w", err)
	}
	return &PostgresStore{ctx: ctx, db: db}, nil
}

// isPgDuplicateKey reports whether err is a PostgreSQL unique-constraint violation.
// pgx wraps these as *pgconn.PgError with code "23505".
func isPgDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

// New enforces one-session-per-userId: a second call for the same UserID
// returns the existing session, matching MemoryStore/MongoStore behaviour.
// ptrOrNil is defined in memory.go (same package).
func (s *PostgresStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errors.New("session must have a user ID")
	}

	// Return existing session for this user if one exists.
	existing, err := s.FindFirst("userId", c.UserID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repoErrors.ErrNotFound) {
		return nil, err
	}

	sess := &models.Session{
		ID:        ids.New(),
		Token:     c.Token,
		UserID:    ptrOrNil(c.UserID),
		GameID:    ptrOrNil(c.GameID),
		TeamID:    ptrOrNil(c.TeamID),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("marshaling session: %w", err)
	}
	// Store an empty token as NULL (not "") so the UNIQUE index on token
	// doesn't collide across sessions without one, mirroring MemoryStore
	// (which never indexes an empty token in byToken).
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO sessions (id, token, user_id, data) VALUES ($1, $2, $3, $4)`,
		sess.ID, ptrOrNil(c.Token), c.UserID, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			// Race: another writer created the session between our FindFirst
			// check and now. Re-read and return it.
			return s.FindFirst("userId", c.UserID)
		}
		return nil, fmt.Errorf("inserting session: %w", err)
	}
	return s.Get(sess.ID)
}

func (s *PostgresStore) Get(id string) (*models.Session, error) {
	var (
		data  []byte
		token sql.NullString
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, token FROM sessions WHERE id = $1`, id,
	).Scan(&data, &token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}
	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	// Restore token from its dedicated column — json:"-" means the blob never has it.
	if token.Valid {
		sess.Token = token.String
	}
	return &sess, nil
}

func (s *PostgresStore) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}
	var (
		data     []byte
		tokenCol sql.NullString
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, token FROM sessions WHERE token = $1`, token,
	).Scan(&data, &tokenCol)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("token: %w", repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying session by token: %w", err)
	}
	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	if tokenCol.Valid {
		sess.Token = tokenCol.String
	}
	return &sess, nil
}

func (s *PostgresStore) Exists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(s.ctx,
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *PostgresStore) Find(key string, value string) ([]*models.Session, error) {
	var col string
	switch key {
	case "token":
		col = "token"
	case "userId":
		col = "user_id"
	default:
		return s.findInGo(key, value)
	}
	rows, err := s.db.QueryContext(s.ctx,
		fmt.Sprintf(`SELECT data, token FROM sessions WHERE %s = $1`, col), value,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *PostgresStore) findInGo(key, value string) ([]*models.Session, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data, token FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	var out []*models.Session
	for _, sess := range all {
		if matchSessionField(sess, key, value) {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *PostgresStore) FindFirst(key string, value string) (*models.Session, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *PostgresStore) Update(id string, u *models.UpdateSession) error {
	if u == nil {
		return errors.New("UpdateSession is nil")
	}
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	if u.GameID != "" {
		sess.GameID = ptrOrNil(u.GameID)
	}
	if u.UserID != "" {
		sess.UserID = ptrOrNil(u.UserID)
	}
	if u.TeamID != "" {
		sess.TeamID = ptrOrNil(u.TeamID)
	}
	sess.UpdatedAt = time.Now()

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}
	userID := ""
	if sess.UserID != nil {
		userID = *sess.UserID
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE sessions SET user_id = $2, data = $3 WHERE id = $1`,
		sess.ID, userID, data,
	)
	return err
}

func (s *PostgresStore) Delete(id string) error {
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func scanSessions(rows *sql.Rows) ([]*models.Session, error) {
	var out []*models.Session
	for rows.Next() {
		var (
			data  []byte
			token sql.NullString
		)
		if err := rows.Scan(&data, &token); err != nil {
			return nil, err
		}
		var sess models.Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, err
		}
		if token.Valid {
			sess.Token = token.String
		}
		out = append(out, &sess)
	}
	return out, rows.Err()
}

// matchSessionField is defined in memory.go (same package) — do NOT redeclare here.

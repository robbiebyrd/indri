package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

// SQLiteStore is a SQLite-backed session.Storer. Each row holds the full JSON
// blob of a models.Session plus indexed scalar columns for token and user_id.
type SQLiteStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*SQLiteStore)(nil)

func NewSQLiteStore(ctx context.Context, db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	return &SQLiteStore{ctx: ctx, db: db}, nil
}

// marshalSession serializes a session to the JSON blob stored in the data column.
// models.Session.Token is tagged `json:"-"`, so it is preserved in a wrapper
// struct field _token to survive the round-trip.
func marshalSession(sess *models.Session) (string, error) {
	type sessionBlob struct {
		models.Session
		BlobToken string `json:"_token"`
	}
	b, err := json.Marshal(sessionBlob{Session: *sess, BlobToken: sess.Token})
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}
	return string(b), nil
}

// unmarshalSession deserializes the JSON blob and restores Token from _token.
func unmarshalSession(data string) (*models.Session, error) {
	type sessionBlob struct {
		models.Session
		BlobToken string `json:"_token"`
	}
	var blob sessionBlob
	if err := json.Unmarshal([]byte(data), &blob); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	blob.Session.Token = blob.BlobToken
	return &blob.Session, nil
}

// New enforces one-session-per-user: if a session already exists for the
// given UserID, the existing session is returned unchanged.
// ptrOrNil is defined in memory.go (same package).
func (s *SQLiteStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errors.New("session must have a user id")
	}

	// SELECT-then-INSERT guard enforces the one-per-user invariant.
	// SQLite treats each NULL as distinct in a UNIQUE column, so the UNIQUE
	// constraint alone does NOT prevent two sessions with an empty user_id.
	var existingData string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM sessions WHERE user_id = ?`, c.UserID,
	).Scan(&existingData)
	if err == nil {
		return unmarshalSession(existingData)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing session: %w", err)
	}

	sess := newSession(c)

	blob, err := marshalSession(sess)
	if err != nil {
		return nil, err
	}

	// Store empty UserID as NULL so each NULL is distinct and the UNIQUE
	// index does not treat two empty-string rows as colliding.
	userIDVal := sql.NullString{String: c.UserID, Valid: c.UserID != ""}
	tokenVal := sql.NullString{String: c.Token, Valid: c.Token != ""}

	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO sessions (id, token, user_id, data) VALUES (?, ?, ?, ?)`,
		sess.ID, tokenVal, userIDVal, blob,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return sess, nil
}

func (s *SQLiteStore) Get(id string) (*models.Session, error) {
	var data string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM sessions WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return unmarshalSession(data)
}

func (s *SQLiteStore) GetByToken(token string) (*models.Session, error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}
	var data string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM sessions WHERE token = ?`, token,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("token: %w", repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session by token: %w", err)
	}
	return unmarshalSession(data)
}

func (s *SQLiteStore) Exists(id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM sessions WHERE id = ?`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists session: %w", err)
	}
	return count > 0, nil
}

// Find scans all session rows and returns those matching the given key/value.
// matchSessionField is defined in memory.go (same package).
func (s *SQLiteStore) Find(key string, value string) ([]*models.Session, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("find sessions: %w", err)
	}
	defer rows.Close()

	var out []*models.Session
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess, err := unmarshalSession(data)
		if err != nil {
			return nil, err
		}
		if matchSessionField(sess, key, value) {
			out = append(out, sess)
		}
	}
	return out, rows.Err()
}

func (s *SQLiteStore) FindFirst(key string, value string) (*models.Session, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *SQLiteStore) Update(id string, u *models.UpdateSession) error {
	if u == nil {
		return errors.New("UpdateSession is nil")
	}
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	applyUpdate(sess, u)

	blob, err := marshalSession(sess)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE sessions SET data = ? WHERE id = ?`,
		blob, id,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Delete(id string) error {
	_, err := s.db.ExecContext(s.ctx,
		`DELETE FROM sessions WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil // idempotent — DELETE of non-existent row is not an error
}

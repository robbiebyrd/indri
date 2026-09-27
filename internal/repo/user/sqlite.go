package user

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

// SQLiteStore is a SQLite-backed user.Storer. Each user is stored as a JSON
// blob in the data column. Password is kept in a separate column because
// models.User.Password is tagged json:"-" and would otherwise be lost on
// every read/write round-trip through the blob.
type SQLiteStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*SQLiteStore)(nil)

// NewSQLiteStore creates a SQLiteStore using an already-open *sql.DB.
// The caller is responsible for opening the DB via sqlite.Open, which runs the DDL.
func NewSQLiteStore(ctx context.Context, db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	return &SQLiteStore{ctx: ctx, db: db}, nil
}

// marshalUser serializes a user to the JSON blob stored in the data column.
// Password is excluded because it has json:"-"; it is stored in a separate column.
func marshalUser(u *models.User) (string, error) {
	b, err := json.Marshal(u)
	if err != nil {
		return "", fmt.Errorf("marshal user: %w", err)
	}
	return string(b), nil
}

// unmarshalUser deserializes the JSON blob from the data column.
func unmarshalUser(data string) (*models.User, error) {
	var u models.User
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return nil, fmt.Errorf("unmarshal user: %w", err)
	}
	return &u, nil
}

// New creates a new user row. Returns ErrDuplicate if the email is taken.
func (s *SQLiteStore) New(c models.CreateUser) (*models.User, error) {
	now := time.Now()
	u := &models.User{
		ID:          ids.New(),
		Email:       c.Email,
		Name:        c.Name,
		DisplayName: c.DisplayName,
		Password:    c.Password,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	blob, err := marshalUser(u)
	if err != nil {
		return nil, err
	}
	// Store password in its own column because json:"-" strips it from the blob.
	var pwVal *string
	if u.Password != nil {
		pwVal = u.Password
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO users (id, email, password, data) VALUES (?, ?, ?, ?)`,
		u.ID, u.Email, pwVal, blob,
	)
	if err != nil {
		if isSQLiteConstraintUnique(err) {
			return nil, fmt.Errorf("email %q: %w", c.Email, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// Get retrieves a user by ID. Returns ErrNotFound if no row exists.
func (s *SQLiteStore) Get(id string) (*models.User, error) {
	var (
		data string
		pw   sql.NullString
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, password FROM users WHERE id = ?`, id,
	).Scan(&data, &pw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u, err := unmarshalUser(data)
	if err != nil {
		return nil, err
	}
	// Restore password from its dedicated column — json:"-" means the blob never has it.
	if pw.Valid {
		u.Password = &pw.String
	}
	return u, nil
}

// Exists reports whether a user with the given ID exists.
func (s *SQLiteStore) Exists(id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM users WHERE id = ?`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists user: %w", err)
	}
	return count > 0, nil
}

// Find returns all users whose field key equals value. Supported keys: "email", "name".
func (s *SQLiteStore) Find(key string, value string) ([]*models.User, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data, password FROM users`)
	if err != nil {
		return nil, fmt.Errorf("find users: %w", err)
	}
	defer rows.Close()

	var out []*models.User
	for rows.Next() {
		var (
			data string
			pw   sql.NullString
		)
		if err := rows.Scan(&data, &pw); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u, err := unmarshalUser(data)
		if err != nil {
			return nil, err
		}
		if pw.Valid {
			u.Password = &pw.String
		}
		if matchUserField(u, key, value) {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

// FindFirst returns the first user matching key=value, or ErrNotFound.
func (s *SQLiteStore) FindFirst(key string, value string) (*models.User, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no user with %s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

// Update applies non-zero fields from u to the stored user. Returns ErrNotFound
// if the ID does not exist, ErrDuplicate if the new email is already taken.
func (s *SQLiteStore) Update(u *models.UpdateUser) error {
	if u == nil || u.ID == "" {
		return errors.New("UpdateUser.ID is required")
	}
	existing, err := s.Get(u.ID)
	if err != nil {
		return err
	}
	if u.Email != "" {
		existing.Email = u.Email
	}
	if u.Name != "" {
		existing.Name = u.Name
	}
	if u.DisplayName != nil {
		existing.DisplayName = u.DisplayName
	}
	if u.Password != nil {
		existing.Password = u.Password
	}
	existing.UpdatedAt = time.Now()

	blob, err := marshalUser(existing)
	if err != nil {
		return err
	}
	var pwVal *string
	if existing.Password != nil {
		pwVal = existing.Password
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE users SET email = ?, password = ?, data = ? WHERE id = ?`,
		existing.Email, pwVal, blob, u.ID,
	)
	if err != nil {
		if isSQLiteConstraintUnique(err) {
			return fmt.Errorf("email %q: %w", u.Email, repoErrors.ErrDuplicate)
		}
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// isSQLiteConstraintUnique detects a UNIQUE constraint violation from modernc.org/sqlite.
// The user package defines its own copy because this is a different package from game/.
func isSQLiteConstraintUnique(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// matchUserField is defined in memory.go (same package) — do NOT redeclare here.

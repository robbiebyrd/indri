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

// password is a nullable column separate from the JSONB blob because
// models.User.Password is tagged json:"-" and would otherwise be lost on
// every read/write round-trip (mirroring how the SQLite store handles it).
const userSchemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id       TEXT PRIMARY KEY,
    email    TEXT UNIQUE NOT NULL,
    password TEXT,
    data     JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
`

// PostgresStore is a PostgreSQL-backed user.Storer. Each user is stored as a
// JSONB blob alongside indexed scalar columns for efficient lookups.
type PostgresStore struct {
	ctx context.Context
	db  *sql.DB
}

var _ Storer = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if _, err := db.ExecContext(ctx, userSchemaSQL); err != nil {
		return nil, fmt.Errorf("running user schema DDL: %w", err)
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

func (s *PostgresStore) New(c models.CreateUser) (*models.User, error) {
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
	data, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("marshaling user: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO users (id, email, password, data) VALUES ($1, $2, $3, $4)`,
		u.ID, u.Email, u.Password, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			return nil, fmt.Errorf("email %q: %w", c.Email, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return s.Get(u.ID)
}

func (s *PostgresStore) Get(id string) (*models.User, error) {
	var (
		data []byte
		pw   sql.NullString
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, password FROM users WHERE id = $1`, id,
	).Scan(&data, &pw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	var u models.User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	// Restore password from its dedicated column — json:"-" means the blob never has it.
	if pw.Valid {
		u.Password = &pw.String
	}
	return &u, nil
}

func (s *PostgresStore) Exists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(s.ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *PostgresStore) Find(key string, value string) ([]*models.User, error) {
	var col string
	switch key {
	case "email":
		col = "email"
	default:
		// Generic fallback: scan all and filter in Go. Only email is indexed.
		return s.findInGo(key, value)
	}
	rows, err := s.db.QueryContext(s.ctx,
		fmt.Sprintf(`SELECT data, password FROM users WHERE %s = $1`, col), value,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *PostgresStore) findInGo(key, value string) ([]*models.User, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data, password FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanUsers(rows)
	if err != nil {
		return nil, err
	}
	var out []*models.User
	for _, u := range all {
		if matchUserField(u, key, value) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (s *PostgresStore) FindFirst(key string, value string) (*models.User, error) {
	list, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no user with %s=%q: %w", key, value, repoErrors.ErrNotFound)
	}
	return list[0], nil
}

func (s *PostgresStore) Update(u *models.UpdateUser) error {
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

	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling user: %w", err)
	}
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE users SET email = $2, password = $3, data = $4 WHERE id = $1`,
		existing.ID, existing.Email, existing.Password, data,
	)
	if err != nil {
		if isPgDuplicateKey(err) {
			return fmt.Errorf("email %q: %w", existing.Email, repoErrors.ErrDuplicate)
		}
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

func scanUsers(rows *sql.Rows) ([]*models.User, error) {
	var out []*models.User
	for rows.Next() {
		var (
			data []byte
			pw   sql.NullString
		)
		if err := rows.Scan(&data, &pw); err != nil {
			return nil, err
		}
		var u models.User
		if err := json.Unmarshal(data, &u); err != nil {
			return nil, err
		}
		if pw.Valid {
			u.Password = &pw.String
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

// matchUserField is defined in memory.go (same package) — do NOT redeclare here.

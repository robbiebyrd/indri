package user

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

// usersSchema is the PostgreSQL store's own DDL. SQLite's lives with the rest of
// its schema in internal/clients/sqlite.
const usersSchema = `
CREATE TABLE IF NOT EXISTS users (
    id       TEXT PRIMARY KEY,
    email    TEXT UNIQUE NOT NULL,
    password TEXT,
    data     JSONB NOT NULL
);
`

// SQLStore is a user store over SQLite or PostgreSQL; the two differ only in
// their dialect. The user is stored as JSON, with id and email — the only
// things a query filters on, and the unique index — lifted into columns.
// Password has a column of its own because models.User.Password is json:"-":
// the JSON form never carries it.
type SQLStore struct {
	ctx     context.Context
	db      *sql.DB
	dialect sqldialect.Dialect
}

// NewSQLiteStore builds a user store over an already-open SQLite database,
// whose schema internal/clients/sqlite.Open created. The caller owns db.
func NewSQLiteStore(ctx context.Context, db *sql.DB) *SQLStore {
	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.SQLite}
}

// NewPostgresStore builds a user store over an already-open PostgreSQL pool and
// creates its table. The caller owns db.
func NewPostgresStore(ctx context.Context, db *sql.DB) (*SQLStore, error) {
	if _, err := db.ExecContext(ctx, usersSchema); err != nil {
		return nil, fmt.Errorf("creating the users table: %w", err)
	}

	return &SQLStore{ctx: ctx, db: db, dialect: sqldialect.Postgres}, nil
}

// querier is what a read or write runs on: the pool, or a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// write runs a statement whose first three arguments are u's email, password
// and JSON form, mapping a taken email onto repo.ErrDuplicate.
func (s *SQLStore) write(q querier, statement string, u *models.User, args ...any) (sql.Result, error) {
	data, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("encoding user %q: %w", u.ID, err)
	}

	res, err := q.ExecContext(s.ctx, s.dialect.Rebind(statement),
		append([]any{u.Email, u.Password, string(data)}, args...)...)
	if s.dialect.IsUniqueViolation(err) {
		return nil, fmt.Errorf("a user with email %q: %w", u.Email, repoErrors.ErrDuplicate)
	}

	if err != nil {
		return nil, fmt.Errorf("writing user %q: %w", u.ID, err)
	}

	return res, nil
}

func (s *SQLStore) New(c models.CreateUser) (*models.User, error) {
	u := newUser(c)

	if _, err := s.write(s.db, `INSERT INTO users (email, password, data, id) VALUES (?, ?, ?, ?)`, u, u.ID); err != nil {
		return nil, err
	}

	return u, nil
}

// selectUsers is every read: the JSON document plus the password beside it.
const selectUsers = `SELECT data, password FROM users`

func scanUser(row interface{ Scan(...any) error }) (*models.User, error) {
	var (
		data     []byte
		password sql.NullString
	)

	if err := row.Scan(&data, &password); err != nil {
		return nil, err
	}

	var u models.User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}

	if password.Valid {
		u.Password = &password.String
	}

	return &u, nil
}

func (s *SQLStore) Get(id string) (*models.User, error) {
	return s.get(s.db, id, "")
}

// get reads one user on q; lock is appended to the SELECT.
func (s *SQLStore) get(q querier, id string, lock string) (*models.User, error) {
	u, err := scanUser(q.QueryRowContext(s.ctx, s.dialect.Rebind(selectUsers+` WHERE id = ?`+lock), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user %q: %w", id, repoErrors.ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("fetching user %q: %w", id, err)
	}

	return u, nil
}

func (s *SQLStore) Exists(id string) (bool, error) {
	_, err := s.Get(id)
	if errors.Is(err, repoErrors.ErrNotFound) {
		return false, nil
	}

	return err == nil, err
}

// Find filters on the email column when it can and in Go otherwise: email is
// the one lookup a server makes on every login.
func (s *SQLStore) Find(key string, value string) ([]*models.User, error) {
	if err := requireLookup(key); err != nil {
		return nil, err
	}

	query, args := selectUsers, []any{}
	if key == "email" {
		query, args = selectUsers+` WHERE email = ?`, []any{value}
	}

	rows, err := s.db.QueryContext(s.ctx, s.dialect.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("finding users by %s: %w", key, err)
	}
	defer rows.Close()

	var found []*models.User

	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("finding users by %s: %w", key, err)
		}

		if matches(u, key, value) {
			found = append(found, u)
		}
	}

	return found, rows.Err()
}

func (s *SQLStore) FindFirst(key string, value string) (*models.User, error) {
	found, err := s.Find(key, value)
	if err != nil {
		return nil, err
	}

	return first(found, key, value)
}

// Update reads the user, applies the update's fields and writes it back, in one
// transaction with the row locked: two updates to different fields would
// otherwise each write back a stale copy of the other's.
func (s *SQLStore) Update(update *models.UpdateUser) error {
	if err := requireID(update); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(s.ctx, nil)
	if err != nil {
		return fmt.Errorf("updating user %q: %w", update.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	u, err := s.get(tx, update.ID, s.dialect.LockForUpdate())
	if err != nil {
		return err
	}

	applyUpdate(u, update)

	if _, err := s.write(tx, `UPDATE users SET email = ?, password = ?, data = ? WHERE id = ?`, u, u.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("updating user %q: %w", update.ID, err)
	}

	return nil
}

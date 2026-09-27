# Pluggable DB Backends — SQLite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a SQLite-backed implementation of every `Storer` interface (`game`, `user`, `session`) and wire it into the backend switch so `"dbBackend": "sqlite"` in `server.json` boots the server with no external infrastructure.

**Architecture:** Three new `SQLiteStore` structs — one per repo package — each backed by a single shared `*sql.DB`. All three share one SQLite database file (separate tables). A new client helper (`internal/clients/sqlite/`) opens the file, enables WAL mode, and runs schema migrations; the game store is the primary owner and the user/session stores receive the same `*sql.DB`. The write path mirrors MemoryStore: read the JSON blob, apply the mutation in Go, write back with a version fence.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (pure-Go, no CGO), stdlib `database/sql` + `encoding/json`, existing `lock.Manager`, `events.Publisher`, `mutation.Run` primitives.

**Design decisions:**

1. **SQL driver — `modernc.org/sqlite` (pure Go).** CGO (`mattn/go-sqlite3`) requires a C toolchain, which breaks `GOARCH` cross-compilation and adds complexity to CI. Pure Go (`modernc.org/sqlite`) compiles cleanly everywhere the Go toolchain runs. The performance difference is negligible for Indri's write-oriented, single-game-at-a-time workload. `modernc.org/sqlite` is registered as `"sqlite"` using the standard `database/sql` driver interface.

2. **Schema — JSON-blob-with-indexed-columns.** Each table stores one JSON blob per row plus a handful of indexed scalar columns needed for queries (`code`, `version`, `private` on games; `email` on users; `token`/`userId` on sessions). This mirrors the document-store model exactly, satisfies every `Storer` method without a relational schema explosion, and keeps the MemoryStore and SQLiteStore implementations structurally parallel. Fully-normalized tables would require 8+ tables and make the `Mutate` CAS loop far more complex with no benefit at Indri's scale.

3. **JSON encoding — stdlib `encoding/json`, stored as TEXT.** TEXT is human-readable in the DB browser, debuggable with `sqlite3` CLI, and imposes zero encoding overhead vs BLOB. SQLite stores both identically on disk for ASCII/UTF-8 content.

4. **Mutate — WAL + version-fenced UPDATE.** `PRAGMA journal_mode=WAL` is set on open for better concurrent read throughput. The CAS loop is: `SELECT data, version FROM games WHERE id = ?` → unmarshal → `apply` → `UPDATE games SET data = ?, version = ? WHERE id = ? AND version = ?`. If `RowsAffected == 0`, the `save` callback returns `(false, nil)` and `mutation.Run` retries.

5. **UpdateField / DeleteField — full read-modify-write.** SQLite's `json_set` / `json_remove` are available but require careful handling of dotted paths with dynamic segment counts. MemoryStore already implements `applyDottedPath` + `fromMap` for the same purpose; SQLiteStore reuses the same functions (they live in `memory.go` in the same package) rather than duplicating SQL-level JSON surgery. The blob is read, the path is mutated in Go, and the blob is written back with an incremented version.

6. **Test databases — `:memory:` DSN.** All unit tests use `dsn = ":memory:"`. This avoids filesystem cleanup, runs in parallel safely, and is the idiomatic SQLite test practice. Production filesystem tests guard on the `INDRI_SQLITE_PATH` env var being set to a real path.

7. **`sqlitePath` env var** — `INDRI_SQLITE_PATH`, JSON key `sqlitePath`, default `./indri.db`. Added to `env.Vars` and `example/server.json`.

**Spec:** `docs/superpowers/specs/2026-09-27-pluggable-database-backends-design.md`

**Prerequisite:** Plan 1 (foundation + in-memory backend) must be merged. The following already exist and must not be re-created: `internal/repo/errors.go`, `internal/repo/ids/ids.go`, `internal/repo/game/memory.go` (+ sub-files), `internal/repo/user/memory.go`, `internal/repo/session/memory.go`, `internal/injector/repos.go` with `"memory"` case.

**Ground truth for types.** Before writing SQLiteStore code, an executor MUST read these files. The types listed here are the reference — the plan snippets follow them exactly. If the plan ever contradicts these, the file wins.

- `internal/models/game.go` — `Game{ID string, Version int64, Code string, Teams map[string]Team, Players map[string]Player, Stage, PublicData, PrivateData, PlayerData map[string]interface{}, Private bool, CreatedAt, UpdatedAt time.Time}`. No `PlayerData` on Team — Team has `PlayerData map[string]map[string]interface{}`.
- `internal/models/player.go` — `Player{Name, Score, Connected, Host, Controller, PublicData *map[string]interface{}, PrivateData *map[string]interface{}}`. No `UserID` or `DisplayName` on Player.
- `internal/models/team.go` — `Team{Name, PlayerIDs []string, PublicData, PrivateData map[string]interface{}, PlayerData map[string]map[string]interface{}}`. No `ID` on Team; team membership is a **slice** of user IDs.
- `internal/models/user.go` — `User{ID string, Email, Name string, DisplayName *string, Password *string, Score *int, PublicData, PrivateData map[string]interface{}, CreatedAt, UpdatedAt time.Time}`. `CreateUser{Email, Name string, DisplayName *string, Password *string}`. `UpdateUser{ID, Email, Name string, DisplayName *string, Password *string}`.
- `internal/models/session.go` — `Session{ID, Token string, GameID, UserID, TeamID *string, CreatedAt, UpdatedAt time.Time}`. `CreateSession{Token, GameID, UserID, TeamID string}`. `UpdateSession{GameID, UserID, TeamID string}`.
- `internal/services/events/events.go` — `ChangeEvent{ID, OperationType, Timestamp, Collection, UpdatedFields, RemovedFields}`; constants `OpUpdate`, `OpInsert`, `OpDelete`.
- `internal/services/mutation/mutation.go` — `Run[T](ctx, mgr, key, load func() (*T, int64, error), apply func(*T) error, save func(*T, int64) (bool, error)) error`.
- `internal/repo/game/interface.go` — 24-method `Storer` interface (read this file before coding to confirm the exact method signatures).
- `internal/repo/game/memory.go` — canonical reference for `applyDottedPath`, `splitPath`, `fromMap`, `publishFieldUpdate`, `publishDiff`, `collectionName` — all usable by SQLiteStore in the same package without re-declaration.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/clients/sqlite/client.go` | Create | Open `*sql.DB`, set pragmas, run DDL migrations |
| `internal/clients/sqlite/client_test.go` | Create | Verify open + schema |
| `internal/repo/game/sqlite.go` | Create | `SQLiteStore` struct, constructor, read ops, write ops |
| `internal/repo/game/sqlite_player.go` | Create | Player methods on `SQLiteStore` |
| `internal/repo/game/sqlite_team.go` | Create | Team methods on `SQLiteStore` |
| `internal/repo/game/sqlite_host.go` | Create | Host methods on `SQLiteStore` |
| `internal/repo/game/sqlite_test.go` | Create | Contract tests (`:memory:` DB) |
| `internal/repo/user/sqlite.go` | Create | `SQLiteStore` for users |
| `internal/repo/user/sqlite_test.go` | Create | Tests |
| `internal/repo/session/sqlite.go` | Create | `SQLiteStore` for sessions |
| `internal/repo/session/sqlite_test.go` | Create | Tests |
| `internal/repo/env/env.go` | Modify | Add `SQLitePath` field |
| `example/server.json` | Modify | Add `"sqlitePath"` flat key |
| `internal/injector/repos.go` | Modify | Add `"sqlite"` case |
| `go.mod`, `go.sum` | Modify | Add `modernc.org/sqlite` |

---

## Task 1: Add SQLite dependency and client helper

**Goal:** Pull in `modernc.org/sqlite`, create `internal/clients/sqlite/client.go` that opens a `*sql.DB`, sets performance pragmas, and runs the three-table DDL. The client is used by all three `SQLiteStore` constructors.

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/clients/sqlite/client.go`
- Create: `internal/clients/sqlite/client_test.go`

- [ ] **Step 1.1 — Add the dependency**

```bash
go get modernc.org/sqlite
```

Verify it appears in `go.mod`:
```bash
grep "modernc.org/sqlite" go.mod
```

- [ ] **Step 1.2 — Write the failing test**

Create `internal/clients/sqlite/client_test.go`:

```go
package sqlite_test

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
)

func TestOpen_CreatesTablesWithMemoryDSN(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	for _, tbl := range []string{"games", "users", "sessions"} {
		var name string
		row := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		)
		if err := row.Scan(&name); err != nil || name != tbl {
			t.Errorf("table %q not found: scan err=%v", tbl, err)
		}
	}
}
```

- [ ] **Step 1.3 — Run to confirm FAIL**

```bash
go test ./internal/clients/sqlite/
```

Expected: FAIL with `cannot find package` or `undefined: sqlite.Open`.

- [ ] **Step 1.4 — Implement**

Create `internal/clients/sqlite/client.go`:

```go
// Package sqlite opens a shared SQLite database used by all SQLiteStore
// implementations. It sets WAL journal mode for concurrent reader throughput
// and runs the schema DDL on first open.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS games (
    id      TEXT    PRIMARY KEY,
    code    TEXT    UNIQUE NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    private INTEGER NOT NULL DEFAULT 0,
    data    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id    TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    data  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id      TEXT PRIMARY KEY,
    token   TEXT UNIQUE,
    user_id TEXT UNIQUE,
    data    TEXT NOT NULL
);
`

// Open opens (or creates) the SQLite database at dsn, enables WAL mode,
// enforces foreign keys, and runs the schema DDL. Pass ":memory:" for tests.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", dsn, err)
	}

	// WAL gives concurrent readers without blocking writers.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma foreign_keys: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}

	return db, nil
}
```

- [ ] **Step 1.5 — Run to confirm PASS**

```bash
go test ./internal/clients/sqlite/
```

- [ ] **Step 1.6 — Build**

```bash
go build ./...
```

- [ ] **Step 1.7 — Commit**

```bash
git status
git add internal/clients/sqlite/ go.mod go.sum
git commit -m "feat(sqlite): add SQLite client helper with schema DDL"
```

---

## Task 2: `game.SQLiteStore` — struct, constructor, and read operations

The `SQLiteStore` stores one row per game. The `data` column is the full JSON blob of `models.Game`. Indexed scalar columns (`id`, `code`, `version`, `private`) support the query methods. Read operations select the blob and unmarshal it.

`Storer` has 24 methods; the compile-time assertion `var _ Storer = (*SQLiteStore)(nil)` will fail until all methods exist. Comment it out in this task and reinstate it in Task 6.

**Files:**
- Create: `internal/repo/game/sqlite.go`
- Create: `internal/repo/game/sqlite_test.go`

- [ ] **Step 2.1 — Write the failing constructor test**

Create `internal/repo/game/sqlite_test.go`:

```go
package game

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db, lock.NewInProcess(), events.NewInProcess())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestNewSQLiteStore_ReturnsUsableStore(t *testing.T) {
	store := newSQLiteFixture(t)
	if store == nil {
		t.Fatal("NewSQLiteStore returned nil without error")
	}
}
```

- [ ] **Step 2.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run TestNewSQLiteStore
```

Expected: FAIL with `undefined: SQLiteStore` or `undefined: NewSQLiteStore`.

- [ ] **Step 2.3 — Implement the skeleton**

Create `internal/repo/game/sqlite.go`:

```go
package game

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
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

// SQLiteStore is a SQLite-backed game.Storer. State is persisted in a single
// JSON blob per row. The write path mirrors MemoryStore: read-modify-write with
// a version fence in the UPDATE WHERE clause.
type SQLiteStore struct {
	ctx       context.Context
	db        *sql.DB
	locks     lock.Manager
	publisher events.Publisher
}

// TODO(sqlite): reinstate after all Storer methods land in Task 6.
// var _ Storer = (*SQLiteStore)(nil)

// NewSQLiteStore creates a SQLiteStore using an already-open *sql.DB. The caller
// is responsible for opening the DB via sqlite.Open, which runs the DDL.
func NewSQLiteStore(ctx context.Context, db *sql.DB, locks lock.Manager, publisher events.Publisher) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if locks == nil {
		return nil, errors.New("locks is required")
	}
	if publisher == nil {
		return nil, errors.New("publisher is required")
	}
	return &SQLiteStore{
		ctx:       ctx,
		db:        db,
		locks:     locks,
		publisher: publisher,
	}, nil
}

// marshalGame serializes a game to the JSON blob stored in the data column.
func marshalGame(g *models.Game) (string, error) {
	b, err := json.Marshal(g)
	if err != nil {
		return "", fmt.Errorf("marshal game: %w", err)
	}
	return string(b), nil
}

// unmarshalGame deserializes the JSON blob from the data column.
func unmarshalGame(data string) (*models.Game, error) {
	var g models.Game
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return nil, fmt.Errorf("unmarshal game: %w", err)
	}
	return &g, nil
}

func (s *SQLiteStore) New(code string, script *models.Script, privateGame bool) (*models.Game, error) {
	if script == nil {
		return nil, errors.New("script is required")
	}
	now := time.Now()
	teams := make(map[string]models.Team, len(script.Teams))
	for k, v := range script.Teams {
		teams[k] = v
	}
	g := &models.Game{
		ID:          ids.New(),
		Version:     1,
		Code:        code,
		Teams:       teams,
		Players:     map[string]models.Player{},
		Stage:       script.Stage,
		PublicData:  map[string]interface{}{},
		PrivateData: map[string]interface{}{},
		PlayerData:  map[string]interface{}{},
		Private:     privateGame,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if script.PublicData != nil {
		g.PublicData = script.PublicData
	}
	if script.PrivateData != nil {
		g.PrivateData = script.PrivateData
	}

	blob, err := marshalGame(g)
	if err != nil {
		return nil, err
	}

	private := 0
	if privateGame {
		private = 1
	}
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO games (id, code, version, private, data) VALUES (?, ?, ?, ?, ?)`,
		g.ID, code, g.Version, private, blob,
	)
	if err != nil {
		if isSQLiteConstraintUnique(err) {
			return nil, fmt.Errorf("game with code %q: %w", code, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("insert game: %w", err)
	}
	return g, nil
}

func (s *SQLiteStore) Get(id string) (*models.Game, error) {
	var data string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM games WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get game: %w", err)
	}
	return unmarshalGame(data)
}

func (s *SQLiteStore) FindByCode(gameCode string) (*models.Game, error) {
	var data string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM games WHERE code = ?`, gameCode,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("code %q: %w", gameCode, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find game by code: %w", err)
	}
	return unmarshalGame(data)
}

func (s *SQLiteStore) GetIDHex(gameCode string) (*string, error) {
	g, err := s.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

func (s *SQLiteStore) Exists(id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM games WHERE id = ?`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists game: %w", err)
	}
	return count > 0, nil
}

func (s *SQLiteStore) FindOpen(limit int) ([]*models.Game, error) {
	rows, err := s.db.QueryContext(s.ctx,
		`SELECT data FROM games WHERE private = 0 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find open games: %w", err)
	}
	defer rows.Close()

	var out []*models.Game
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan game: %w", err)
		}
		g, err := unmarshalGame(data)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// isSQLiteConstraintUnique reports whether err is a SQLite UNIQUE constraint
// violation. modernc.org/sqlite surfaces this as an error message containing
// "UNIQUE constraint failed".
func isSQLiteConstraintUnique(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

- [ ] **Step 2.4 — Run to confirm PASS**

```bash
go test ./internal/repo/game/ -run TestNewSQLiteStore
```

- [ ] **Step 2.5 — Write failing read tests**

Append to `internal/repo/game/sqlite_test.go`:

```go
func TestSQLiteStore_NewGame_AssignsIDAndCode(t *testing.T) {
	store := newSQLiteFixture(t)
	g, err := store.New("ABCD", makeScript(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if g.Code != "ABCD" || g.Version != 1 {
		t.Errorf("Code=%q Version=%d; want ABCD, 1", g.Code, g.Version)
	}
}

func TestSQLiteStore_NewGame_DuplicateCode_ReturnsErrDuplicate(t *testing.T) {
	store := newSQLiteFixture(t)
	if _, err := store.New("ABCD", makeScript(), false); err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err := store.New("ABCD", makeScript(), false)
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestSQLiteStore_Get_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := newSQLiteFixture(t)
	_, err := store.Get("no-such-id")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_FindByCode(t *testing.T) {
	store := newSQLiteFixture(t)
	created, _ := store.New("WXYZ", makeScript(), false)

	got, err := store.FindByCode("WXYZ")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestSQLiteStore_Exists(t *testing.T) {
	store := newSQLiteFixture(t)
	created, _ := store.New("ABCD", makeScript(), false)

	exists, _ := store.Exists(created.ID)
	if !exists {
		t.Errorf("expected exists=true for created id")
	}
	exists, _ = store.Exists("nope")
	if exists {
		t.Errorf("expected exists=false for unknown id")
	}
}

func TestSQLiteStore_FindOpen_ExcludesPrivate(t *testing.T) {
	store := newSQLiteFixture(t)
	_, _ = store.New("PUB1", makeScript(), false)
	_, _ = store.New("PRV1", makeScript(), true)

	games, err := store.FindOpen(10)
	if err != nil {
		t.Fatalf("FindOpen: %v", err)
	}
	if len(games) != 1 || games[0].Code != "PUB1" {
		t.Fatalf("want [PUB1], got %v", games)
	}
}
```

Note: `makeScript()` is already defined in `memory_test.go` in the same package — do not redeclare it.

- [ ] **Step 2.6 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run TestSQLiteStore -v
```

- [ ] **Step 2.7 — Run to confirm PASS (implementation is already in sqlite.go above)**

```bash
go test ./internal/repo/game/ -run TestSQLiteStore -v
```

If any test fails, diagnose and fix before continuing.

- [ ] **Step 2.8 — Full build**

```bash
go build ./...
```

Expected: PASS (assertion is commented out).

- [ ] **Step 2.9 — Commit**

```bash
git status
git add internal/repo/game/sqlite.go internal/repo/game/sqlite_test.go
git commit -m "feat(game/sqlite): add SQLiteStore with read operations"
```

---

## Task 3: `game.SQLiteStore` — write path (Update, UpdateField, DeleteField, Mutate)

The write path reuses `applyDottedPath`, `splitPath`, `fromMap`, `publishFieldUpdate`, and `publishDiff` from `memory.go` — they live in the same `game` package and need no re-declaration. The `saveWithVersion` helper performs the version-fenced UPDATE and is the heart of the CAS loop.

**Files:**
- Modify: `internal/repo/game/sqlite.go`
- Modify: `internal/repo/game/sqlite_test.go`

- [ ] **Step 3.1 — Write failing tests**

Append to `internal/repo/game/sqlite_test.go`:

```go
func TestSQLiteStore_Update_BumpsVersion(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	upd := &models.UpdateGame{Private: true}
	if err := store.Update(g.ID, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Version=%d Private=%v; want 2, true", got.Version, got.Private)
	}
}

func TestSQLiteStore_UpdateField(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	if err := store.UpdateField(g.ID, "data.foo", "bar"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if v, _ := got.PublicData["foo"]; v != "bar" {
		t.Errorf("data.foo = %v; want bar", v)
	}
}

func TestSQLiteStore_DeleteField(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.UpdateField(g.ID, "data.foo", "bar")

	if err := store.DeleteField(g.ID, "data.foo"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}
	got, _ := store.Get(g.ID)
	if _, still := got.PublicData["foo"]; still {
		t.Errorf("data.foo not deleted")
	}
}

func TestSQLiteStore_Mutate_SuccessfulApply(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	err := store.Mutate(g.ID, func(game *models.Game) error {
		game.Private = true
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Private || got.Version != 2 {
		t.Errorf("Mutate did not apply or bump version: %+v", got)
	}
}

func TestSQLiteStore_Mutate_UnknownID(t *testing.T) {
	store := newSQLiteFixture(t)
	err := store.Mutate("no-such-id", func(g *models.Game) error { return nil })
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 3.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run "TestSQLiteStore_Update|TestSQLiteStore_Delete|TestSQLiteStore_Mutate"
```

- [ ] **Step 3.3 — Implement the write path**

Append to `internal/repo/game/sqlite.go`. Note: `publishFieldUpdate` and `publishDiff` are methods on `MemoryStore`, not package-level functions — `SQLiteStore` needs its own copies. `applyDottedPath`, `splitPath`, and `fromMap` **are** package-level functions in `memory.go` and are shared without redeclaration.

```go
// publishFieldUpdate and publishDiff mirror MemoryStore's methods. They are
// defined on SQLiteStore because Go methods are not shared between structs.
func (s *SQLiteStore) publishFieldUpdate(id string, updated map[string]interface{}, removed []string) {
	if s.publisher == nil {
		return
	}
	updated, removed = events.SanitizeDelta(updated, removed)
	ev := events.ChangeEvent{
		ID:            id,
		OperationType: events.OpUpdate,
		Timestamp:     time.Now(),
		Collection:    collectionName,
		UpdatedFields: updated,
		RemovedFields: removed,
	}
	if !ev.HasChanges() {
		return
	}
	_ = s.publisher.Publish(s.ctx, ev)
}

func (s *SQLiteStore) publishDiff(id string, before map[string]interface{}, after *models.Game) {
	if s.publisher == nil {
		return
	}
	afterMap, err := events.ToMap(after)
	if err != nil {
		return
	}
	updated, removed := events.Diff(before, afterMap)
	s.publishFieldUpdate(id, updated, removed)
}

// saveWithVersion performs the version-fenced UPDATE. Returns (true, nil) on
// success, (false, nil) if the version has drifted (triggering mutation.Run
// retry), or (false, err) on a hard error.
func (s *SQLiteStore) saveWithVersion(g *models.Game, expectedVersion int64) (bool, error) {
	g.Version = expectedVersion + 1
	g.UpdatedAt = time.Now()

	blob, err := marshalGame(g)
	if err != nil {
		return false, err
	}
	private := 0
	if g.Private {
		private = 1
	}
	result, err := s.db.ExecContext(s.ctx,
		`UPDATE games SET data = ?, version = ?, private = ? WHERE id = ? AND version = ?`,
		blob, g.Version, private, g.ID, expectedVersion,
	)
	if err != nil {
		return false, fmt.Errorf("save game: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n == 1, nil
}

// loadWithVersion reads the game and its current version for use in Mutate's
// CAS loop.
func (s *SQLiteStore) loadWithVersion(id string) (*models.Game, int64, error) {
	var (
		data    string
		version int64
	)
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data, version FROM games WHERE id = ?`, id,
	).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load game: %w", err)
	}
	g, err := unmarshalGame(data)
	if err != nil {
		return nil, 0, err
	}
	return g, version, nil
}

func (s *SQLiteStore) Update(id string, upd *models.UpdateGame) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
	}
	before, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	if upd.Teams != nil {
		g.Teams = *upd.Teams
	}
	if upd.Players != nil {
		g.Players = *upd.Players
	}
	if upd.Stage != nil {
		g.Stage = *upd.Stage
	}
	if upd.PublicData != nil {
		g.PublicData = upd.PublicData
	}
	if upd.PrivateData != nil {
		g.PrivateData = upd.PrivateData
	}
	if upd.PlayerData != nil {
		g.PlayerData = upd.PlayerData
	}
	g.Private = upd.Private

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.publishDiff(id, before, g)
	return nil
}

func (s *SQLiteStore) UpdateField(id string, key string, value interface{}) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, value, false)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.publishFieldUpdate(id, map[string]interface{}{key: value}, nil)
	return nil
}

func (s *SQLiteStore) DeleteField(id string, key string) error {
	g, version, err := s.loadWithVersion(id)
	if err != nil {
		return err
	}
	m, err := events.ToMap(g)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	applyDottedPath(m, key, nil, true)
	if err := fromMap(m, g); err != nil {
		return fmt.Errorf("rehydrate: %w", err)
	}

	committed, err := s.saveWithVersion(g, version)
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("id %q: %w", id, repoErrors.ErrConflict)
	}
	s.publishFieldUpdate(id, nil, []string{key})
	return nil
}

// Mutate uses mutation.Run for the optimistic retry loop. The load/save
// callbacks do not hold any in-process lock — SQLite's WAL mode handles
// concurrent reads, and the version fence in saveWithVersion prevents lost
// updates. The distributed lock.Manager (passed via clients) serialises
// concurrent callers across goroutines within the process.
func (s *SQLiteStore) Mutate(id string, apply func(g *models.Game) error) error {
	var before map[string]interface{}

	return mutation.Run(
		s.ctx,
		s.locks,
		"game:"+id,
		func() (*models.Game, int64, error) {
			g, version, err := s.loadWithVersion(id)
			if err != nil {
				return nil, 0, err
			}
			beforeMap, err := events.ToMap(g)
			if err != nil {
				return nil, 0, err
			}
			before = beforeMap
			return g, version, nil
		},
		apply,
		func(g *models.Game, expectedVersion int64) (bool, error) {
			committed, err := s.saveWithVersion(g, expectedVersion)
			if err != nil {
				return false, err
			}
			if committed {
				s.publishDiff(id, before, g)
			}
			return committed, nil
		},
	)
}
```

- [ ] **Step 3.4 — Run tests to confirm PASS**

```bash
go test ./internal/repo/game/ -run "TestSQLiteStore" -v
```

- [ ] **Step 3.5 — Full build + race**

```bash
go build ./...
go test -race ./internal/repo/game/ -run TestSQLiteStore
```

- [ ] **Step 3.6 — Commit**

```bash
git status
git add internal/repo/game/sqlite.go internal/repo/game/sqlite_test.go
git commit -m "feat(game/sqlite): add write path (Update, UpdateField, DeleteField, Mutate)"
```

---

## Task 4: `game.SQLiteStore` — Player operations

Player operations delegate to `Mutate` for all writes, exactly mirroring `memory_player.go`. `HasPlayer` and `PlayerOnATeam` run a direct SELECT to avoid the full CAS overhead.

**Files:**
- Create: `internal/repo/game/sqlite_player.go`
- Modify: `internal/repo/game/sqlite_test.go`

- [ ] **Step 4.1 — Write failing tests**

Append to `internal/repo/game/sqlite_test.go`:

```go
func TestSQLiteStore_AddPlayer_ThenHasPlayer(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)

	if err := store.AddPlayer(g.ID, "user-1", "Alice"); err != nil {
		t.Fatalf("AddPlayer: %v", err)
	}
	if !store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = false")
	}
	if store.HasPlayer(g.ID, "user-x") {
		t.Error("HasPlayer(user-x) = true")
	}
	got, _ := store.Get(g.ID)
	if got.Players["user-1"].Name != "Alice" {
		t.Errorf("Name = %q; want Alice", got.Players["user-1"].Name)
	}
}

func TestSQLiteStore_RemovePlayer(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if err := store.RemovePlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("RemovePlayer: %v", err)
	}
	if store.HasPlayer(g.ID, "user-1") {
		t.Error("HasPlayer(user-1) = true after remove")
	}
}

func TestSQLiteStore_ConnectDisconnectPlayer(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if err := store.ConnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("ConnectPlayer: %v", err)
	}
	got, _ := store.Get(g.ID)
	if !got.Players["user-1"].Connected {
		t.Errorf("Connected = false after ConnectPlayer")
	}

	if err := store.DisconnectPlayer(g.ID, "user-1"); err != nil {
		t.Fatalf("DisconnectPlayer: %v", err)
	}
	got, _ = store.Get(g.ID)
	if got.Players["user-1"].Connected {
		t.Errorf("Connected = true after DisconnectPlayer")
	}
}
```

- [ ] **Step 4.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run "TestSQLiteStore_AddPlayer|TestSQLiteStore_Remove|TestSQLiteStore_Connect"
```

- [ ] **Step 4.3 — Implement**

Create `internal/repo/game/sqlite_player.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *SQLiteStore) HasPlayer(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	_, ok := g.Players[userId]
	return ok
}

func (s *SQLiteStore) PlayerOnATeam(id string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, team := range g.Teams {
		for _, pid := range team.PlayerIDs {
			if pid == userId {
				return true
			}
		}
	}
	return false
}

func (s *SQLiteStore) AddPlayer(id string, userId string, displayName string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, exists := g.Players[userId]; exists {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrDuplicate)
		}
		if g.Players == nil {
			g.Players = map[string]models.Player{}
		}
		g.Players[userId] = models.Player{
			Name:      displayName,
			Connected: false,
		}
		return nil
	})
}

func (s *SQLiteStore) RemovePlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		delete(g.Players, userId)
		return nil
	})
}

func (s *SQLiteStore) ConnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = true
		g.Players[userId] = p
		return nil
	})
}

func (s *SQLiteStore) DisconnectPlayer(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		p, ok := g.Players[userId]
		if !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		p.Connected = false
		g.Players[userId] = p
		return nil
	})
}
```

- [ ] **Step 4.4 — Run tests to confirm PASS**

```bash
go test ./internal/repo/game/ -run TestSQLiteStore -v
```

- [ ] **Step 4.5 — Commit**

```bash
git status
git add internal/repo/game/sqlite_player.go internal/repo/game/sqlite_test.go
git commit -m "feat(game/sqlite): add player operations"
```

---

## Task 5: `game.SQLiteStore` — Team operations

Team operations reuse `containsID` and `filterOutID` from `memory_team.go` — they are in the same package and need no re-declaration.

**Files:**
- Create: `internal/repo/game/sqlite_team.go`
- Modify: `internal/repo/game/sqlite_test.go`

- [ ] **Step 5.1 — Write failing tests**

Append to `internal/repo/game/sqlite_test.go`:

```go
func TestSQLiteStore_AddPlayerToTeam_ThenHasPlayerOnTeam(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if err := store.AddPlayerToTeam(g.ID, "red", "user-1"); err != nil {
		t.Fatalf("AddPlayerToTeam: %v", err)
	}
	if !store.HasPlayerOnTeam(g.ID, "red", "user-1") {
		t.Fatal("HasPlayerOnTeam(red, user-1) = false")
	}
}

func TestSQLiteStore_ChangePlayerTeam(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.AddPlayerToTeam(g.ID, "red", "user-1")

	if err := store.ChangePlayerTeam(g.ID, "blue", "user-1"); err != nil {
		t.Fatalf("ChangePlayerTeam: %v", err)
	}
	teamID, err := store.PlayerOnWhichTeam(g.ID, "user-1")
	if err != nil {
		t.Fatalf("PlayerOnWhichTeam: %v", err)
	}
	if teamID == nil || *teamID != "blue" {
		t.Errorf("team: want blue, got %v", teamID)
	}
}

func TestSQLiteStore_RemovePlayerFromTeam(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", scriptWithTeams(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.AddPlayerToTeam(g.ID, "red", "user-1")

	if err := store.RemovePlayerFromTeam(g.ID, "user-1"); err != nil {
		t.Fatalf("RemovePlayerFromTeam: %v", err)
	}
	if store.HasPlayerOnTeam(g.ID, "red", "user-1") {
		t.Errorf("still on team after remove")
	}
}
```

Note: `scriptWithTeams()` is already defined in `memory_test.go` in the same package — do not redeclare it.

- [ ] **Step 5.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run "TestSQLiteStore_AddPlayerToTeam|TestSQLiteStore_ChangePlayerTeam|TestSQLiteStore_RemovePlayerFromTeam"
```

- [ ] **Step 5.3 — Implement**

Create `internal/repo/game/sqlite_team.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *SQLiteStore) HasPlayerOnTeam(id string, teamId string, userId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	team, ok := g.Teams[teamId]
	if !ok {
		return false
	}
	return containsID(team.PlayerIDs, userId)
}

func (s *SQLiteStore) AddPlayerToTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		team, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		if !containsID(team.PlayerIDs, userId) {
			team.PlayerIDs = append(team.PlayerIDs, userId)
			g.Teams[teamId] = team
		}
		return nil
	})
}

func (s *SQLiteStore) RemovePlayerFromTeam(id string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		return nil
	})
}

func (s *SQLiteStore) ChangePlayerTeam(id string, teamId string, userId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		newTeam, ok := g.Teams[teamId]
		if !ok {
			return fmt.Errorf("team %q: %w", teamId, repoErrors.ErrNotFound)
		}
		if _, ok := g.Players[userId]; !ok {
			return fmt.Errorf("player %q: %w", userId, repoErrors.ErrNotFound)
		}
		for tid, team := range g.Teams {
			if containsID(team.PlayerIDs, userId) {
				team.PlayerIDs = filterOutID(team.PlayerIDs, userId)
				g.Teams[tid] = team
			}
		}
		newTeam.PlayerIDs = append(newTeam.PlayerIDs, userId)
		g.Teams[teamId] = newTeam
		return nil
	})
}

func (s *SQLiteStore) PlayerOnWhichTeam(id string, userId string) (*string, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	for tid, team := range g.Teams {
		if containsID(team.PlayerIDs, userId) {
			out := tid
			return &out, nil
		}
	}
	return nil, nil
}
```

- [ ] **Step 5.4 — Run tests to confirm PASS**

```bash
go test ./internal/repo/game/ -run TestSQLiteStore -v
```

- [ ] **Step 5.5 — Commit**

```bash
git status
git add internal/repo/game/sqlite_team.go internal/repo/game/sqlite_test.go
git commit -m "feat(game/sqlite): add team operations"
```

---

## Task 6: `game.SQLiteStore` — Host operations and restore Storer assertion

**Files:**
- Create: `internal/repo/game/sqlite_host.go`
- Modify: `internal/repo/game/sqlite.go` (uncomment assertion)
- Modify: `internal/repo/game/sqlite_test.go`

- [ ] **Step 6.1 — Write failing tests**

Append to `internal/repo/game/sqlite_test.go`:

```go
func TestSQLiteStore_SetPlayerAsHost_ThenPlayerIsHost(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")

	if store.HasHost(g.ID) {
		t.Fatal("HasHost = true before SetPlayerAsHost")
	}
	if err := store.SetPlayerAsHost(g.ID, "user-1"); err != nil {
		t.Fatalf("SetPlayerAsHost: %v", err)
	}
	if !store.HasHost(g.ID) || !store.PlayerIsHost(g.ID, "user-1") {
		t.Error("SetPlayerAsHost did not take effect")
	}
}

func TestSQLiteStore_UnsetHost(t *testing.T) {
	store := newSQLiteFixture(t)
	g, _ := store.New("ABCD", makeScript(), false)
	_ = store.AddPlayer(g.ID, "user-1", "Alice")
	_ = store.SetPlayerAsHost(g.ID, "user-1")

	if err := store.UnsetHost(g.ID); err != nil {
		t.Fatalf("UnsetHost: %v", err)
	}
	if store.HasHost(g.ID) {
		t.Error("HasHost = true after UnsetHost")
	}
}
```

- [ ] **Step 6.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/game/ -run "TestSQLiteStore_SetPlayerAsHost|TestSQLiteStore_UnsetHost"
```

- [ ] **Step 6.3 — Implement**

Create `internal/repo/game/sqlite_host.go`:

```go
package game

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func (s *SQLiteStore) HasHost(id string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	for _, p := range g.Players {
		if p.Host {
			return true
		}
	}
	return false
}

func (s *SQLiteStore) PlayerIsHost(id string, playerId string) bool {
	g, err := s.Get(id)
	if err != nil {
		return false
	}
	p, ok := g.Players[playerId]
	return ok && p.Host
}

func (s *SQLiteStore) SetPlayerAsHost(id string, playerId string) error {
	return s.Mutate(id, func(g *models.Game) error {
		if _, ok := g.Players[playerId]; !ok {
			return fmt.Errorf("player %q: %w", playerId, repoErrors.ErrNotFound)
		}
		for uid, p := range g.Players {
			p.Host = (uid == playerId)
			g.Players[uid] = p
		}
		return nil
	})
}

func (s *SQLiteStore) UnsetHost(id string) error {
	return s.Mutate(id, func(g *models.Game) error {
		for uid, p := range g.Players {
			p.Host = false
			g.Players[uid] = p
		}
		return nil
	})
}
```

- [ ] **Step 6.4 — Restore the assertion**

In `internal/repo/game/sqlite.go`, replace the commented-out placeholder with:

```go
var _ Storer = (*SQLiteStore)(nil)
```

- [ ] **Step 6.5 — Build + test + race**

```bash
go build ./...
go test -race ./internal/repo/game/ -v
```

Expected: PASS, no data-race warnings.

- [ ] **Step 6.6 — Commit**

```bash
git status
git add internal/repo/game/sqlite_host.go internal/repo/game/sqlite.go internal/repo/game/sqlite_test.go
git commit -m "feat(game/sqlite): add host operations; complete Storer implementation"
```

---

## Task 7: `user.SQLiteStore`

User rows: `(id TEXT PRIMARY KEY, email TEXT UNIQUE, data TEXT)`. The data column holds the full `models.User` JSON. `FindFirst`/`Find` scan all rows matching a field value — the user table is small enough that this is acceptable.

**Files:**
- Create: `internal/repo/user/sqlite.go`
- Create: `internal/repo/user/sqlite_test.go`

- [ ] **Step 7.1 — Write failing tests**

Create `internal/repo/user/sqlite_test.go`:

```go
package user

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestUserSQLiteStore_New_AssignsID(t *testing.T) {
	s := newSQLiteFixture(t)
	u, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID empty")
	}
}

func TestUserSQLiteStore_New_DuplicateEmail(t *testing.T) {
	s := newSQLiteFixture(t)
	_, _ = s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	_, err := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice2"})
	if !errors.Is(err, repoErrors.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestUserSQLiteStore_Get_UnknownID(t *testing.T) {
	s := newSQLiteFixture(t)
	_, err := s.Get("nope")
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserSQLiteStore_FindFirst(t *testing.T) {
	s := newSQLiteFixture(t)
	u, _ := s.New(models.CreateUser{Email: "a@b.c", Name: "Alice"})
	got, err := s.FindFirst("email", "a@b.c")
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindFirst: got=%v err=%v", got, err)
	}
}
```

- [ ] **Step 7.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/user/ -run TestUserSQLiteStore
```

- [ ] **Step 7.3 — Implement**

Create `internal/repo/user/sqlite.go`:

```go
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

func marshalUser(u *models.User) (string, error) {
	b, err := json.Marshal(u)
	if err != nil {
		return "", fmt.Errorf("marshal user: %w", err)
	}
	return string(b), nil
}

func unmarshalUser(data string) (*models.User, error) {
	var u models.User
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return nil, fmt.Errorf("unmarshal user: %w", err)
	}
	return &u, nil
}

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
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO users (id, email, data) VALUES (?, ?, ?)`,
		u.ID, u.Email, blob,
	)
	if err != nil {
		if isSQLiteConstraintUnique(err) {
			return nil, fmt.Errorf("email %q: %w", c.Email, repoErrors.ErrDuplicate)
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (s *SQLiteStore) Get(id string) (*models.User, error) {
	var data string
	err := s.db.QueryRowContext(s.ctx,
		`SELECT data FROM users WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("id %q: %w", id, repoErrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return unmarshalUser(data)
}

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

func (s *SQLiteStore) Find(key string, value string) ([]*models.User, error) {
	rows, err := s.db.QueryContext(s.ctx, `SELECT data FROM users`)
	if err != nil {
		return nil, fmt.Errorf("find users: %w", err)
	}
	defer rows.Close()

	var out []*models.User
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u, err := unmarshalUser(data)
		if err != nil {
			return nil, err
		}
		if matchUserField(u, key, value) {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

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
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE users SET email = ?, data = ? WHERE id = ?`,
		existing.Email, blob, u.ID,
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
// It is package-local here in the user package; there is no collision with
// memory.go because memory.go does not define this function.
func isSQLiteConstraintUnique(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// matchUserField is already defined in memory.go (same package) — do NOT
// redeclare it here. This comment is a reminder to the implementer.
```

- [ ] **Step 7.4 — Run tests to confirm PASS**

```bash
go test ./internal/repo/user/ -v
```

- [ ] **Step 7.5 — Full build**

```bash
go build ./...
```

- [ ] **Step 7.6 — Commit**

```bash
git status
git add internal/repo/user/sqlite.go internal/repo/user/sqlite_test.go
git commit -m "feat(user/sqlite): add SQLiteStore"
```

---

## Task 8: `session.SQLiteStore`

Session rows: `(id TEXT PRIMARY KEY, token TEXT UNIQUE, user_id TEXT UNIQUE, data TEXT)`. One-session-per-user is enforced by the `UNIQUE` constraint on `user_id`. A second `New` for the same `UserID` must return the existing session (matching MongoStore behaviour).

**Files:**
- Create: `internal/repo/session/sqlite.go`
- Create: `internal/repo/session/sqlite_test.go`

- [ ] **Step 8.1 — Write failing tests**

Create `internal/repo/session/sqlite_test.go`:

```go
package session

import (
	"context"
	"errors"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
)

func newSQLiteFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	store, err := NewSQLiteStore(context.Background(), db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestSessionSQLiteStore_New_AssignsIDAndKeepsCallerToken(t *testing.T) {
	s := newSQLiteFixture(t)
	sess, err := s.New(models.CreateSession{Token: "tok-1", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sess.ID == "" || sess.Token != "tok-1" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestSessionSQLiteStore_New_DuplicateUserID_ReturnsExisting(t *testing.T) {
	s := newSQLiteFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	again, err := s.New(models.CreateSession{Token: "t-2", UserID: "u-1"})
	if err != nil {
		t.Fatalf("New (dup user): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("expected same session ID; got %q vs %q", again.ID, first.ID)
	}
}

func TestSessionSQLiteStore_GetByToken(t *testing.T) {
	s := newSQLiteFixture(t)
	first, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	got, err := s.GetByToken("t-1")
	if err != nil || got.ID != first.ID {
		t.Fatalf("GetByToken: got=%v err=%v", got, err)
	}
}

func TestSessionSQLiteStore_Delete(t *testing.T) {
	s := newSQLiteFixture(t)
	sess, _ := s.New(models.CreateSession{Token: "t-1", UserID: "u-1"})
	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(sess.ID)
	if !errors.Is(err, repoErrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}
```

- [ ] **Step 8.2 — Run to confirm FAIL**

```bash
go test ./internal/repo/session/ -run TestSessionSQLiteStore
```

- [ ] **Step 8.3 — Implement**

Create `internal/repo/session/sqlite.go`:

```go
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/robbiebyrd/indri/internal/models"
	repoErrors "github.com/robbiebyrd/indri/internal/repo"
	"github.com/robbiebyrd/indri/internal/repo/ids"
)

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

func marshalSession(sess *models.Session) (string, error) {
	// Token is tagged `json:"-"` on Session; store it separately so it round-trips.
	// We embed it in a wrapper struct for the blob.
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

func ptrOrNilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// New enforces one-session-per-user: if a session already exists for the
// given UserID, the existing session is returned unchanged.
func (s *SQLiteStore) New(c models.CreateSession) (*models.Session, error) {
	if c.UserID == "" {
		return nil, errors.New("session must have a user id")
	}

	// Check for existing session by user_id.
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

	sess := &models.Session{
		ID:        ids.New(),
		Token:     c.Token,
		GameID:    ptrOrNilStr(c.GameID),
		UserID:    ptrOrNilStr(c.UserID),
		TeamID:    ptrOrNilStr(c.TeamID),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	blob, err := marshalSession(sess)
	if err != nil {
		return nil, err
	}

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
	if u.GameID != "" {
		sess.GameID = ptrOrNilStr(u.GameID)
	}
	if u.UserID != "" {
		sess.UserID = ptrOrNilStr(u.UserID)
	}
	if u.TeamID != "" {
		sess.TeamID = ptrOrNilStr(u.TeamID)
	}
	sess.UpdatedAt = time.Now()

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

// matchSessionField is already defined in memory.go (same package) — do NOT
// redeclare it here. SQLiteStore.Find calls it directly.
```

- [ ] **Step 8.4 — Run tests to confirm PASS**

```bash
go test ./internal/repo/session/ -v
```

- [ ] **Step 8.5 — Full build**

```bash
go build ./...
```

- [ ] **Step 8.6 — Commit**

```bash
git status
git add internal/repo/session/sqlite.go internal/repo/session/sqlite_test.go
git commit -m "feat(session/sqlite): add SQLiteStore"
```

---

## Task 9: Env var, config key, wire `"sqlite"` case, end-to-end smoke test

**Files:**
- Modify: `internal/repo/env/env.go`
- Modify: `example/server.json`
- Modify: `internal/injector/repos.go`

- [ ] **Step 9.1 — Add `SQLitePath` to `env.Vars`**

In `internal/repo/env/env.go`, add after the `DBBackend` line:

```go
SQLitePath string `default:"./indri.db" envconfig:"SQLITE_PATH" json:"sqlitePath" flag:"sqlite-path"`
```

- [ ] **Step 9.2 — Verify env test still passes**

```bash
go test ./internal/repo/env/
```

- [ ] **Step 9.3 — Update `example/server.json`**

Add the flat key at the top level, alongside `dbBackend`:

```json
"sqlitePath": "./indri.db",
```

The full block will look like:

```json
{
  "listenAddress": "localhost",
  "listenPort": 5002,
  ...
  "dbBackend": "mongodb",
  "sqlitePath": "./indri.db",
  ...
}
```

- [ ] **Step 9.4 — Add `"sqlite"` case to `GetRepos`**

In `internal/injector/repos.go`, add the import for the sqlite client and the new case. The repos injector receives `env.SQLitePath` as the DSN; the SQLite `*sql.DB` is opened here and shared across all three stores.

Add imports:

```go
import (
    ...
    sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
)
```

Add case inside the switch:

```go
case "sqlite":
    sqliteDB, err := sqliteClient.Open(env.SQLitePath)
    if err != nil {
        return nil, fmt.Errorf("open sqlite %q: %w", env.SQLitePath, err)
    }
    gr, err = gameRepo.NewSQLiteStore(ctx, sqliteDB, clients.LockManager, clients.Publisher)
    if err != nil {
        return nil, err
    }
    ur, err = userRepo.NewSQLiteStore(ctx, sqliteDB)
    if err != nil {
        return nil, err
    }
    sr, err = sessionRepo.NewSQLiteStore(ctx, sqliteDB)
    if err != nil {
        return nil, err
    }
```

- [ ] **Step 9.5 — Build + full test suite**

```bash
go build ./...
go test ./...
go test -race ./...
```

Expected: PASS. MongoDB integration tests skip because no URI is set.

- [ ] **Step 9.6 — Smoke-test the server on SQLite backend**

```bash
INDRI_DB_BACKEND=sqlite INDRI_SQLITE_PATH=":memory:" go run ./cmd/server -script ./example/server.json > /tmp/indri-sqlite.log 2>&1 &
SERVER_PID=$!
sleep 2
nc -z localhost 5002 && echo "listening" || echo "not listening"
grep -i "mongo\|dial\|connect" /tmp/indri-sqlite.log && echo "!!! unexpected db contact" || echo "clean start"
kill $SERVER_PID 2>/dev/null
wait $SERVER_PID 2>/dev/null
```

Expected: `listening`, then `clean start`. Fix any log output that indicates unintended MongoDB contact before committing.

- [ ] **Step 9.7 — Commit**

```bash
git status
git add internal/repo/env/env.go example/server.json internal/injector/repos.go
git commit -m "feat(repo): add sqlitePath env var; wire sqlite backend in injector"
```

---

## Final Verification

```bash
go vet ./...
go build ./...
go test ./...
go test -race ./...
```

Expected: all PASS.

Manual end-to-end with persistent file:

```bash
INDRI_DB_BACKEND=sqlite go run ./example/tictactoe -script ./example/tictactoe.json
```

Connect a client, play a full game. Restart the server — state should persist across restarts (unlike memory backend). Verify by:
1. Creating a game, noting the game code.
2. Stopping the server (`ctrl-c`).
3. Restarting it with the same `INDRI_SQLITE_PATH`.
4. Using `reconnect` action with the saved session token — the game state should still be present.

In-memory smoke test (no file):

```bash
INDRI_DB_BACKEND=sqlite INDRI_SQLITE_PATH=":memory:" go run ./cmd/server -script ./example/server.json
```

Expected: boots without errors, no Mongo contact in logs.

---

## Open Questions

None. All design decisions are made above. Implementers should verify the exact error message format of `modernc.org/sqlite` constraint errors at Step 2.3 if the `isSQLiteConstraintUnique` helper does not fire correctly — the string `"UNIQUE constraint failed"` is the documented SQLite error prefix.

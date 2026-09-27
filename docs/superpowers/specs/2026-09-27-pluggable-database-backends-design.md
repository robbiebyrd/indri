# Pluggable Database Backends Design

**Date:** 2026-09-27  
**Status:** Draft

## Overview

Make MongoDB optional by introducing a pluggable database backend system. The server selects its storage backend from a compiled-in registry based on a key in `server.json`. Four backends ship in the first pass: MongoDB (existing), in-memory, SQLite, and PostgreSQL.

## Goals

- Run the server without any external infrastructure (in-memory backend for development and testing).
- Support SQLite for zero-config single-instance deployments.
- Support PostgreSQL for production deployments already running Postgres.
- Select the backend via `server.json` — no code changes required to switch.
- Preserve all existing MongoDB behaviour and the mutation/concurrency model.

## Non-Goals

- Dynamic plugin loading (`.so` files, separate binaries, gRPC adapters).
- Data migration tooling for existing MongoDB deployments using `bson.ObjectID` IDs.
- Cross-backend data sync or replication.
- Connection pooling beyond what each driver provides natively.

---

## Architecture

### 1. Models (`internal/models/`)

Replace `bson.ObjectID` with `string` on all three stored entities.

```go
// Game, User, Session — before
ID bson.ObjectID `bson:"_id"`

// After
ID string `bson:"_id" json:"id"`
```

- `bson:` struct tags are kept — MongoDB stores string UUIDs under `_id` without issue, and the go-mongox ORM uses the tags for field name mapping.
- The `go.mongodb.org/mongo-driver` package is no longer imported in `internal/models/`.
- Each backend's constructor generates a UUID v4 when creating new records (`google/uuid` or stdlib `crypto/rand` + `encoding/hex`).
- No migration path for existing data is in scope; this is a clean break.

### 2. Shared Error Sentinel (`internal/repo/errors.go`)

A new file defines the canonical set of repo-layer errors that all backends map to:

```go
var (
    ErrNotFound  = errors.New("not found")
    ErrDuplicate = errors.New("duplicate key")
    ErrConflict  = errors.New("version conflict")
)
```

Callers never inspect driver-specific error types. Each adapter wraps its own driver errors into these sentinels. The existing `mongo.IsDuplicateKeyError(err)` check moves inside `MongoStore`.

### 3. Repo Layer Structure

The Storer interfaces in each `interface.go` are unchanged — they already define the contract.  
Each repo package gains multiple named implementations:

```
internal/repo/game/
  interface.go           unchanged — Storer interface (35 methods)
  errors.go              shared sentinel errors (or internal/repo/errors.go)
  mongo.go               MongoStore struct + constructor (was game.go)
  mongo_player.go        player methods on MongoStore (was player.go)
  mongo_team.go          team methods on MongoStore (was team.go)
  mongo_host.go          host methods on MongoStore (was host.go)
  memory.go              MemoryStore — map[string]*models.Game + sync.RWMutex
  sqlite.go              SQLiteStore — database/sql + mattn/go-sqlite3
  postgres.go            PostgresStore — database/sql + lib/pq or pgx

internal/repo/user/
  interface.go           unchanged
  mongo.go               MongoStore (was user.go)
  memory.go
  sqlite.go
  postgres.go

internal/repo/session/
  interface.go           unchanged
  mongo.go               MongoStore (was session.go)
  memory.go
  sqlite.go
  postgres.go
```

**Struct naming:** `MongoStore`, `MemoryStore`, `SQLiteStore`, `PostgresStore`.  
**Constructors:** `NewMongoStore(...)`, `NewMemoryStore(...)`, `NewSQLiteStore(...)`, `NewPostgresStore(...)`.

All implement the package's `Storer` interface; compile-time assertions (`var _ Storer = (*MongoStore)(nil)` etc.) enforce this.

#### Mutate() across backends

`Mutate()` is part of `game.Storer` — each backend implements its own version-fenced concurrent write:

| Backend  | Mechanism |
|----------|-----------|
| MongoDB  | Existing: `$set` + version filter in update, with distributed lock |
| PostgreSQL | `UPDATE games SET ... WHERE id = $1 AND version = $2` in a transaction |
| SQLite   | Same `UPDATE ... WHERE version = ?` pattern (serialized by SQLite's WAL) |
| Memory   | `sync.Mutex` per game + version increment check |

The `mutation.Run()` service layer (optimistic retry loop + lock manager) is already backend-agnostic and requires no changes.

### 4. Config (`internal/repo/env/env.go` + `server.json`)

**`server.json`** gains a top-level `database` block:

```json
{
  "database": {
    "backend": "mongodb",
    "mongodb": {
      "uri": "mongodb://localhost:27017",
      "database": "indri",
      "authDatabase": "admin"
    },
    "sqlite": {
      "path": "./indri.db"
    },
    "postgres": {
      "uri": "postgres://user:pass@localhost/indri?sslmode=disable"
    },
    "memory": {}
  }
}
```

**`env.go`** adds:

```go
DBBackend       string `default:"mongodb"   envconfig:"DB_BACKEND"`
SQLitePath      string `default:"./indri.db" envconfig:"SQLITE_PATH"`
PostgresURI     string `default:""           envconfig:"POSTGRES_URI"`
```

Existing `INDRI_MONGO_URI`, `INDRI_MONGO_DATABASE`, `INDRI_MONGO_AUTH_DATABASE` continue to work unchanged.

### 5. Injector (`internal/injector/`)

**`injector.go`** — `ReposInjector` fields change from concrete types to interfaces:

```go
// Before
GameRepo    *gameRepo.Store
UserRepo    *userRepo.Store
SessionRepo *sessionRepo.Store

// After
GameRepo    gameRepo.Storer
UserRepo    userRepo.Storer
SessionRepo sessionRepo.Storer
```

**`clients.go`** — `MongoDBClient` becomes optional (nil when backend ≠ mongodb). No new generic DB client abstraction is needed; each backend constructor receives only what it needs.

**`repos.go`** — `GetRepos()` selects the implementation:

```go
switch env.DBBackend {
case "mongodb":
    gameRepo, err = game.NewMongoStore(ctx, clients.MongoDBClient, clients.LockManager, clients.Publisher)
case "memory":
    gameRepo, err = game.NewMemoryStore(ctx, clients.LockManager, clients.Publisher)
case "sqlite":
    gameRepo, err = game.NewSQLiteStore(ctx, env.SQLitePath, clients.LockManager, clients.Publisher)
case "postgres":
    gameRepo, err = game.NewPostgresStore(ctx, env.PostgresURI, clients.LockManager, clients.Publisher)
default:
    return nil, fmt.Errorf("unknown database backend %q", env.DBBackend)
}
```

Any service currently typed to `*gameRepo.Store` is updated to `gameRepo.Storer`; `go build ./...` surfaces every callsite.

### 6. Error Handling

- All adapter-specific error translation happens inside the adapter (e.g., `mongo.IsDuplicateKeyError` stays in `MongoStore`, `sqlite3.ErrConstraintUnique` stays in `SQLiteStore`).
- Public surface to callers: `repo.ErrNotFound`, `repo.ErrDuplicate`, `repo.ErrConflict`.
- Publish failures remain logged-but-not-returned (unchanged from current behaviour).

### 7. Testing

Each adapter gets its own test file (`mongo_test.go`, `memory_test.go`, `sqlite_test.go`, `postgres_test.go`). A shared helper runs the full `Storer` contract against any implementation:

```go
// internal/repo/game/testing.go (build tag: testing)
func RunStorerTests(t *testing.T, s game.Storer) { ... }
```

This avoids duplicating the 35-method game store test suite across four backends. The in-memory backend is the default for unit tests (no external deps); MongoDB tests run only when `INDRI_MONGO_URI` is set (integration tag).

---

## File Change Summary

| File | Change |
|------|--------|
| `internal/models/game.go`, `user.go`, `session.go` | `bson.ObjectID` → `string` |
| `internal/repo/errors.go` | New — shared sentinel errors |
| `internal/repo/game/game.go` | Rename → `mongo.go`, struct → `MongoStore` |
| `internal/repo/game/player.go` | Rename → `mongo_player.go` |
| `internal/repo/game/team.go` | Rename → `mongo_team.go` |
| `internal/repo/game/host.go` | Rename → `mongo_host.go` |
| `internal/repo/game/memory.go` | New |
| `internal/repo/game/sqlite.go` | New |
| `internal/repo/game/postgres.go` | New |
| `internal/repo/user/user.go` | Rename → `mongo.go`, struct → `MongoStore` |
| `internal/repo/user/memory.go` | New |
| `internal/repo/user/sqlite.go` | New |
| `internal/repo/user/postgres.go` | New |
| `internal/repo/session/session.go` | Rename → `mongo.go`, struct → `MongoStore` |
| `internal/repo/session/memory.go` | New |
| `internal/repo/session/sqlite.go` | New |
| `internal/repo/session/postgres.go` | New |
| `internal/repo/env/env.go` | Add `DBBackend`, `SQLitePath`, `PostgresURI` |
| `internal/injector/injector.go` | Repo fields → interfaces |
| `internal/injector/clients.go` | `MongoDBClient` optional |
| `internal/injector/repos.go` | Backend switch in `GetRepos()` |
| `example/server.json` | Add `database` block |
| Services holding `*gameRepo.Store` | Update to `gameRepo.Storer` |

---

## Open Questions

None — all key decisions were made during the design session.

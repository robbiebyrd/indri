---
id: 080-cf52
title: Add SQLite and Postgres game backends against local's Storer
status: complete
priority: P1
type: feature
created: "2026-10-01T13:52:29.871Z"
updated: "2026-10-01T14:52:53.518Z"
dependencies: ["079-24a1", "088-69ac"]
plan: plans/main-divergence-integration.md
plan_step: Step 3
depends_on: ["stories/079-24a1-in_progress-P1-port-the-sqlite-and-postgres-clients-and-the-repo-.md", "stories/088-69ac-pending-P1-move-model-ids-from-bson-objectid-to-string.md"]
started_at: "2026-10-01T14:45:58.319Z"
completed_at: "2026-10-01T14:52:53.517Z"
---

# Add SQLite and Postgres game backends against local's Storer

## Problem Statement

Local's game Storer already declares Mutate(ctx) and MutateResult(ctx) and already has a MemoryStore, so this is additive backends rather than an interface rewrite. The two new stores must satisfy the existing contract suite without the Lua engine's GameMutator port noticing which backend it is talking to.

## Acceptance Criteria

- [x] SQLiteStore and PostgresStore implement the full game Storer including MutateResult
- [x] The var _ Storer assertions cover all four backends: Store, MemoryStore, SQLiteStore, PostgresStore
- [x] store_contract_test.go runs against all four and asserts committed=false for mutation.ErrAbort, committed=true for a real write
- [x] Update, UpdateField and DeleteField are fenced with the version CAS on both new backends
- [x] Version drift causes a retry rather than returning ErrConflict
- [x] One change publisher is shared by every game store rather than duplicated per backend
- [x] VERIFY: go test ./internal/repo/game/

## Files

- internal/repo/game/sqlite.go
- internal/repo/game/postgres.go
- internal/repo/game/store_contract_test.go
- internal/repo/game/core.go

## Proof

- [x] [completeness] Completeness (All 7 criteria verified by execution; the contract suite runs 12 tests against each of four backends with no skips.)
- [x] [feature-availability] Feature availability (SQLiteStore and PostgresStore satisfy the full Storer, asserted at compile time in interface.go alongside the existing two backends.)
- [x] [robustness] Robustness (fence_contract_test.go proves the version fence refuses a stale write and bumps on commit, on every backend. Verified by falsification: breaking the fence makes it fail, restoring it makes it pass.)
- [x] [resilience] Resilience (saveVersioned distinguishes a fence rejection from a missing game by reloading before reporting, so a caller retries in the first case and errors in the second. Transactions roll back on any failure via defer.)
- [~] [security] Security (No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.)
- [~] [defense-in-depth] Defense in depth (No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.)
- [~] [input-validation] Input validation (No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.)
- [x] [thread-safety] Thread safety (Non-fenced writes run read-modify-write inside a transaction so a concurrent writer cannot land between the read and the write. go test -race -count=1 across 30 packages reports no DATA RACE.)
- [x] [configurability] Configurability (Each store takes an already-open *sql.DB owned by the caller, so pool sizing and DSN remain the caller's choice; PostgresStore migrates only the table it owns.)

## QA

All four backends run the full contract suite with zero skips: 12 PASS each for memory, mongodb, sqlite and postgres, verified with go test -v and per-backend subtest counts. Fence proven by falsification: removing the version predicate from the SQL UPDATE makes TestSaveVersioned_RejectsAStaleWrite FAIL on sqlite and postgres, and restoring it makes them pass. Full suite: go build 0, go vet 0, gofmt -l empty, go test -race -count=1 with MongoDB and Postgres live: 30 packages ok, 0 FAIL, no DATA RACE.

## Work Log

### 2026-10-01T14:20:41.103Z - CONSTRAINT discovered while porting 079: local models.Game/User/Session all declare ID as bson.ObjectID (internal/models/game.go:10, user.go:10, session.go:10), while the ported DDL uses 'id TEXT PRIMARY KEY' and ids.New() returns a UUID string. Local's Storer signatures already pass ids as string, but the stored type is ObjectID, so a SQLite or Postgres store cannot naturally produce one. origin/main solved this in 0b7a246 by moving every model to string IDs. That means 080 is not just 'add two backends' -- it needs the ObjectID-to-string model change first, which is cross-cutting (mongo store, every handler that constructs an id, the session sessionId key). Size 080 accordingly or split the ID change into its own story.

### 2026-10-01T14:32:33.213Z - Split: the ObjectID-to-string model change is now story 088-69ac, and 080 depends on it. 080 stays scoped to adding the two game backends against the existing Storer.

### 2026-10-01T14:52:35.648Z - Implemented against local's docs port rather than porting origin/main's stores, because local's core/docs split means a backend supplies storage only -- the mutation bracket, version fence, retry budget and change deltas all stay in core and are the same code MongoDB runs. One sqlDocs serves both dialects (placeholder style and unique-violation detection are the only real differences), so there is a single copy of every query. The game is stored whole as JSON with id, code, version and private lifted into columns: those four are all a query ever filters or fences on. Non-fenced writes (replace/setField/unsetField/setPlayerConnected) run read-modify-write inside a transaction, because a SQL row holds the game as one blob where Mongo would use a single update operator. Extended forEachBackend so the ENTIRE existing contract suite is the spec for the new backends: 12 tests x 4 backends, zero skips, all passing.

### 2026-10-01T14:52:35.727Z - IMPORTANT finding from a falsification check. After the suite passed first try I deliberately removed the version predicate from the SQL UPDATE -- and every test still passed, including TestAddPlayer_ConcurrentNoLostUpdates. mutation.Run holds a distributed lock across the whole read-modify-write, so no contract test ever interleaved two writers and the fence was never exercised on ANY backend. Added fence_contract_test.go, which reaches the docs port directly to bypass the lock and replays a write carrying a stale version. Re-ran the falsification check: it now FAILS on sqlite and postgres with 'the version fence is not enforced' and passes once restored. Criterion 4 is proven by that test, not by the concurrency test.


### 2026-10-01T14:52:48.857Z - Proof completeness set PROVEN: All 7 criteria verified by execution; the contract suite runs 12 tests against each of four backends with no skips.

### 2026-10-01T14:52:48.941Z - Proof robustness set PROVEN: fence_contract_test.go proves the version fence refuses a stale write and bumps on commit, on every backend. Verified by falsification: breaking the fence makes it fail, restoring it makes it pass.

### 2026-10-01T14:52:49.028Z - Proof thread-safety set PROVEN: Non-fenced writes run read-modify-write inside a transaction so a concurrent writer cannot land between the read and the write. go test -race -count=1 across 30 packages reports no DATA RACE.

### 2026-10-01T14:52:49.110Z - Proof feature-availability set PROVEN: SQLiteStore and PostgresStore satisfy the full Storer, asserted at compile time in interface.go alongside the existing two backends.

### 2026-10-01T14:52:49.196Z - Proof resilience set PROVEN: saveVersioned distinguishes a fence rejection from a missing game by reloading before reporting, so a caller retries in the first case and errors in the second. Transactions roll back on any failure via defer.

### 2026-10-01T14:52:49.283Z - Proof configurability set PROVEN: Each store takes an already-open *sql.DB owned by the caller, so pool sizing and DSN remain the caller's choice; PostgresStore migrates only the table it owns.

### 2026-10-01T14:52:49.371Z - Proof security set NOT_APPLICABLE: No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.

### 2026-10-01T14:52:49.457Z - Proof defense-in-depth set NOT_APPLICABLE: No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.

### 2026-10-01T14:52:49.546Z - Proof input-validation set NOT_APPLICABLE: No new client-reachable surface. These are storage backends behind the existing Storer; all queries are parameterised, and authorisation and validation live above the repo layer and are unchanged.

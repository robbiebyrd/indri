---
id: 080-cf52
title: Add SQLite and Postgres game backends against local's Storer
status: pending
priority: P1
type: feature
created: "2026-10-01T13:52:29.871Z"
updated: "2026-10-01T13:52:39.625Z"
dependencies: ["079-24a1"]
plan: plans/main-divergence-integration.md
plan_step: Step 3
depends_on: ["stories/079-24a1-pending-P1-port-the-sqlite-and-postgres-clients-and-the-repo-.md"]
---

# Add SQLite and Postgres game backends against local's Storer

## Problem Statement

Local's game Storer already declares Mutate(ctx) and MutateResult(ctx) and already has a MemoryStore, so this is additive backends rather than an interface rewrite. The two new stores must satisfy the existing contract suite without the Lua engine's GameMutator port noticing which backend it is talking to.

## Acceptance Criteria

- [ ] SQLiteStore and PostgresStore implement the full game Storer including MutateResult
- [ ] The var _ Storer assertions cover all four backends: Store, MemoryStore, SQLiteStore, PostgresStore
- [ ] store_contract_test.go runs against all four and asserts committed=false for mutation.ErrAbort, committed=true for a real write
- [ ] Update, UpdateField and DeleteField are fenced with the version CAS on both new backends (origin 26b9688)
- [ ] Version drift causes a retry rather than returning ErrConflict (origin 046a524)
- [ ] One change publisher is shared by every game store rather than duplicated per backend
- [ ] VERIFY: go test ./internal/repo/game/

## Files

- internal/repo/game/sqlite.go
- internal/repo/game/postgres.go
- internal/repo/game/store_contract_test.go
- internal/repo/game/core.go

## Proof

- [ ] [completeness] Completeness
- [ ] [feature-availability] Feature availability
- [ ] [robustness] Robustness
- [ ] [resilience] Resilience
- [ ] [security] Security
- [ ] [defense-in-depth] Defense in depth
- [ ] [input-validation] Input validation
- [ ] [thread-safety] Thread safety
- [ ] [configurability] Configurability

## Work Log

### 2026-10-01T14:20:41.103Z - CONSTRAINT discovered while porting 079: local models.Game/User/Session all declare ID as bson.ObjectID (internal/models/game.go:10, user.go:10, session.go:10), while the ported DDL uses 'id TEXT PRIMARY KEY' and ids.New() returns a UUID string. Local's Storer signatures already pass ids as string, but the stored type is ObjectID, so a SQLite or Postgres store cannot naturally produce one. origin/main solved this in 0b7a246 by moving every model to string IDs. That means 080 is not just 'add two backends' -- it needs the ObjectID-to-string model change first, which is cross-cutting (mongo store, every handler that constructs an id, the session sessionId key). Size 080 accordingly or split the ID change into its own story.


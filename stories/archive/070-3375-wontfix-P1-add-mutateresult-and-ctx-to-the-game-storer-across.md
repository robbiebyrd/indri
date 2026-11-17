---
id: 070-3375
title: Add MutateResult and ctx to the game Storer across all four backends
status: wontfix
priority: P1
type: feature
created: "2026-10-01T13:29:44.857Z"
updated: "2026-10-01T13:51:33.142Z"
dependencies: ["069-1d64"]
plan: plans/main-divergence-integration.md
plan_step: Step 3
depends_on: ["stories/069-1d64-pending-P1-carry-mutation-runresult-and-the-lock-tests-onto-t.md"]
completed_at: "2026-10-01T13:51:33.141Z"
---

# Add MutateResult and ctx to the game Storer across all four backends

## Problem Statement

Remote's Storer.Mutate(id, apply) takes no context and does not report whether it committed. The Lua engine's GameMutator port requires MutateResult(ctx, id, apply) (committed bool, err error). This is the single foundation change the engine port needs, and it must land on mongo, memory, sqlite and postgres together or the Storer assertion breaks the build.

## Acceptance Criteria

- [ ] Storer declares Mutate(ctx, id, apply) error and MutateResult(ctx, id, apply) (committed bool, err error)
- [ ] MutateResult is the primitive and Mutate delegates to it, so no second load/save round trip is introduced
- [ ] MongoStore, MemoryStore, SQLiteStore and PostgresStore all satisfy the interface, with the var _ Storer assertions updated
- [ ] store_contract_test.go asserts committed=false for mutation.ErrAbort and committed=true for a real write, on every backend
- [ ] Every existing trunk caller of Mutate is updated in the same change so the build is never left broken
- [ ] VERIFY: go build ./... && go test ./internal/repo/game/

## Files

- internal/repo/game/interface.go
- internal/repo/game/mongo.go
- internal/repo/game/memory.go
- internal/repo/game/sqlite.go
- internal/repo/game/postgres.go
- internal/repo/game/store_contract_test.go

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

### 2026-10-01T13:51:33.063Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.


---
id: 082-b854
title: Port user and session backends across memory, SQLite and Postgres
status: pending
priority: P1
type: feature
created: "2026-10-01T13:52:29.872Z"
updated: "2026-10-01T13:52:39.768Z"
dependencies: ["081-7e0a"]
plan: plans/main-divergence-integration.md
plan_step: Step 5
depends_on: ["stories/081-7e0a-pending-P1-port-the-player-slot-model-and-migrate-the-user-ke.md"]
---

# Port user and session backends across memory, SQLite and Postgres

## Problem Statement

Local has Mongo-only user and session stores. origin/main added Storer interfaces plus three backends for each, and then had to fix five real behaviours after shipping them. Porting the stores without those fixes would reintroduce bugs that are already solved upstream.

## Acceptance Criteria

- [ ] user and session declare Storer interfaces with memory, sqlite and postgres implementations alongside mongo
- [ ] store_contract_test.go for both repos runs against all four backends
- [ ] Sessions expire after sessionMaxAge on postgres, sqlite and memory (origin 2527068, 6b8bdd4)
- [ ] Update returns ErrNotFound when no row was updated (origin f5870ec)
- [ ] Moving a session onto a taken user reports repo.ErrDuplicate (origin 33bf874)
- [ ] A user's password survives an Update that omits it (origin 284d617)
- [ ] Sessions remain one-per-user; the unique index on userId is preserved on every backend
- [ ] VERIFY: go test ./internal/repo/user/ ./internal/repo/session/

## Files

- internal/repo/user/
- internal/repo/session/

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


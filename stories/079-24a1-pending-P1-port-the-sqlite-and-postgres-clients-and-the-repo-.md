---
id: 079-24a1
title: Port the SQLite and Postgres clients and the repo primitives
status: pending
priority: P1
type: feature
created: "2026-10-01T13:52:29.870Z"
updated: "2026-10-01T13:52:39.546Z"
dependencies: ["078-ceca"]
plan: plans/main-divergence-integration.md
plan_step: Step 2
depends_on: ["stories/078-ceca-pending-P1-verified-baseline-on-an-integration-branch-off-loc.md"]
---

# Port the SQLite and Postgres clients and the repo primitives

## Problem Statement

Local main has only MongoDB plus an in-memory game store. The trunk-to-be needs the connection helpers, the id helper and the shared sentinel errors from origin/main before any backend can be added, because the contract suites assert on those sentinels.

## Acceptance Criteria

- [ ] internal/clients/sqlite and internal/clients/postgres exist with their schema DDL
- [ ] internal/repo/ids provides the UUID helper with tests
- [ ] internal/repo/errors.go declares ErrNotFound, ErrDuplicate and ErrConflict
- [ ] SQLite uses a single connection so :memory: databases and per-connection pragmas are shared (origin 2579bab)
- [ ] Postgres shares one capped connection pool and closes it on shutdown (origin ec04cc0)
- [ ] VERIFY: go build ./... && go test ./internal/repo/ids/

## Files

- internal/clients/sqlite/
- internal/clients/postgres/
- internal/repo/ids/
- internal/repo/errors.go

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


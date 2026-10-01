---
id: 079-24a1
title: Port the SQLite and Postgres clients and the repo primitives
status: in_progress
priority: P1
type: feature
created: "2026-10-01T13:52:29.870Z"
updated: "2026-10-01T14:17:24.127Z"
dependencies: ["078-ceca"]
plan: plans/main-divergence-integration.md
plan_step: Step 2
depends_on: ["stories/078-ceca-pending-P1-verified-baseline-on-an-integration-branch-off-loc.md"]
started_at: "2026-10-01T14:17:24.126Z"
---

# Port the SQLite and Postgres clients and the repo primitives

## Problem Statement

Local main has only MongoDB plus an in-memory game store. The trunk-to-be needs the connection helpers, the id helper and the shared sentinel errors from origin/main before any backend can be added, because the contract suites assert on those sentinels.

## Acceptance Criteria

- [x] internal/clients/sqlite and internal/clients/postgres exist with their schema DDL
- [x] internal/repo/ids provides the UUID helper with tests
- [x] internal/repo/errors.go declares ErrNotFound, ErrDuplicate and ErrConflict
- [x] SQLite uses a single connection so :memory: databases and per-connection pragmas are shared
- [REJECTED] Postgres shares one capped connection pool and closes it on shutdown (Cannot be proven on this machine: TestOpen_CapsOpenConnections SKIPS without INDRI_TEST_POSTGRES_URI, and there is no Postgres available -- no postgres/psql/pg_ctl binaries, nothing listening on 5432, the Docker daemon is down, and neither local nor remote docker-compose.yml defines a postgres service (remote ran these in CI only, 4216073). The cap itself is implemented and readable (db.SetMaxOpenConns(MaxOpenConns), internal/clients/postgres/client.go:32) but is NOT verified by an executed assertion. The 'closes it on shutdown' half is injector/boot wiring that does not exist until the stores land, so it is out of scope for this story as written.)
- [x] VERIFY: go build ./... && go test ./internal/repo/ids/

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

### 2026-10-01T14:21:56.350Z - Ported faithfully from origin/main: internal/clients/sqlite (client + 3 tests), internal/clients/postgres (client + 2 tests), internal/repo/ids (+2 tests), internal/repo/errors.go. TDD followed: tests committed first and confirmed RED (build failure, undefined: New), then implementations made them GREEN. Deps added: modernc.org/sqlite v1.59.0, github.com/jackc/pgx/v5 v5.11.0; google/uuid v1.6.0 promoted from indirect to direct. Full suite after the port: go vet 0, gofmt clean, go test -race 30 packages ok (was 27), 0 FAIL, no data races -- no regression against the Step 1 baseline. NOTE on criterion 1: the postgres client carries no DDL by upstream design (its package comment says each store runs its own schema DDL), so 'their schema DDL' is satisfied by sqlite only; postgres DDL arrives with the stores in 080.


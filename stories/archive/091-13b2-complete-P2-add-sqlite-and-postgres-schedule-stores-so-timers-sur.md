---
id: 091-13b2
title: Add SQLite and PostgreSQL schedule stores so script timers survive a restart
status: complete
priority: P2
type: feature
created: "2026-10-03T05:18:53.000Z"
updated: "2026-10-04T19:13:16.000Z"
dependencies: ["082-b854"]
---

# Add SQLite and PostgreSQL schedule stores so script timers survive a restart

## Problem Statement

Story 082 made INDRI_DB_BACKEND choose the game, user and session stores, but internal/repo/schedule has only MongoDB and in-memory stores. Boss chose (082): on sqlite and postgres, timers (indri.after) are kept in memory with a boot warning -- lost on restart, not shared between instances. A SQL schedule store removes that gap.

## Acceptance Criteria

- [x] A SQL schedule store (one implementation, both dialects via internal/repo/sqldialect) satisfies schedule.Storer
- [x] Claiming is atomic (a conditional UPDATE), with the same lease fence and failure outcomes the MongoDB store mirrors (claimable, applyClaim, failureOutcome, heldBy)
- [x] storer_contract_test.go runs against it on SQLite and PostgreSQL
- [x] openStores uses it for sqlite and postgres, and the in-memory-timers boot warning is gone for them
- [x] VERIFY: go test -race ./internal/repo/schedule/ ./internal/injector/

## Work Log

### 2026-10-04T19:13:16.000Z - FEATURE. internal/repo/schedule/sql.go: SQLStore over SQLite and PostgreSQL (one implementation, sqldialect). Every entry field is a column, times in Unix ms (mongoTime precision), payload BSON as the game store does. SQLite DDL joins internal/clients/sqlite's schema; PostgreSQL's store creates its own table. Claim: a transaction selects the oldest claimable row (claimable written in SQL) with the new sqldialect ClaimLock (" FOR UPDATE SKIP LOCKED" on PostgreSQL, none on SQLite's single connection), applies applyClaim, and writes back with an UPDATE conditional on the attempts read. Lease-held writes: locked read, heldBy, then the same edits the in-memory store applies -- lifted out of memory.go into shared renewEdit/completeEdit/failEdit/markDeadEdit, so failureOutcome is reused rather than copied. No TTL index in SQL, so Schedule prunes entries OrphanGrace past fire time. openStores uses it for sqlite and postgres; memoryTimers and its boot warning are gone; CLAUDE.md, ARCHITECTURE and .env.example updated.

### 2026-10-04T19:13:16.000Z - VERIFY. Contract suite (15 tests) runs on memory, sqlite, postgres and mongodb with no skips; new TestSQLStore_PrunesEntriesPastTheirOrphanGrace (sqlite, postgres); TestClaimLock in sqldialect; injector test expects scheduleRepo.SQLStore on both. Tests failed to build first, then passed. Concurrent-claim and lease tests -count=20 green on PostgreSQL. Mutations: claim lock removed -> killed (postgres concurrent claim); lock and fence both removed -> killed (double claim); pruning disabled -> killed; heldBy check removed -> killed; live lease ignored -> killed (timeout); terminal states claimable -> killed; cancel deleting non-pending -> killed; idempotent re-schedule answer removed -> killed. Weakening only the attempts fence survives, as expected: the row lock alone serialises the write, and the fence is the backstop the lock-removed mutant shows. go test -race ./... green with INDRI_TEST_POSTGRES_URI set.


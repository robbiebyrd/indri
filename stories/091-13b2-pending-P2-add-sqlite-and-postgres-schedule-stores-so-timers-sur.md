---
id: 091-13b2
title: Add SQLite and PostgreSQL schedule stores so script timers survive a restart
status: pending
priority: P2
type: feature
created: "2026-10-03T05:18:53.000Z"
updated: "2026-10-03T05:18:53.000Z"
dependencies: ["082-b854"]
---

# Add SQLite and PostgreSQL schedule stores so script timers survive a restart

## Problem Statement

Story 082 made INDRI_DB_BACKEND choose the game, user and session stores, but internal/repo/schedule has only MongoDB and in-memory stores. Boss chose (082): on sqlite and postgres, timers (indri.after) are kept in memory with a boot warning -- lost on restart, not shared between instances. A SQL schedule store removes that gap.

## Acceptance Criteria

- [ ] A SQL schedule store (one implementation, both dialects via internal/repo/sqldialect) satisfies schedule.Storer
- [ ] Claiming is atomic (a conditional UPDATE), with the same lease fence and failure outcomes the MongoDB store mirrors (claimable, applyClaim, failureOutcome, heldBy)
- [ ] storer_contract_test.go runs against it on SQLite and PostgreSQL
- [ ] openStores uses it for sqlite and postgres, and the in-memory-timers boot warning is gone for them
- [ ] VERIFY: go test -race ./internal/repo/schedule/ ./internal/injector/

## Work Log

---
id: 092-5cce
title: SQL game stores drop empty data maps, so UpdateField under them fails where MongoDB succeeds
status: pending
priority: P1
type: fix
created: "2026-10-03T05:29:27.000Z"
updated: "2026-10-03T05:29:27.000Z"
dependencies: ["082-b854"]
---

# SQL game stores drop empty data maps, so UpdateField under them fails where MongoDB succeeds

## Problem Statement

Found while writing 082's concurrent-UpdateField test. The SQL game backends (story 080) store the game as encoding/json, and the data stores are tagged json:"data,omitempty", so an empty map is not stored and reads back nil. UpdateField("data.k", v) on such a game then fails on SQLite and PostgreSQL with 'cannot create field "k" in element {data: <nil>}', while MongoDB (BSON, no omitempty, keeps {}) succeeds. internal/services/stage writes through UpdateField under stage.scenes.<id>.data, so a scene declared with empty data becomes unwritable on a SQL backend -- reachable at runtime since 082 added INDRI_DB_BACKEND.

## Acceptance Criteria

- [ ] A contract test: UpdateField under an empty data map (game, stage and scene level) succeeds on every backend and publishes the delta
- [ ] The SQL and Mongo backends agree on whether an empty map survives a write/read round trip
- [ ] VERIFY: go test -race ./internal/repo/game/ ./internal/services/stage/

## Work Log

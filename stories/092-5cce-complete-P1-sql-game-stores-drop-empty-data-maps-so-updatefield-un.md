---
id: 092-5cce
title: SQL game stores drop empty data maps, so UpdateField under them fails where MongoDB succeeds
status: complete
priority: P1
type: fix
created: "2026-10-03T05:29:27.000Z"
updated: "2026-10-03T17:01:43.000Z"
dependencies: ["082-b854"]
---

# SQL game stores drop empty data maps, so UpdateField under them fails where MongoDB succeeds

## Problem Statement

Found while writing 082's concurrent-UpdateField test. The SQL game backends (story 080) store the game as encoding/json, and the data stores are tagged json:"data,omitempty", so an empty map is not stored and reads back nil. UpdateField("data.k", v) on such a game then fails on SQLite and PostgreSQL with 'cannot create field "k" in element {data: <nil>}', while MongoDB (BSON, no omitempty, keeps {}) succeeds. internal/services/stage writes through UpdateField under stage.scenes.<id>.data, so a scene declared with empty data becomes unwritable on a SQL backend -- reachable at runtime since 082 added INDRI_DB_BACKEND.

## Acceptance Criteria

- [x] A contract test: UpdateField under an empty data map (game, stage and scene level) succeeds on every backend and publishes the delta
- [x] The SQL and Mongo backends agree on whether an empty map survives a write/read round trip
- [x] VERIFY: go test -race ./internal/repo/game/ ./internal/services/stage/

## Work Log

### 2026-10-03T17:01:43.000Z - ROOT CAUSE. Two causes, both shown red before the fix. (1) The SQL game stores encoded the document as JSON, whose data stores are omitempty: an empty map was not stored, read back nil, and the shared path editor (which works on the BSON form, mirroring MongoDB) then saw null and refused to descend -- the empty-store test failed on sqlite and postgres only. (2) newGameDoc copied a script's data stores as given, so a script declaring no data gave every backend a null store -- that test failed on all four, MongoDB included. Note: the first run of these tests reported only memory and sqlite because OrbStack had stopped and MongoDB and PostgreSQL skipped; both were restarted (the compose Postgres container had to be recreated, having been created during the earlier port clash without its port binding) and every result here is with all four backends running.

### 2026-10-03T17:01:43.000Z - FIX. The SQL stores keep the game as BSON -- the encoding MongoDB stores and the memory store round-trips through -- in a BLOB (SQLite) / BYTEA (PostgreSQL) column; id, code, version and private stay in their own columns, so nothing queries the document. newGameDoc starts a game's own data, privateData and playerData empty rather than absent, as origin's newGame did. No migration: an existing SQL database with the old JSON/JSONB column must be recreated (no backward compatibility by default).

### 2026-10-03T17:01:43.000Z - VERIFICATION. go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL up, 0 skips. Mutations: restoring JSON encoding fails the empty-store test on sqlite and postgres (and the absent-store test there); dropping the normalisation fails the absent-store test on all four. End-to-end: a server on INDRI_DB_BACKEND=sqlite with the tictactoe config registered, logged in and created a game returned whole, slots and team data included.


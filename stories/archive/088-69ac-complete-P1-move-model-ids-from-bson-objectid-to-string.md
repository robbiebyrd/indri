---
id: 088-69ac
title: Move model IDs from bson.ObjectID to string
status: complete
priority: P1
type: refactor
created: "2026-10-01T14:32:29.318Z"
updated: "2026-10-01T14:45:02.969Z"
dependencies: ["079-24a1"]
plan: plans/main-divergence-integration.md
plan_step: Step 3
depends_on: ["stories/079-24a1-in_progress-P1-port-the-sqlite-and-postgres-clients-and-the-repo-.md"]
started_at: "2026-10-01T14:34:12.440Z"
completed_at: "2026-10-01T14:45:02.969Z"
---

# Move model IDs from bson.ObjectID to string

## Problem Statement

models.Game, models.User and models.Session all declare ID as bson.ObjectID (internal/models/game.go:10, user.go:10, session.go:10). The Storer interfaces already pass ids as string, but the stored type is Mongo-specific, so a SQLite or Postgres store cannot produce one and the ported DDL ('id TEXT PRIMARY KEY', ids.New() returning a UUID) has nothing to write into. origin/main solved this in 0b7a246. This is a cross-cutting prerequisite for every non-Mongo backend and is too large to sit inside the 'add two game backends' story.

## Acceptance Criteria

- [x] models.Game.ID, models.User.ID and models.Session.ID are string, not bson.ObjectID
- [REJECTED] The Mongo stores convert at the driver boundary so existing documents still read and write correctly (NOT implemented, deliberately. This criterion asks for backward compatibility with documents whose _id is an ObjectID, and CLAUDE.md requires explicit approval before implementing ANY backward compatibility. origin/main did not implement it either (0b7a246 stores string _ids outright). The stores now write and read string _ids; a MongoDB database holding games, users or sessions created before this change will not find them, and reads fail with 'decoding an object ID into a string is not supported'. That is acceptable pre-production but is a real behaviour change: a developer with an existing local database must drop it. Say the word if a migration or a dual-read path is wanted and I will add it.)
- [x] New ids come from repo/ids.New(); no code constructs a bson.ObjectID to mint an identifier
- [x] The connection key sessionId still carries session.ID and is still never sent to clients, while the wire field sessionId still carries session.Token
- [x] Every handler that builds or compares an id works on strings, with no ObjectID parsing left outside internal/repo
- [x] Existing game, user and session store tests pass unchanged in behaviour
- [x] VERIFY: go build ./... && go vet ./... && go test -race ./internal/repo/... ./internal/handlers/...
- [x] Full suite shows no regression against .integration/baseline.txt

## Files

- internal/models/game.go
- internal/models/user.go
- internal/models/session.go
- internal/repo/game/
- internal/repo/user/
- internal/repo/session/

## Proof

- [x] [completeness] Completeness (7 of 8 criteria verified by execution; criterion 2 explicitly rejected as a backward-compatibility request that CLAUDE.md forbids without approval.)
- [~] [feature-availability] Feature availability (A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.)
- [x] [robustness] Robustness (Two latent runtime failures the compiler could not catch were found and fixed: the session and user stores still allowed MongoDB to mint an ObjectID _id, which no longer decodes into a string. Caught by TestDelete_InvalidatesToken against real MongoDB, not by the build.)
- [~] [resilience] Resilience (A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.)
- [x] [security] Security (The sessionId overload is preserved: the connection key carries Session.ID （never sent to clients） and the wire field carries Session.Token. Verified by inspecting the only SetKey call site （boot/handlers.go:210） and both login and reconnect handlers.)
- [~] [defense-in-depth] Defense in depth (A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.)
- [~] [input-validation] Input validation (A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.)
- [x] [thread-safety] Thread safety (go test -race -count=1 across all 30 packages with both databases live reports no DATA RACE.)
- [~] [configurability] Configurability (A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.)

## QA

Verified against live MongoDB and Postgres. New id_contract_test.go confirmed RED before the change (ID was bson.ObjectID, an array type) and PASSES after on both the memory and mongodb backends. Full suite with -count=1 so nothing is cached: go build 0, go vet 0, gofmt -l empty, go test -race 30 packages ok, 0 FAIL, no DATA RACE. The sessionId invariant was re-checked by hand: boot/handlers.go:210 sets the connection key from result.Session.ID, while login and reconnect still put session.Token in the wire field -- the two values stay distinct.

## Work Log

### 2026-10-01T14:44:45.283Z - TDD: added internal/repo/game/id_contract_test.go asserting a new game's ID is a UUIDv4 string, round-trips through Get, and is unique -- confirmed RED (ID was an array type) before any change. Changed models.Game/User/Session.ID to string, dropped the mongox autoID tag, added ID to CreateGame/CreateUser/CreateSession so the caller mints it via repo/ids.New(), and removed every ObjectIDFromHex conversion from the game, user and session Mongo stores. Found two latent runtime bugs the compiler could not: the session and user stores still let MongoDB generate an ObjectID _id, which now fails to decode into a string -- caught by TestDelete_InvalidatesToken against real MongoDB. internal/repo/schedule deliberately keeps its own ObjectID; it is Mongo-only and out of scope. Final: build 0, vet 0, gofmt clean, go test -race -count=1 with MongoDB and Postgres live: 30 packages ok, 0 FAIL, no data races. The new ID contract passes on both the memory and mongodb backends.


### 2026-10-01T14:44:59.835Z - Proof completeness set PROVEN: 7 of 8 criteria verified by execution; criterion 2 explicitly rejected as a backward-compatibility request that CLAUDE.md forbids without approval.

### 2026-10-01T14:44:59.914Z - Proof security set PROVEN: The sessionId overload is preserved: the connection key carries Session.ID (never sent to clients) and the wire field carries Session.Token. Verified by inspecting the only SetKey call site (boot/handlers.go:210) and both login and reconnect handlers.

### 2026-10-01T14:44:59.991Z - Proof robustness set PROVEN: Two latent runtime failures the compiler could not catch were found and fixed: the session and user stores still allowed MongoDB to mint an ObjectID _id, which no longer decodes into a string. Caught by TestDelete_InvalidatesToken against real MongoDB, not by the build.

### 2026-10-01T14:45:00.067Z - Proof thread-safety set PROVEN: go test -race -count=1 across all 30 packages with both databases live reports no DATA RACE.

### 2026-10-01T14:45:00.155Z - Proof feature-availability set NOT_APPLICABLE: A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.

### 2026-10-01T14:45:00.235Z - Proof resilience set NOT_APPLICABLE: A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.

### 2026-10-01T14:45:00.315Z - Proof defense-in-depth set NOT_APPLICABLE: A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.

### 2026-10-01T14:45:00.396Z - Proof input-validation set NOT_APPLICABLE: A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.

### 2026-10-01T14:45:00.478Z - Proof configurability set NOT_APPLICABLE: A type change to an existing identifier field. No new feature, failure mode, trust boundary, user input or configuration is introduced.

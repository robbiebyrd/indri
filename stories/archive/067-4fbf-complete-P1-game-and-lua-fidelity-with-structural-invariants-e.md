---
id: 067-4fbf
title: Game and Lua fidelity with structural invariants enforced
status: complete
priority: P1
type: feature
created: "2026-09-12T02:40:57.313Z"
updated: "2026-09-12T03:00:51.306Z"
dependencies: ["048"]
plan: plans/lua-game-scripting.md
plan_step: Step 4
depends_on: []
completed_at: "2026-09-12T03:00:51.306Z"
---

# Game and Lua fidelity with structural invariants enforced

## Problem Statement

models.Game.Version is json-excluded and ID is a BSON ObjectID, so a naive JSON round trip through Lua silently zeroes the CAS fence. A whole-document round trip also lets a script rewrite membership, host flags, the game code and privacy. Separately, Lua has one table type, so an empty slice and an empty map are indistinguishable on the way back, and PlayerIDs, SceneOrder and Players all lack omitempty.

## Acceptance Criteria

- [x] A fully populated models.Game survives the round trip unchanged, including Version, ID, CreatedAt and DeletedAt
- [x] Deleting a key in Lua removes it from the Go map
- [x] A script assigning state.version or state.id cannot change the Go field
- [x] The mutation is rejected when the script changed the player id set, any team PlayerIDs membership, any Host or Connected flag, the game Code, or Private
- [x] Player.Score, every data map and the whole Stage remain freely writable
- [x] A script that promotes itself to host is rejected
- [x] A reflect-guided coercion pass reconciles empty collections before unmarshal, applied only where the target Go type is concrete
- [x] A game with an empty Players map and an empty PlayerIDs slice at the same time survives the round trip with both shapes intact
- [x] Clearing sceneOrder from Lua yields an empty JSON array and clearing players yields an empty JSON object
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/game.go

## Proof

- [x] [completeness] Completeness (10 of 10 criteria; 9 table-driven tests covering fidelity, the empty-collection hole, forgery and the invariant matrix)
- [~] [feature-availability] Feature availability (Conversion layer with no entry point yet; story 052 consumes it inside the Mutate apply closure)
- [x] [robustness] Robustness (Removing the coercion pass, the invariant check, the field restore, or decoding into the existing game instead of a zero value each fails the matching test)
- [~] [resilience] Resilience (Pure in-memory conversion with no I/O and no external dependency that can fail)
- [x] [security] Security (Version and ID are unforgeable from Lua; the invariant check rejects membership, host, connected, code and private rewrites that would desync the game from live sessions)
- [x] [defense-in-depth] Defense in depth (The id key is dropped before decode AND server-owned fields are restored after, so neither path alone is load-bearing)
- [x] [input-validation] Input validation (checkInvariants rejects before any write; the coercion pass is applied only where the target Go type is concrete, so interface targets are never rewritten)
- [~] [thread-safety] Thread safety (Pure functions over caller-owned values with no shared mutable state)
- [~] [configurability] Configurability (No configuration; depth and node limits come from the budget supplied by the caller)

## QA

Verified independently: build, vet and full suite green under -race. Agent proved each guard load-bearing by reverting it in turn. 10/10 criteria. Team membership interpreted as a per-team set comparison, so team rename and playerIds reordering are allowed.

## Work Log

### 2026-09-12T02:58:02.166Z - Implemented Step 4 in internal/services/lua/game.go: gameToLua (events.ToMap + toLua) and applyLua. applyLua decodes into a zero models.Game so an absent key really deletes, drops the script-supplied 'id' key, then restores ID/Version/CreatedAt/DeletedAt from the original so the CAS fence cannot be forged. A reflect-guided coerceEmpty pass walks models.Game's type alongside the decoded document and swaps empty map<->empty slice only where the target Go type is concrete (pointer fields followed; interface{} targets inside *Data maps untouched). checkInvariants rejects the mutation before any write when the player-id set, any Team.PlayerIDs membership, any Host/Connected flag, Code or Private changed; Score, every *Data map and the whole Stage stay writable. 9 new table-driven tests in game_test.go (stdlib testing, no DB). Verified each guard is load-bearing by removing it and watching the matching tests fail. go build ./... && go vet ./... clean; go test -race ./internal/services/lua/ and go test ./... green.


### 2026-09-12T03:00:50.355Z - Proof completeness set PROVEN: 10 of 10 criteria; 9 table-driven tests covering fidelity, the empty-collection hole, forgery and the invariant matrix

### 2026-09-12T03:00:50.426Z - Proof feature-availability set NOT_APPLICABLE: Conversion layer with no entry point yet; story 052 consumes it inside the Mutate apply closure

### 2026-09-12T03:00:50.500Z - Proof robustness set PROVEN: Removing the coercion pass, the invariant check, the field restore, or decoding into the existing game instead of a zero value each fails the matching test

### 2026-09-12T03:00:50.573Z - Proof resilience set NOT_APPLICABLE: Pure in-memory conversion with no I/O and no external dependency that can fail

### 2026-09-12T03:00:50.651Z - Proof security set PROVEN: Version and ID are unforgeable from Lua; the invariant check rejects membership, host, connected, code and private rewrites that would desync the game from live sessions

### 2026-09-12T03:00:50.729Z - Proof defense-in-depth set PROVEN: The id key is dropped before decode AND server-owned fields are restored after, so neither path alone is load-bearing

### 2026-09-12T03:00:50.807Z - Proof input-validation set PROVEN: checkInvariants rejects before any write; the coercion pass is applied only where the target Go type is concrete, so interface targets are never rewritten

### 2026-09-12T03:00:50.884Z - Proof thread-safety set NOT_APPLICABLE: Pure functions over caller-owned values with no shared mutable state

### 2026-09-12T03:00:50.963Z - Proof configurability set NOT_APPLICABLE: No configuration; depth and node limits come from the budget supplied by the caller

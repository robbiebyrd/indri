---
id: 067-4fbf
title: Game and Lua fidelity with structural invariants enforced
status: pending
priority: P1
type: feature
created: "2026-09-12T02:40:57.313Z"
updated: "2026-09-12T02:41:00.887Z"
dependencies: ["048"]
plan: plans/lua-game-scripting.md
plan_step: Step 4
depends_on: []
---

# Game and Lua fidelity with structural invariants enforced

## Problem Statement

models.Game.Version is json-excluded and ID is a BSON ObjectID, so a naive JSON round trip through Lua silently zeroes the CAS fence. A whole-document round trip also lets a script rewrite membership, host flags, the game code and privacy. Separately, Lua has one table type, so an empty slice and an empty map are indistinguishable on the way back, and PlayerIDs, SceneOrder and Players all lack omitempty.

## Acceptance Criteria

- [ ] A fully populated models.Game survives the round trip unchanged, including Version, ID, CreatedAt and DeletedAt
- [ ] Deleting a key in Lua removes it from the Go map
- [ ] A script assigning state.version or state.id cannot change the Go field
- [ ] The mutation is rejected when the script changed the player id set, any team PlayerIDs membership, any Host or Connected flag, the game Code, or Private
- [ ] Player.Score, every data map and the whole Stage remain freely writable
- [ ] A script that promotes itself to host is rejected
- [ ] A reflect-guided coercion pass reconciles empty collections before unmarshal, applied only where the target Go type is concrete
- [ ] A game with an empty Players map and an empty PlayerIDs slice at the same time survives the round trip with both shapes intact
- [ ] Clearing sceneOrder from Lua yields an empty JSON array and clearing players yields an empty JSON object
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/game.go

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


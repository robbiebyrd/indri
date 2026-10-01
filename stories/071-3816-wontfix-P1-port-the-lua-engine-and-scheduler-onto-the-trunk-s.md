---
id: 071-3816
title: Port the Lua engine and scheduler onto the trunk storage layer
status: wontfix
priority: P1
type: feature
created: "2026-10-01T13:29:44.858Z"
updated: "2026-10-01T13:51:33.344Z"
dependencies: ["070-3375"]
plan: plans/main-divergence-integration.md
plan_step: Step 4
depends_on: ["stories/070-3375-pending-P1-add-mutateresult-and-ctx-to-the-game-storer-across.md"]
completed_at: "2026-10-01T13:51:33.343Z"
---

# Port the Lua engine and scheduler onto the trunk storage layer

## Problem Statement

The server-side Lua engine is local main's largest unique asset at 80 files and 24,554 lines. It is already hexagonal, reaching the store only through the one-method GameMutator port and deliberately not importing the repo, so it ports against the trunk's Storer by wiring rather than rewriting. Without it the trunk has no scripting layer at all.

## Acceptance Criteria

- [ ] internal/services/lua, internal/repo/schedule, internal/services/scheduler, internal/handlers/actions/script and cmd/indri-script are present on the integration branch
- [ ] The engine is wired to the trunk store via GameMutator with no import of internal/repo from internal/services/lua
- [ ] effects.go (the effect ledger) is landed with the engine, not deferred
- [ ] Per-script grants (assets, http) are enforced and an unrecognised grant still fails boot
- [ ] The Lua sandbox hardening is intact: debug library never opened, host table frozen
- [ ] Scheduler lands with mongo and memory backends; sqlite and postgres backends are filed as a follow-up story rather than stubbed
- [ ] VERIFY: go test -race ./internal/services/lua/... ./internal/services/scheduler/... ./internal/repo/schedule/...

## Files

- internal/services/lua/
- internal/repo/schedule/
- internal/services/scheduler/
- internal/handlers/actions/script/
- cmd/indri-script/

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

### 2026-10-01T13:40:42.584Z - BLOCKER FOUND: origin/main never received the connection-independent dispatch refactor. Its actions.MessageHandler is still Handle(s transport.Conn, decodedMsg map[string]interface{}) error (verified: origin/main:internal/handlers/actions/handler.go). Local refactored to Handle(actions.Request) (actions.Result, error) in d03e6b0, its first post-base commit, 16 files +366/-393. 20 trunk files are conn-coupled; 50 local files depend on actions.Request. The Lua engine cannot port until the trunk has the refactor. New prerequisite step needed.

### 2026-10-01T13:51:33.265Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.


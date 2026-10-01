---
id: 085-82ef
title: Harvest the luahandler behaviours into the Lua engine
status: pending
priority: P2
type: feature
created: "2026-10-01T13:52:35.317Z"
updated: "2026-10-01T13:52:40.001Z"
dependencies: ["081-7e0a"]
plan: plans/main-divergence-integration.md
plan_step: Step 8
depends_on: ["stories/081-7e0a-pending-P1-port-the-player-slot-model-and-migrate-the-user-ke.md"]
---

# Harvest the luahandler behaviours into the Lua engine

## Problem Statement

origin/main's luahandler provides indri.refresh(), indri.refreshSelf(), post-Mutate execution ordering and an inquire notification path. Its registration model puts scripts on built-in action names, which local's engine forbids by design. The behaviours are worth having; the registration model is not.

## Acceptance Criteria

- [ ] indri.refresh() and indri.refreshSelf() exist in internal/services/lua with tests covering post-Mutate execution ordering
- [ ] The inquire notification behaviour is reachable through indri.before/indri.after_action and proven by a test
- [ ] A script hooking join, leave, kick and inquire works through the hook API
- [ ] login, logout, reconnect and register remain unhookable and a script attempting it is refused
- [ ] A script still may not claim a built-in action name or a dispatch phase (received, processed)
- [ ] No code is copied from luahandler; only behaviours are reimplemented against the engine's model
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/hooks.go
- internal/services/lua/lifecycle.go

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


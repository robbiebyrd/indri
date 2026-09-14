---
id: 084-01a8
title: "scene:changed never fires, because stage.Service is constructed nowhere"
status: ready
priority: P3
type: feature
created: "2026-09-14T00:16:38.720Z"
updated: "2026-09-14T00:46:32.454Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 19
---

# scene:changed never fires, because stage.Service is constructed nowhere

## Problem Statement

Story 063 emits scene:changed from stage.Service.SetCurrentScene, which is the Go call site the plan named - but stage.Service has no injector wiring at all and is constructed nowhere, so that path never runs. The event is implemented and tested but unreachable in production. The live way currentScene moves today is a scripts own indri.mutate setting state.stage.currentScene. Hooking that means diffing inside applyThroughLua and queueing a lifecycle effect on the ledger, which is clean but is a new recursion surface: a scene:changed handler can change the scene again, so it needs a depth cap. 063 deliberately did not do it rather than silently widening scope.

## Acceptance Criteria

- [ ] scene:changed fires when a script changes stage.currentScene through indri.mutate
- [ ] The emission is queued on the ledger like every other deferred effect, so a losing mutate attempt does not raise it
- [ ] A scene:changed handler that changes the scene again is bounded, and the bound is asserted rather than incidental
- [ ] Either stage.Service is wired into the injector or its SetCurrentScene emission is removed, so there are not two paths one of which is dead
- [ ] VERIFY: go test -race ./internal/services/lua/ ./internal/services/game/

## Files

- internal/services/lua/host_mutate.go
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


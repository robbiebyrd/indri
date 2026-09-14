---
id: 084-01a8
title: "scene:changed never fires, because stage.Service is constructed nowhere"
status: complete
priority: P3
type: feature
created: "2026-09-14T00:16:38.720Z"
updated: "2026-09-14T01:04:58.723Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 19
completed_at: "2026-09-14T01:04:58.723Z"
---

# scene:changed never fires, because stage.Service is constructed nowhere

## Problem Statement

Story 063 emits scene:changed from stage.Service.SetCurrentScene, which is the Go call site the plan named - but stage.Service has no injector wiring at all and is constructed nowhere, so that path never runs. The event is implemented and tested but unreachable in production. The live way currentScene moves today is a scripts own indri.mutate setting state.stage.currentScene. Hooking that means diffing inside applyThroughLua and queueing a lifecycle effect on the ledger, which is clean but is a new recursion surface: a scene:changed handler can change the scene again, so it needs a depth cap. 063 deliberately did not do it rather than silently widening scope.

## Acceptance Criteria

- [x] scene:changed fires when a script changes stage.currentScene through indri.mutate
- [x] The emission is queued on the ledger like every other deferred effect, so a losing mutate attempt does not raise it
- [x] A scene:changed handler that changes the scene again is bounded, and the bound is asserted rather than incidental
- [x] Either stage.Service is wired into the injector or its SetCurrentScene emission is removed, so there are not two paths one of which is dead
- [x] VERIFY: go test -race ./internal/services/lua/ ./internal/services/game/

## Files

- internal/services/lua/host_mutate.go
- internal/services/lua/lifecycle.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; the event now fires where the scene actually moves, is queued on the ledger, is bounded, and the dead path is removed rather than wired)
- [x] [feature-availability] Feature availability (scene:changed now fires from a script's own indri.mutate, which is the only way currentScene moves in production)
- [x] [robustness] Robustness (Three mutations confirm the tests can fail: dropping withSceneDepth fails all four derivation cases in about 30ms, disabling the cap fails those plus the cap test, and queueing on the committed level instead of the attempt level fails the losing-attempt test)
- [x] [resilience] Resilience (The effect is queued at the ledger's attempt level, so a scene change in a losing CAS attempt is discarded with it - proved through a real MemoryStore with a fence loser committing between the callback and the save)
- [~] [security] Security (No authorisation or input surface; this bounds a recursion rather than gating access)
- [x] [defense-in-depth] Defense in depth (The write is refused at the cap rather than the event dropped, because letting the last move land while telling nobody leaves the game on a scene no subscriber heard about)
- [x] [input-validation] Input validation (The change is detected by comparing the same two document views the delta is diffed on, so subscriber and broadcast describe the same move by construction and a callback assigning twice is one event)
- [x] [thread-safety] Thread safety (Suite green under -race with a second agent editing the same package throughout)
- [x] [configurability] Configurability (The bound is a named constant with its reasoning, and the test states explicitly that neither the shared deadline nor maxPendingLifecycle bounds this chain - the depth is the only bound)

## QA

Verified independently: build, vet, gofmt and full suite green under -race. Confirmed the stage emission is gone so there is one path not two, the depth bound exists in both host_mutate and lifecycle, and the effect is queued at the attempt level. Followed 071's lesson: asserts the derivation of depth into the handler, not that a chain terminates. 5/5.

## Work Log

### 2026-09-14T01:01:42.751Z - Chose to remove the dead emission and raise scene:changed where the scene actually moves. internal/services/stage is imported by zero Go files and no action, route or handler calls SetCurrentScene, so wiring stage.Service into the injector would have constructed a service nothing calls - the call site would still be dead. The emission, its Lifecycle field and the luaService import are gone from stage.go, replaced by a comment recording why, and the three stage_test cases that tested an unreachable path are gone with them.

The event now comes from invocation.queueSceneChange in internal/services/lua/host_mutate.go, called at the end of applyThroughLua. It compares stage.currentScene across the two document views the delta is already diffed on, so what a subscriber is told and what the broadcast says are the same move by construction, and one callback that assigns twice is one change and one event. It is skipped entirely when no script subscribed.

The emission is a sceneChangedEffect on the ledger (internal/services/lua/lifecycle.go), queued at the attempt level like every other deferred effect, so an attempt that loses the version fence takes its scene change with it. Proved against a real game.MemoryStore through a fenceLoser that commits an ordinary UpdateField between the script callback and the store save: the script picks a different scene per attempt, and the subscriber is told only about the one that was stored.

The recursion bound is a depth carried on lifecycleEvent, installed into the handling invocation's context by emit (withSceneDepth) and read back by queueSceneChange, which queues the next link at depth+1 and refuses the write at maxSceneChangeDepth (10). Refusing the write, not just the event, because letting the last move through while telling nobody would leave the game on a scene no subscriber heard about. The deadline does not bound this chain - emit gives every event a fresh lifecycleTimeout on purpose - and maxPendingLifecycle does not either, since an A->B->A ping-pong never grows the queue past one.

Following story 071, the bound is asserted rather than watched: a white-box capability probe reports, from inside each running handler, the scene depth of its invocation and the depth of the scene change it queued, and TestSceneChanged_CarriesTheChainDepthIntoTheHandler asserts every link from four starting depths. Three mutations were run to prove the tests can fail: dropping withSceneDepth in emit fails all four cases in ~30ms; removing the cap check fails those plus TestSceneChanged_RefusesTheMoveAtTheChainCap; queueing the effect off the attempt level fails the losing-attempt test. The chain script carries a valve at four times the cap so a broken bound fails on the marks instead of hanging.

Verified: go build ./... && go vet ./... && gofmt -l internal/ && go test -race ./... all clean, including go test -race -count=1 ./internal/services/lua/ ./internal/services/game/.


### 2026-09-14T01:04:53.469Z - Proof completeness set PROVEN: 5 of 5 criteria; the event now fires where the scene actually moves, is queued on the ledger, is bounded, and the dead path is removed rather than wired

### 2026-09-14T01:04:53.640Z - Proof feature-availability set PROVEN: scene:changed now fires from a script's own indri.mutate, which is the only way currentScene moves in production

### 2026-09-14T01:04:53.790Z - Proof robustness set PROVEN: Three mutations confirm the tests can fail: dropping withSceneDepth fails all four derivation cases in about 30ms, disabling the cap fails those plus the cap test, and queueing on the committed level instead of the attempt level fails the losing-attempt test

### 2026-09-14T01:04:53.972Z - Proof resilience set PROVEN: The effect is queued at the ledger's attempt level, so a scene change in a losing CAS attempt is discarded with it - proved through a real MemoryStore with a fence loser committing between the callback and the save

### 2026-09-14T01:04:54.079Z - Proof security set NOT_APPLICABLE: No authorisation or input surface; this bounds a recursion rather than gating access

### 2026-09-14T01:04:54.207Z - Proof defense-in-depth set PROVEN: The write is refused at the cap rather than the event dropped, because letting the last move land while telling nobody leaves the game on a scene no subscriber heard about

### 2026-09-14T01:04:54.356Z - Proof input-validation set PROVEN: The change is detected by comparing the same two document views the delta is diffed on, so subscriber and broadcast describe the same move by construction and a callback assigning twice is one event

### 2026-09-14T01:04:54.526Z - Proof thread-safety set PROVEN: Suite green under -race with a second agent editing the same package throughout

### 2026-09-14T01:04:54.699Z - Proof configurability set PROVEN: The bound is a named constant with its reasoning, and the test states explicitly that neither the shared deadline nor maxPendingLifecycle bounds this chain - the depth is the only bound

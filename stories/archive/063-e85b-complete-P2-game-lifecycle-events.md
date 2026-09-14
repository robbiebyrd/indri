---
id: 063-e85b
title: Game lifecycle events
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:39.044Z"
updated: "2026-09-14T00:16:42.777Z"
dependencies: ["057"]
plan: plans/lua-game-scripting.md
plan_step: Step 19
depends_on: ["stories/057-1deb-pending-P2-indri-after-indri-at-and-indri-cancel.md"]
completed_at: "2026-09-14T00:16:42.777Z"
---

# Game lifecycle events

## Problem Statement

Step 14 gates the http capability to scheduled and lifecycle scripts, but no lifecycle event exists anywhere in the plan or the repo. models.Game has no status or ended field at all, so this is a dangling reference that must be closed.

## Acceptance Criteria

- [x] Scripts can subscribe to game created, player joined, player left and scene changed
- [x] Each event carries the game id and the relevant subject
- [x] Events are emitted from the existing Go call sites through the deferred queue and never inline
- [x] Events are emitted only after the originating mutation commits, so a handler never observes uncommitted state
- [x] A lifecycle handler can call indri.mutate
- [x] A lifecycle error does not fail the originating built-in action
- [x] Lifecycle events carry no session, matching the timer contract
- [x] An unknown lifecycle name is refused at load
- [x] VERIFY: go test -race ./internal/services/lua/

## Files

- internal/services/lua/lifecycle.go
- internal/services/game/game.go

## Proof

- [x] [completeness] Completeness (9 of 9 criteria; four events, deferred drain, after-commit ordering, error isolation and load-time refusal)
- [x] [feature-availability] Feature availability (game:created, player:joined and player:left fire from live call sites; scene:changed is implemented and tested but its call site is unreachable, filed separately rather than claimed working)
- [x] [robustness] Robustness (A pre-existing production race was found and fixed: goaway.Censor builds its package-global detector lazily without a lock, so two concurrent joins race on that write - reported by -race the moment two AddPlayer calls overlapped)
- [x] [resilience] Resilience (EmitLifecycle returns nothing at all, so a script bug is structurally incapable of failing join, create or leave rather than merely being caught)
- [x] [security] Security (Lifecycle names never enter Engine.Actions（）, so the router, the REST route table and the scheduler's dispatchable set cannot reach one; event payloads write the host-owned gameId last so a subject cannot forge it)
- [x] [defense-in-depth] Defense in depth (The two kinds read different registries, so an action can never resolve to a lifecycle handler - structural rather than a check someone has to remember)
- [x] [input-validation] Input validation (An unknown lifecycle name fails at load naming the four events, an action name may not contain a colon, and agreesWith compares both manifests so a non-deterministic subscription is a boot failure)
- [x] [thread-safety] Thread safety (Whichever goroutine finds the queue idle owns the drain and runs it to exhaustion, with no lock held across a Lua call; a re-entrant raise joins that drain rather than nesting)
- [x] [configurability] Configurability (067-a0a4 now adds a hostView method to the trigger interface and implements it per kind, rather than growing a refuseInAction condition - which is why the shape was chosen)

## QA

Verified independently: build, vet, gofmt and full suite green under -race, no data races. Confirmed the trigger interface is a closed per-kind type rather than a flag, that the colon keeps lifecycle names out of Engine.Actions() and therefore out of the router and scheduler, and that the goaway race fix is real. 9/9.

## Work Log

### 2026-09-14T00:15:22.625Z - Lifecycle events land as a distinct kind of invocation, not a flag. internal/services/lua/lifecycle.go adds a closed 'trigger' interface with two implementations — actionTrigger and lifecycleTrigger — each answering three questions per kind: which registry holds the handler (h.fns vs h.lifecycle), what to call the call in a message, and who is owed a script failure (a WSError frame for an action; a returned error the drain logs for an event). Invoke and emit both go through one Engine.run(ctx, trigger, session, gameID, arg), and invocation carries the trigger value, so 067-a0a4 adds a hostView method to the interface rather than growing a boolean condition. The colon marks the namespace: indri.on("player:quit") fails at load naming the four events, and a lifecycle name never reaches Engine.Actions(), so the router, the REST table and the scheduler cannot dispatch one. Emission is deferred through lifecycleQueue: whichever goroutine finds it idle owns the drain and runs it to exhaustion, so nothing dispatches inline and a re-entrant raise joins the running drain. EmitLifecycle returns nothing at all, which is what makes a script bug structurally unable to fail join/create/leave. Commit is threaded, never inferred: new core.AddPlayerResult and core.RemovePlayerResult report through MutateResult, and RemovePlayerResult also requires the player to have been present in the winning attempt, so a leave racing a kick raises player:left once. game:created comes after the insert, player:joined/left from GameService, scene:changed from stage.SetCurrentScene. Tests are stdlib and table-driven over a real game.MemoryStore (real lock, fence, retry, publish): after-commit ordering is proved by a recorder that reads the game back at the instant it is told, and error isolation by a real engine whose subscriber raises on every event while ConnectPlayer still succeeds and the player stays in the game. Found and fixed a pre-existing production race: goaway.Censor lazily builds its detector without a lock, so two concurrent joins raced; repo/game now holds a detector built at package init. Gap reported, not hidden: stage.Service is constructed nowhere in the injector, so scene:changed is emitted and tested but unreachable in production until that service is wired; the live way currentScene moves today is a script's own indri.mutate, which this story does not hook.


### 2026-09-14T00:16:38.897Z - Proof completeness set PROVEN: 9 of 9 criteria; four events, deferred drain, after-commit ordering, error isolation and load-time refusal

### 2026-09-14T00:16:38.972Z - Proof feature-availability set PROVEN: game:created, player:joined and player:left fire from live call sites; scene:changed is implemented and tested but its call site is unreachable, filed separately rather than claimed working

### 2026-09-14T00:16:39.050Z - Proof robustness set PROVEN: A pre-existing production race was found and fixed: goaway.Censor builds its package-global detector lazily without a lock, so two concurrent joins race on that write - reported by -race the moment two AddPlayer calls overlapped

### 2026-09-14T00:16:39.133Z - Proof resilience set PROVEN: EmitLifecycle returns nothing at all, so a script bug is structurally incapable of failing join, create or leave rather than merely being caught

### 2026-09-14T00:16:39.215Z - Proof security set PROVEN: Lifecycle names never enter Engine.Actions(), so the router, the REST route table and the scheduler's dispatchable set cannot reach one; event payloads write the host-owned gameId last so a subject cannot forge it

### 2026-09-14T00:16:39.295Z - Proof defense-in-depth set PROVEN: The two kinds read different registries, so an action can never resolve to a lifecycle handler - structural rather than a check someone has to remember

### 2026-09-14T00:16:39.374Z - Proof input-validation set PROVEN: An unknown lifecycle name fails at load naming the four events, an action name may not contain a colon, and agreesWith compares both manifests so a non-deterministic subscription is a boot failure

### 2026-09-14T00:16:39.454Z - Proof thread-safety set PROVEN: Whichever goroutine finds the queue idle owns the drain and runs it to exhaustion, with no lock held across a Lua call; a re-entrant raise joins that drain rather than nesting

### 2026-09-14T00:16:39.531Z - Proof configurability set PROVEN: 067-a0a4 now adds a hostView method to the trigger interface and implements it per kind, rather than growing a refuseInAction condition - which is why the shape was chosen

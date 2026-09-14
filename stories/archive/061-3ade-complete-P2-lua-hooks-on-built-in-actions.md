---
id: 061-3ade
title: Lua hooks on built-in actions
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:39.039Z"
updated: "2026-09-14T01:20:57.933Z"
dependencies: ["060", "045"]
plan: plans/lua-game-scripting.md
plan_step: Step 16
depends_on: ["stories/060-905f-pending-P2-port-tic-tac-toe-to-lua-and-delete-its-go-handlers.md", "stories/045-795e-pending-P1-thread-context-and-action-name-through-dispatch-mu.md"]
started_at: "2026-09-14T00:48:25.586Z"
completed_at: "2026-09-14T01:20:57.933Z"
---

# Lua hooks on built-in actions

## Problem Statement

Built-in actions stay in Go, so scripts extend them through the router received and processed phases. But received runs for every message including pre-auth login, whose payload carries a plaintext password, and Dispatch returns early on error so processed is not an unconditional after phase.

## Acceptance Criteria

- [x] before and after-success hooks fire around the Go handler for a given action
- [x] Hooks on login, register, reconnect and logout are refused at load time
- [x] The payload is stripped for any hook firing on an unauthenticated request
- [x] The two hook kinds are named honestly as before and after-success, since a failed action never reaches the processed phase
- [x] Hooks on an unknown action name are refused at load
- [x] The no-hook path adds no measurable latency, because received runs for every inbound message
- [x] Before-hooks are documented as validation-only, since a before-hook mutation commits independently of an action that later fails
- [x] VERIFY: go vet ./... && go test ./...

## Files

- internal/services/lua/hooks.go

## Proof

- [x] [completeness] Completeness (All criteria met; before and after hooks, per-action filtering, error aborting the chain and state mutation through indri.mutate all exist with tests, green across 27 packages)
- [x] [feature-availability] Feature availability (A before-hook runs before the Go handler and an after-hook after it, asserted by ordering rather than presence; swapping the phases fails the test)
- [x] [robustness] Robustness (Every 'a hook ran' assertion has a paired negative: six other actions are dispatched with no Go handler registered, so any frame returned is a hook that should not have fired, plus a control proving the registry was not simply empty)
- [x] [resilience] Resilience (A hook error aborts the phase chain and returns both a frame and an error, because the WebSocket path only logs the error while the frame is what the player is told)
- [x] [security] Security (A hook cannot replace or suppress a built-in, cannot acquire a dispatchable name since every inbound surface is built from Actions（）, and cannot re-bind identity because invokeHook clears Result.Session unconditionally. Credential actions are not hookable at all)
- [x] [defense-in-depth] Defense in depth (A hook runs on the action host view. I mutated hostView to return the full view myself: both view tests fail, one asserting the capability is absent and one walking globals, package.loaded, preload and metatables to prove it is unreachable by any route)
- [x] [input-validation] Input validation (The payload is stripped for an unauthenticated caller because received runs before anything has authorized anyone; Request is taken by value so the dispatcher's copy is untouched)
- [x] [thread-safety] Thread safety (The hook manifest is mutex-guarded, settled by the first pooled state and every later state held to it, so a script whose registrations vary between states fails the build rather than serving some connections and not others. go test -race green)
- [~] [configurability] Configurability (Which actions are hookable is an allow-list fixed in code, deliberately: making it configurable would let a deployment open the credential actions it exists to close)

## QA

None — covered by tests in internal/services/lua, internal/handlers/actions/script and internal/services/boot

## Work Log

### 2026-09-14T01:20:56.342Z - Implemented indri.before and indri.after_action over the existing received/processed phases rather than a new registry and deliberately not a registration under the action's own name: registering under the name cannot produce a before-hook at all, since the handler registry is a flat ordered slice and a hook would precede the built-in only by being registered earlier, and it would be the smuggling vector additive registration makes possible. The per-action filter lives in Go, so a miss costs no pooled state and no Lua, and a kind nothing hooked is not registered at all. Hooks are a third registry separate from actions and lifecycle; Engine.Actions() reports only actions, and every inbound surface is built from Actions(), so a hook can never acquire a dispatchable name. Hookable is an allow-list of the built-ins minus login, logout, reconnect and register, so a typo fails at boot. A hook runs on the action host view, not the full one, because an after-success hook still runs inside the same dispatch before the caller is answered and is therefore no freer than a before hook: the http capability 067-a0a4 keeps out of the request path stays out of a hook. The scoping moved from registration time into the bind step, reversing the first design after 067-a0a4 reshaped trigger mid-flight. The old code read the scoped table out of the indri global during load, which worked only because loadChunk installs it there while a chunk runs, and under two views it silently picked the full one: a trick that was merely clever became wrong when the thing it read changed shape, with no compile error at the point of the mistake. The agent ran 17 mutants with no survivors and flagged one of its own as faulty, a deleted call that failed to compile and was counted as caught, redoing it as a no-op body. I independently mutated hookTrigger.hostView to return the full view and confirmed both view tests fail. Verified after rebasing onto a main that moved four times during the work: gofmt clean, build and vet clean, go test -race -count=1 ./... green across 27 packages.


### 2026-09-14T01:20:57.082Z - Proof completeness set PROVEN: All criteria met; before and after hooks, per-action filtering, error aborting the chain and state mutation through indri.mutate all exist with tests, green across 27 packages

### 2026-09-14T01:20:57.271Z - Proof feature-availability set PROVEN: A before-hook runs before the Go handler and an after-hook after it, asserted by ordering rather than presence; swapping the phases fails the test

### 2026-09-14T01:20:57.346Z - Proof robustness set PROVEN: Every 'a hook ran' assertion has a paired negative: six other actions are dispatched with no Go handler registered, so any frame returned is a hook that should not have fired, plus a control proving the registry was not simply empty

### 2026-09-14T01:20:57.414Z - Proof resilience set PROVEN: A hook error aborts the phase chain and returns both a frame and an error, because the WebSocket path only logs the error while the frame is what the player is told

### 2026-09-14T01:20:57.484Z - Proof security set PROVEN: A hook cannot replace or suppress a built-in, cannot acquire a dispatchable name since every inbound surface is built from Actions(), and cannot re-bind identity because invokeHook clears Result.Session unconditionally. Credential actions are not hookable at all

### 2026-09-14T01:20:57.558Z - Proof defense-in-depth set PROVEN: A hook runs on the action host view. I mutated hostView to return the full view myself: both view tests fail, one asserting the capability is absent and one walking globals, package.loaded, preload and metatables to prove it is unreachable by any route

### 2026-09-14T01:20:57.630Z - Proof input-validation set PROVEN: The payload is stripped for an unauthenticated caller because received runs before anything has authorized anyone; Request is taken by value so the dispatcher's copy is untouched

### 2026-09-14T01:20:57.707Z - Proof thread-safety set PROVEN: The hook manifest is mutex-guarded, settled by the first pooled state and every later state held to it, so a script whose registrations vary between states fails the build rather than serving some connections and not others. go test -race green

### 2026-09-14T01:20:57.783Z - Proof configurability set NOT_APPLICABLE: Which actions are hookable is an allow-list fixed in code, deliberately: making it configurable would let a deployment open the credential actions it exists to close

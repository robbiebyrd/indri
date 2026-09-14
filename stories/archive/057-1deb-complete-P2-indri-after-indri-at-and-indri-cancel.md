---
id: 057-1deb
title: indri.after, indri.at and indri.cancel
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.572Z"
updated: "2026-09-13T23:54:48.897Z"
dependencies: ["054", "056"]
plan: plans/lua-game-scripting.md
plan_step: Step 12
depends_on: ["stories/054-5cca-pending-P2-indri-reply-and-indri-send-with-depth-and-fan-out-.md", "stories/056-f2b1-pending-P2-durable-schedule-store-for-deferred-events.md"]
started_at: "2026-09-13T23:21:48.663Z"
completed_at: "2026-09-13T23:54:48.896Z"
---

# indri.after, indri.at and indri.cancel

## Problem Statement

Countdown and wall-clock triggers collapse to one absolute fire time. A fired timer has no session, but every built-in Go handler rejects a nil session, and a hook that dereferences the session would fault on a timer fire.

## Acceptance Criteria

- [x] A scheduled action dispatches through router.Dispatch after its fire time with its payload
- [x] indri.after stamps the current game id so the fired request exposes a game id even with no session
- [x] cancel before the fire time prevents dispatch
- [x] A handler error marks the entry and does not hot-loop
- [x] A scheduled action name must be a script action, never a built-in
- [x] A hook that dereferences the session does not fault on a timer fire
- [x] The scheduler runs as a third goroutine in the boot errgroup and shuts down on context cancellation
- [x] State is read at fire time through indri.mutate rather than from a snapshot frozen at schedule time
- [x] VERIFY: go test -race ./internal/services/scheduler/

## Files

- internal/services/scheduler/scheduler.go
- internal/services/boot/serve.go

## Proof

- [x] [completeness] Completeness (All 9 criteria met; host functions, scheduler, errgroup goroutine and both enforcement points exist with tests, and go test -race is green across 25 packages)
- [x] [feature-availability] Feature availability (A scheduled action dispatches through router.Dispatch after its fire time with its payload, driven by an injected clock rather than a real sleep, so the test is deterministic)
- [x] [robustness] Robustness (A handler error goes through the store's Fail with backoff pushing fireAt out and attempts exhausting into StateDead; the scheduler adds a per-tick budget so a backlog drains across ticks rather than hot-looping. The no-backoff mutant is killed by a fixed-now 100-tick phase)
- [x] [resilience] Resilience (A recover around the dispatch on top of the router's own, so a hook dereferencing the nil session on a timer fire costs one timer rather than the scheduler goroutine; the removed-recover mutant is killed)
- [x] [security] Security (Cancel is scoped to the calling game: I mutated the guard myself and TestCancelTimer_RefusesATimerBelongingToAnotherGame fails. The game id is stamped from the invocation, never read from the payload, so it cannot be forged by a client)
- [x] [defense-in-depth] Defense in depth (A built-in action name is refused at schedule time and again at fire time against Engine.Actions（）, because an entry outlives the script that wrote it; the second check dead-letters rather than retrying)
- [x] [input-validation] Input validation (indri.at rejects an unparseable timestamp, maxTimerDelay caps a delay at a year and maxTimerOps caps a single invocation at 32 timer operations, since queueing is cheap but the cost lands later on a shared collection)
- [x] [thread-safety] Thread safety (The scheduler shuts down on context cancellation with an in-loop ctx.Err（） check; serve_test.go covers a scheduler built but never run. go test -race green tree-wide)
- [x] [configurability] Configurability (Tick interval, per-tick budget and lease renewal are Config fields rather than constants, so a deployment can tune them without a rebuild)

## QA

None — covered by tests in internal/services/scheduler, internal/services/lua and internal/services/boot

## Work Log

### 2026-09-13T17:18:25.180Z - CI now fails on any test that skips without a database (repo-wide guard in ci.yml, KNOWN_SKIPS allowlist). The scheduler package inherits this automatically - write its tests DB-free against schedule.MemoryStore or CI will reject them.

### 2026-09-13T23:54:37.246Z - Implemented indri.after/at/cancel over the Step 8 ledger and a claim-and-dispatch scheduler running as a third goroutine in boot.Serve's errgroup. A timer set inside a losing indri.mutate attempt is discarded with it. The plan's example carries no game id but a timer fires with no session, so after/at stamp the caller's game id and it reaches the fired request through the context rather than the payload; session wins where there is one, so a stamped value can only fill a gap. The plan gives no way for cancel to work across invocations at all: scheduling must be deferred to the ledger, so the store cannot assign the id, so a handle stored in game state and read back is meaningless. Timer ids are host-minted and returned synchronously while the entry is written at flush. 'Script action never a built-in' is enforced at schedule time against the state manifest and again at fire time against Engine.Actions(), because an entry outlives the script that wrote it; a failure there is dead-lettered, not retried. Cancel is scoped to the calling game since an ObjectID is not a secret. Agent ran 39 mutants, 38 killed; the survivor is the injector wiring line, untestable without a live Mongo pool, and its failure mode is loud and covered. I independently mutated the cross-game cancel guard (fails TestCancelTimer_RefusesATimerBelongingToAnotherGame) and confirmed the nil-scheduler guard is tested. Verified after rebasing onto a main that moved twice mid-verification: gofmt clean, build and vet clean, go test -race -count=1 ./... green across 25 packages.


### 2026-09-13T23:54:38.029Z - Proof completeness set PROVEN: All 9 criteria met; host functions, scheduler, errgroup goroutine and both enforcement points exist with tests, and go test -race is green across 25 packages

### 2026-09-13T23:54:38.105Z - Proof feature-availability set PROVEN: A scheduled action dispatches through router.Dispatch after its fire time with its payload, driven by an injected clock rather than a real sleep, so the test is deterministic

### 2026-09-13T23:54:38.184Z - Proof robustness set PROVEN: A handler error goes through the store's Fail with backoff pushing fireAt out and attempts exhausting into StateDead; the scheduler adds a per-tick budget so a backlog drains across ticks rather than hot-looping. The no-backoff mutant is killed by a fixed-now 100-tick phase

### 2026-09-13T23:54:38.265Z - Proof resilience set PROVEN: A recover around the dispatch on top of the router's own, so a hook dereferencing the nil session on a timer fire costs one timer rather than the scheduler goroutine; the removed-recover mutant is killed

### 2026-09-13T23:54:38.349Z - Proof security set PROVEN: Cancel is scoped to the calling game: I mutated the guard myself and TestCancelTimer_RefusesATimerBelongingToAnotherGame fails. The game id is stamped from the invocation, never read from the payload, so it cannot be forged by a client

### 2026-09-13T23:54:38.431Z - Proof defense-in-depth set PROVEN: A built-in action name is refused at schedule time and again at fire time against Engine.Actions(), because an entry outlives the script that wrote it; the second check dead-letters rather than retrying

### 2026-09-13T23:54:38.512Z - Proof input-validation set PROVEN: indri.at rejects an unparseable timestamp, maxTimerDelay caps a delay at a year and maxTimerOps caps a single invocation at 32 timer operations, since queueing is cheap but the cost lands later on a shared collection

### 2026-09-13T23:54:38.589Z - Proof thread-safety set PROVEN: The scheduler shuts down on context cancellation with an in-loop ctx.Err() check; serve_test.go covers a scheduler built but never run. go test -race green tree-wide

### 2026-09-13T23:54:38.679Z - Proof configurability set PROVEN: Tick interval, per-tick budget and lease renewal are Config fields rather than constants, so a deployment can tune them without a rebuild

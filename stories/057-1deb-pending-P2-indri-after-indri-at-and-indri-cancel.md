---
id: 057-1deb
title: indri.after, indri.at and indri.cancel
status: pending
priority: P2
type: feature
created: "2026-09-12T01:28:12.572Z"
updated: "2026-09-12T01:29:08.011Z"
dependencies: ["054", "056"]
plan: plans/lua-game-scripting.md
plan_step: Step 12
depends_on: ["stories/054-5cca-pending-P2-indri-reply-and-indri-send-with-depth-and-fan-out-.md", "stories/056-f2b1-pending-P2-durable-schedule-store-for-deferred-events.md"]
---

# indri.after, indri.at and indri.cancel

## Problem Statement

Countdown and wall-clock triggers collapse to one absolute fire time. A fired timer has no session, but every built-in Go handler rejects a nil session, and a hook that dereferences the session would fault on a timer fire.

## Acceptance Criteria

- [ ] A scheduled action dispatches through router.Dispatch after its fire time with its payload
- [ ] indri.after stamps the current game id so the fired request exposes a game id even with no session
- [ ] cancel before the fire time prevents dispatch
- [ ] A handler error marks the entry and does not hot-loop
- [ ] A scheduled action name must be a script action, never a built-in
- [ ] A hook that dereferences the session does not fault on a timer fire
- [ ] The scheduler runs as a third goroutine in the boot errgroup and shuts down on context cancellation
- [ ] State is read at fire time through indri.mutate rather than from a snapshot frozen at schedule time
- [ ] VERIFY: go test -race ./internal/services/scheduler/

## Files

- internal/services/scheduler/scheduler.go
- internal/services/boot/serve.go

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

### 2026-09-13T17:18:25.180Z - CI now fails on any test that skips without a database (repo-wide guard in ci.yml, KNOWN_SKIPS allowlist). The scheduler package inherits this automatically - write its tests DB-free against schedule.MemoryStore or CI will reject them.


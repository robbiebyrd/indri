---
id: 065-ed08
title: Make the new stateful tests actually run in CI
status: pending
priority: P1
type: chore
created: "2026-09-12T01:28:39.047Z"
updated: "2026-09-12T01:29:08.857Z"
dependencies: ["056"]
plan: plans/lua-game-scripting.md
plan_step: Step 21
depends_on: ["stories/056-f2b1-pending-P2-durable-schedule-store-for-deferred-events.md"]
---

# Make the new stateful tests actually run in CI

## Problem Statement

CI runs on a plain runner with no Mongo or Redis service containers, and game_concurrency_test.go skips when Mongo is unreachable. Every stateful test in this plan would therefore report green in CI without ever running, shipping the whole feature unverified.

## Acceptance Criteria

- [ ] Either the CI workflow gains a mongodb service, or in-memory fakes for the game and schedule stores are added
- [ ] The in-memory fake approach is preferred, matching the bare-injector precedent in the boot handlers test
- [ ] The tests for the mutate bracket, the schedule store and the scheduler fail when the logic is broken on a runner with no local Mongo
- [ ] No unexpected skips remain in the new packages
- [ ] air include_ext includes lua so editing a script triggers a reload
- [ ] VERIFY: go test ./...

## Files

- .github/workflows/ci.yml
- .air.toml

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


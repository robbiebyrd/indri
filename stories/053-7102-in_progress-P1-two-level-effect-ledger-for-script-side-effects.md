---
id: 053-7102
title: Two-level effect ledger for script side effects
status: in_progress
priority: P1
type: feature
created: "2026-09-12T01:28:12.567Z"
updated: "2026-09-13T18:34:10.965Z"
dependencies: ["052"]
plan: plans/lua-game-scripting.md
plan_step: Step 8
depends_on: ["stories/052-f8ac-pending-P1-indri-mutate-as-the-transaction-bracket.md"]
started_at: "2026-09-13T18:34:10.964Z"
---

# Two-level effect ledger for script side effects

## Problem Statement

The apply closure runs up to ten times on contention, so any side effect performed inline would fire once per attempt. Effects must buffer per attempt and flush only after the write commits, and the guarantee differs by effect kind because a standalone mongod has no multi-document transactions.

## Acceptance Criteria

- [ ] Effects queued inside a retried mutate attempt are emitted once per commit, not once per attempt
- [ ] Effects from an aborted or errored mutate are dropped
- [ ] Effects queued outside mutate flush when the handler returns
- [ ] Ordering is preserved across both ledger levels
- [ ] Reply and send are documented as best-effort, since a lost frame is recovered by the next refresh keyframe
- [ ] Timers are documented as at-least-once and carry an idempotency key, because a lost timer hangs a game forever
- [ ] The ledger consumes the committed flag from the mutate variant rather than inferring commit from a nil error
- [ ] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/effects.go

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


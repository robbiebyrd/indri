---
id: 073-03ec
title: Re-baseline script fixtures against per-index array diffing
status: wontfix
priority: P2
type: fix
created: "2026-10-01T13:29:44.859Z"
updated: "2026-10-01T13:51:33.689Z"
dependencies: ["071-3816"]
plan: plans/main-divergence-integration.md
plan_step: Step 6
depends_on: ["stories/071-3816-pending-P1-port-the-lua-engine-and-scheduler-onto-the-trunk-s.md"]
completed_at: "2026-10-01T13:51:33.689Z"
---

# Re-baseline script fixtures against per-index array diffing

## Problem Statement

The scripttest harness ports unchanged because remote's events.Diff still returns dotted-path maps, with positional encoding applied downstream by EncodePath. But remote added diffSlice, so arrays now diff per index: a tictactoe move emits stage.scenes.board.data.board.1.2 rather than the whole board array. Existing fixture expectations assert the whole array and will fail.

## Acceptance Criteria

- [ ] The scripttest harness itself needs no change to its assertion mechanism, confirmed by it running green against trunk's events package
- [ ] move.test.json expectations updated to per-index paths for the board writes
- [ ] Fixtures still assert on the published delta, since a write that never publishes is invisible to players
- [ ] VERIFY: go test ./internal/services/scripttest/
- [ ] indri-script test passes against the example: go run ./cmd/indri-script test ./example/tictactoe

## Files

- internal/services/scripttest/
- internal/services/scripttest/testdata/tictactoe/move.test.json

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

### 2026-10-01T13:51:33.618Z - Superseded by the trunk reversal: origin/main lacks the connection-independent dispatch refactor, so local main became the trunk and the port direction inverted. Plan plans/main-divergence-integration.md revised; this story's criteria were written for the opposite direction. Replaced by the revised story set.


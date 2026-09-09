---
id: 023-5e72
title: Measure GameStateParser delta replay cost with layouts in state
status: ready
priority: P3
type: task
created: "2026-09-09T19:04:59.226Z"
updated: "2026-09-09T19:05:28.272Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 13
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
---

# Measure GameStateParser delta replay cost with layouts in state

## Problem Statement

reapply() deep-clones the entire game via JSON round-trip and replays every retained delta on every message. Deltas are pruned only on keyframe, which arrives only on create/join/refresh/reconnect. Putting layouts in game state inflates both terms. The cost is currently unknown, and nobody should optimise it before it is measured.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] A benchmark replays a realistic ~200-widget layout at 1, 10, 100 and 500 accumulated deltas
- [ ] The benchmark asserts only that it completes; the measured numbers are the deliverable
- [ ] Measured figures are recorded in plans/layout-engine-renderer.md
- [ ] No optimisation is performed in this story; folding settled deltas into the base is explicitly left as a follow-up
- [ ] A recommendation is stated based on the measured numbers, not on speculation

## Files

- client/services/game-state-parser.bench.node-test.ts

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


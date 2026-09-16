---
id: 023-5e72
title: Measure GameStateParser delta replay cost with layouts in state
status: complete
priority: P3
type: task
created: "2026-09-09T19:04:59.226Z"
updated: "2026-09-16T03:58:39.885Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 13
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
completed_at: "2026-09-16T03:58:39.885Z"
---

# Measure GameStateParser delta replay cost with layouts in state

## Problem Statement

reapply() deep-clones the entire game via JSON round-trip and replays every retained delta on every message. Deltas are pruned only on keyframe, which arrives only on create/join/refresh/reconnect. Putting layouts in game state inflates both terms. The cost is currently unknown, and nobody should optimise it before it is measured.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] A benchmark replays a realistic ~200-widget layout at 1, 10, 100 and 500 accumulated deltas
- [x] The benchmark asserts only that it completes; the measured numbers are the deliverable
- [x] Measured figures are recorded in plans/layout-engine-renderer.md
- [x] No optimisation is performed in this story; folding settled deltas into the base is explicitly left as a follow-up
- [x] A recommendation is stated based on the measured numbers, not on speculation

## Files

- client/services/game-state-parser.bench.node-test.ts

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-09-16T03:58:35.472Z - Benchmarked delta replay at 200 widgets across 1/10/100/500 deltas. Results: 0.21ms, 0.19ms, 0.29ms, 0.49ms. No optimization needed at PoC scale.


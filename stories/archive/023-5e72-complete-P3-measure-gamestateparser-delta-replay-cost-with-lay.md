---
id: 023-5e72
title: Measure GameStateParser delta replay cost with layouts in state
status: complete
priority: P3
type: task
created: "2026-09-09T19:04:59.226Z"
updated: "2026-09-10T00:01:40.416Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 13
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
started_at: "2026-09-09T23:51:55.107Z"
completed_at: "2026-09-10T00:01:40.416Z"
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

- [x] [completeness] Completeness (All 6 criteria checked. Numbers recorded in the plan's Performance Considerations. 87/87 tests.)
- [x] [feature-availability] Feature availability (Benchmark runs as part of pnpm test and prints its table to stdout; reproduced independently after the agent reported （0.206ms clone, 0.680ms at 500 deltas）.)
- [x] [robustness] Robustness (Deterministic: no RNG, fixed keyframe timestamp, fixed delta stride. Asserts only completion plus a deliberately generous 10s bound so a catastrophic regression fails CI without flaky timing assertions.)
- [~] [resilience] Resilience (No IO or external dependency.)
- [~] [security] Security (A benchmark over synthetic in-memory data. No IO, no wire data.)
- [~] [defense-in-depth] Defense in depth (Measurement only; the 10s ceiling is the single guard and is intentionally loose.)
- [~] [input-validation] Input validation (No external input; the harness builds its own state.)
- [~] [thread-safety] Thread safety (Single-threaded synchronous measurement.)
- [~] [configurability] Configurability (A fixed measurement harness; the delta counts are the experiment, not a knob.)

## QA

87/87 tests. Benchmark reproduced independently of the agent's run; numbers recorded in the plan. No optimisation performed, per the story's explicit constraint.

## Work Log

### 2026-09-10T00:01:38.262Z - Measured, not optimised. 200-widget layout, 49.8KB serialised, Node 22 on M-series: 1 delta 0.205ms, 10 0.221ms, 100 0.315ms, 500 0.680ms median. Deep clone alone is 0.206ms and FLAT, so at low backlogs the clone is the entire cost; replay adds ~1us per retained delta. Model 0.21 + N*0.001ms fits every point; break-even ~200 deltas. Noise controlled with JIT warm-up on the timed paths, 11 samples per point (5 at 500), median and best reported; four consecutive runs stayed within 5%. Deltas are deterministic single-leaf dotted paths cycled with a fixed stride. CONCLUSION: layouts are not the problem - unbounded delta retention is. Widget count is the benign term (300 widgets ~= 75KB ~= 0.3ms, flat per message); the retained-delta count has no ceiling because keyframes only arrive on create/join/refresh/reconnect. A 30-minute session at 1 delta/sec retains ~1800 deltas -> ~2.1ms per message here, and Hermes on a mid-range phone is realistically 3-10x slower, so 6-20ms synchronously on the JS thread before React re-renders. Layouts roughly double a sub-millisecond constant; they did not create this. Deferred recommendation (NOT implemented, per the story): fold settled deltas into baseState past a small reorder window, advancing cutoff and dropping them - reuses the deleteBefore/cutoff machinery keyframes already have, making per-message cost flat regardless of session length. Cheaper to price first: structuredClone instead of the JSON round-trip, or a periodic server-side keyframe which needs no client change at all. Numbers recorded in plans/layout-engine-renderer.md.


### 2026-09-10T00:01:39.291Z - Proof completeness set PROVEN: All 6 criteria checked. Numbers recorded in the plan's Performance Considerations. 87/87 tests.

### 2026-09-10T00:01:39.483Z - Proof feature-availability set PROVEN: Benchmark runs as part of pnpm test and prints its table to stdout; reproduced independently after the agent reported (0.206ms clone, 0.680ms at 500 deltas).

### 2026-09-10T00:01:39.599Z - Proof robustness set PROVEN: Deterministic: no RNG, fixed keyframe timestamp, fixed delta stride. Asserts only completion plus a deliberately generous 10s bound so a catastrophic regression fails CI without flaky timing assertions.

### 2026-09-10T00:01:39.699Z - Proof configurability set NOT_APPLICABLE: A fixed measurement harness; the delta counts are the experiment, not a knob.

### 2026-09-10T00:01:39.804Z - Proof security set NOT_APPLICABLE: A benchmark over synthetic in-memory data. No IO, no wire data.

### 2026-09-10T00:01:39.915Z - Proof resilience set NOT_APPLICABLE: No IO or external dependency.

### 2026-09-10T00:01:40.038Z - Proof defense-in-depth set NOT_APPLICABLE: Measurement only; the 10s ceiling is the single guard and is intentionally loose.

### 2026-09-10T00:01:40.132Z - Proof input-validation set NOT_APPLICABLE: No external input; the harness builds its own state.

### 2026-09-10T00:01:40.259Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded synchronous measurement.

---
id: 014-516d
title: Grid coordinate math and AABB collision detection
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.220Z"
updated: "2026-09-09T19:05:27.267Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 4
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
---

# Grid coordinate math and AABB collision detection

## Problem Statement

The board grid ranges from 8x8 to 4096x4096. At the maximum that is 16.7 million cells, so the grid must be a pure coordinate space and never materialised as cells. Widgets are rects converted to percentage boxes, and non-absolute widgets must not overlap.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] toPercentBox converts a rect to percentage left/top/width/height, verified at 8x8, 12x12 and 4096x4096
- [ ] No cell array or per-cell structure is allocated anywhere, at any grid size
- [ ] collides() is the four-comparison AABB test; edge-touching returns false, corner-touching, containment and identical rects return true, and a rect never collides with itself
- [ ] canPlace() implements reject-and-snap-back, not push-cascade
- [ ] Absolute widgets are excluded from the collision set as both subject and obstacle
- [ ] Collision is scoped per grid level, so a sub-grid child collides only with its own siblings
- [ ] cols or rows outside 8..4096 are rejected; zero, negative and fractional w/h are rejected; an out-of-bounds rect is clamped

## Files

- client/layout/grid/coords.ts
- client/layout/grid/collision.ts
- client/layout/grid/grid.node-test.ts

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


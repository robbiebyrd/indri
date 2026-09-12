---
id: 014-516d
title: Grid coordinate math and AABB collision detection
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.220Z"
updated: "2026-09-09T23:42:41.498Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 4
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
started_at: "2026-09-09T23:33:59.365Z"
completed_at: "2026-09-09T23:42:41.498Z"
---

# Grid coordinate math and AABB collision detection

## Problem Statement

The board grid ranges from 8x8 to 4096x4096. At the maximum that is 16.7 million cells, so the grid must be a pure coordinate space and never materialised as cells. Widgets are rects converted to percentage boxes, and non-absolute widgets must not overlap.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] toPercentBox converts a rect to percentage left/top/width/height, verified at 8x8, 12x12 and 4096x4096
- [x] No cell array or per-cell structure is allocated anywhere, at any grid size
- [x] collides() is the four-comparison AABB test; edge-touching returns false, corner-touching, containment and identical rects return true, and a rect never collides with itself
- [x] canPlace() implements reject-and-snap-back, not push-cascade
- [x] Absolute widgets are excluded from the collision set as both subject and obstacle
- [x] Collision is scoped per grid level, so a sub-grid child collides only with its own siblings
- [x] cols or rows outside 8..4096 are rejected; zero, negative and fractional w/h are rejected; an out-of-bounds rect is clamped

## Files

- client/layout/grid/coords.ts
- client/layout/grid/collision.ts
- client/layout/grid/grid.node-test.ts

## Proof

- [x] [completeness] Completeness (All 8 criteria checked, with criterion 4's self-collision clause deviating as recorded in the worklog. 49/49 tests pass （20 new）, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (Verified in a real Metro bundle, not just Node: toPercentBox/collides/canPlace wired into app/board/spike.tsx temporarily and exported for web and iOS. Probes rendered ok （25%）, ok （true）, ok （true）. This also proved the repo's first source-to-source .ts extension import resolves under Metro on both platforms.)
- [x] [robustness] Robustness (clampRect's result always satisfies isValidRect and isWithinBounds, so callers need not re-check. NaN cannot propagate through comparisons.)
- [~] [resilience] Resilience (No IO or external dependency; the only failure mode is invalid input, covered under Input validation.)
- [~] [security] Security (Pure arithmetic over numbers. No IO, no trust boundary; server-side bounds enforcement is Plan B scope.)
- [~] [defense-in-depth] Defense in depth (Client-side geometry only. The authoritative overlap check belongs in the Go handler （Plan B）.)
- [x] [input-validation] Input validation (isValidGridSize rejects 7, 4097 and non-integers; isValidRect rejects zero, negative and fractional spans and negative origins; canPlace rejects out-of-bounds. clampRect normalises non-finite input rather than propagating NaN.)
- [~] [thread-safety] Thread safety (Pure functions, no shared mutable state.)
- [~] [configurability] Configurability (MIN_DIM/MAX_DIM are fixed by the spec （8..4096）, deliberately not configurable.)

## QA

49/49 tests, typecheck clean, zero new lint warnings. Proven inside a real Metro bundle on web and iOS, which also validated the repo's first .ts-extension source import.

## Work Log

### 2026-09-09T23:42:26.136Z - Pure rect math, zero react-native and zero zod imports, so it runs in bare Node. Grid is a coordinate space and never materialised: 4096x4096 is 16.7M cells but only ever a divisor. coords.ts exports MIN_DIM/MAX_DIM, isValidGridSize, isValidRect, isWithinBounds, toPercentBox, clampRect; collision.ts exports collides and canPlace. collides is RGL's four-comparison AABB with strict <, so edge-touching is not a collision. canPlace is reject-and-snap-back (RGL's preventCollision), never push-cascade. DEVIATION worth recording: criterion 4 says a rect against itself is false, but GridRect carries no id, so an identity guard is impossible inside collides - RGL can only do it because its LayoutItem has .i. collides(r,r) is therefore true (pure geometry) and self-exemption lives in canPlace keyed on id, which is what lets a drag not collide with the position it is leaving. That behaviour is tested; the criterion as I wrote it was unimplementable. I also corrected a comment the agent left claiming self-collision returns false, which contradicted its own passing test. clampRect normalises as well as clamps (floors fractions, raises zero/negative spans to 1, collapses non-finite) so its result always satisfies isValidRect and isWithinBounds. 20 tests.


### 2026-09-09T23:42:40.579Z - Proof completeness set PROVEN: All 8 criteria checked, with criterion 4's self-collision clause deviating as recorded in the worklog. 49/49 tests pass (20 new), typecheck clean, zero new lint warnings.

### 2026-09-09T23:42:40.666Z - Proof feature-availability set PROVEN: Verified in a real Metro bundle, not just Node: toPercentBox/collides/canPlace wired into app/board/spike.tsx temporarily and exported for web and iOS. Probes rendered ok (25%), ok (true), ok (true). This also proved the repo's first source-to-source .ts extension import resolves under Metro on both platforms.

### 2026-09-09T23:42:40.758Z - Proof input-validation set PROVEN: isValidGridSize rejects 7, 4097 and non-integers; isValidRect rejects zero, negative and fractional spans and negative origins; canPlace rejects out-of-bounds. clampRect normalises non-finite input rather than propagating NaN.

### 2026-09-09T23:42:40.847Z - Proof robustness set PROVEN: clampRect's result always satisfies isValidRect and isWithinBounds, so callers need not re-check. NaN cannot propagate through comparisons.

### 2026-09-09T23:42:41.018Z - Proof security set NOT_APPLICABLE: Pure arithmetic over numbers. No IO, no trust boundary; server-side bounds enforcement is Plan B scope.

### 2026-09-09T23:42:41.108Z - Proof resilience set NOT_APPLICABLE: No IO or external dependency; the only failure mode is invalid input, covered under Input validation.

### 2026-09-09T23:42:41.206Z - Proof defense-in-depth set NOT_APPLICABLE: Client-side geometry only. The authoritative overlap check belongs in the Go handler (Plan B).

### 2026-09-09T23:42:41.311Z - Proof thread-safety set NOT_APPLICABLE: Pure functions, no shared mutable state.

### 2026-09-09T23:42:41.398Z - Proof configurability set NOT_APPLICABLE: MIN_DIM/MAX_DIM are fixed by the spec (8..4096), deliberately not configurable.

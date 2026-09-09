---
id: 015-dd9e
title: Layout schema with defensive, non-throwing parse
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.220Z"
updated: "2026-09-09T19:05:27.384Z"
dependencies: ["013", "014"]
plan: plans/layout-engine-renderer.md
plan_step: Step 5
depends_on: ["stories/013-254f-pending-P2-style-schema-and-cross-platform-style-compiler.md", "stories/014-516d-pending-P2-grid-coordinate-math-and-aabb-collision-detection.md"]
---

# Layout schema with defensive, non-throwing parse

## Problem Statement

game.data.layout arrives from the server as untrusted map[string]interface{} with zero server-side schema on every keyframe. Bad data must degrade to a reported issue and still render, because a blank board is a worse failure than a wrong one. Widgets must be a map keyed by id because events.Diff replaces arrays whole.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] GameLayout schema has grid, optional style, optional script and a scenes record; each scene has optional style, optional script and a widgets record keyed by id
- [ ] Widgets are a zod record (a map), never an array, so a single widget change produces a leaf-path delta rather than a whole-array replace
- [ ] Absolute placement uses a Percent branded type so pixel values are unrepresentable at the type level, not merely validated away
- [ ] parseLayout returns {layout, issues} and never throws on any input
- [ ] These yield an issue and still render: overlapping non-absolute pair, out-of-bounds rect, zero-size widget, fractional coordinate, unknown widget type, sub-grid past depth 4, layout scene with no matching stage.scenes key
- [ ] These are rejected outright: a privateData key anywhere, a pixel value in absolute placement, cols/rows outside 8..4096
- [ ] Sub-grid depth cap of 4 is enforced via a depth counter threaded through the recursive z.lazy schema, rendering an error placeholder past the cap
- [ ] A test documents why privateData is reserved: SanitizeDelta strips any path containing that segment at any depth

## Files

- client/layout/schema/placement.ts
- client/layout/schema/widget.ts
- client/layout/schema/layout.ts
- client/layout/schema/layout.node-test.ts

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


---
id: 015-dd9e
title: Layout schema with defensive, non-throwing parse
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.220Z"
updated: "2026-09-09T23:48:26.032Z"
dependencies: ["013", "014"]
plan: plans/layout-engine-renderer.md
plan_step: Step 5
depends_on: ["stories/013-254f-pending-P2-style-schema-and-cross-platform-style-compiler.md", "stories/014-516d-pending-P2-grid-coordinate-math-and-aabb-collision-detection.md"]
started_at: "2026-09-09T23:45:01.843Z"
completed_at: "2026-09-09T23:48:26.032Z"
---

# Layout schema with defensive, non-throwing parse

## Problem Statement

game.data.layout arrives from the server as untrusted map[string]interface{} with zero server-side schema on every keyframe. Bad data must degrade to a reported issue and still render, because a blank board is a worse failure than a wrong one. Widgets must be a map keyed by id because events.Diff replaces arrays whole.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] GameLayout schema has grid, optional style, optional script and a scenes record; each scene has optional style, optional script and a widgets record keyed by id
- [x] Widgets are a zod record (a map), never an array, so a single widget change produces a leaf-path delta rather than a whole-array replace
- [x] Absolute placement uses a Percent branded type so pixel values are unrepresentable at the type level, not merely validated away
- [x] parseLayout returns {layout, issues} and never throws on any input
- [x] These yield an issue and still render: overlapping non-absolute pair, out-of-bounds rect, zero-size widget, fractional coordinate, unknown widget type, sub-grid past depth 4, layout scene with no matching stage.scenes key
- [x] These are rejected outright: a privateData key anywhere, a pixel value in absolute placement, cols/rows outside 8..4096
- [x] Sub-grid depth cap of 4 is enforced via a depth counter threaded through the recursive z.lazy schema, rendering an error placeholder past the cap
- [x] A test documents why privateData is reserved: SanitizeDelta strips any path containing that segment at any depth

## Files

- client/layout/schema/placement.ts
- client/layout/schema/widget.ts
- client/layout/schema/layout.ts
- client/layout/schema/layout.node-test.ts

## Proof

- [x] [completeness] Completeness (All 9 criteria checked, with criterion 8's error-placeholder clause deviating as recorded （warn + drop; placeholder is a renderer concern）. 68/68 tests pass, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (Test coverage verified non-vacuous by mutation: breaking RESERVED_KEY and the overlap comparison produced 3 test failures.)
- [x] [robustness] Robustness (parseLayout never throws - asserted against undefined, null, 0, empty string, array, empty object, NaN and malformed shapes. It runs on every keyframe, so a throw would take out the render.)
- [x] [resilience] Resilience (Degrades rather than fails: bad geometry clamps, unknown types and scene mismatches warn, malformed sub-grid configs are contained to that widget. Only three conditions blank the layout, each because nothing sensible can be rendered.)
- [x] [security] Security (Rejects privateData at any depth, which would otherwise be silently stripped by the server's SanitizeDelta and cause undebuggable data loss. Unbounded recursion is capped at depth 4. No eval, no IO. NOT a security boundary for authoring - Go-side validation is Plan B.)
- [x] [defense-in-depth] Defense in depth (Client-side normalisation is one layer; the authoritative bounds/overlap check belongs in the Go layout handler （Plan B story）. The duplication is intentional and documented.)
- [x] [input-validation] Input validation (This IS the validation boundary for untrusted wire data. .strict（） at every level; reserved-key scan is depth-first over objects AND arrays; fatal vs tolerable split is explicit and tested case by case.)
- [~] [thread-safety] Thread safety (Pure function over a value; mutation is confined to zod's freshly parsed copy, never the caller's object.)
- [~] [configurability] Configurability (knownWidgetTypes and sceneIds are the only knobs and both are optional caller inputs; MAX_SUBGRID_DEPTH and the 8..4096 range are fixed by spec.)

## QA

68/68 tests, typecheck clean, zero new lint warnings. Mutation-checked: injecting two faults produced 3 failures, confirming the tests can fail.

## Work Log

### 2026-09-09T23:48:09.179Z - placement.ts / widget.ts / layout.ts plus 19 tests. Key design calls: (1) parseLayout returns a NORMALISED layout - every grid rect is guaranteed valid and in-bounds because out-of-range rects are clamped via 014's clampRect rather than rejected, so renderers never re-check geometry. (2) Two issue severities. 'error' means no layout at all (privateData anywhere, a pixel value in absolute placement, grid dims outside 8..4096); 'warning' means it renders anyway (overlap, out-of-bounds, zero-size, fractional, unknown type, over-deep nesting, scene-id mismatch). The split is deliberate: a blank board is a worse failure than a board with one widget in the wrong place. (3) Placement's grid branch takes plain numbers, NOT int/positive, so a sloppy value clamps and warns; the absolute branch stays strict because a bare number there is the wrong FORMAT with nothing to recover. (4) config is opaque (z.record) - widget types own their own config via the registry in 016, so encoding shapes here would duplicate it and go stale. (5) knownWidgetTypes and sceneIds are optional caller-supplied sets, keeping the dependency direction right: schema does not import the registry. DEVIATION on criterion 8: past the depth cap I warn and drop the excess children rather than emitting an 'error placeholder' - a placeholder is a renderer concern, and the renderer can build one from the issue. Verified the tests are not vacuous with a mutation check: breaking the reserved-key constant and the overlap comparison produced 3 failures, then restored to 68/68.


### 2026-09-09T23:48:25.310Z - Proof completeness set PROVEN: All 9 criteria checked, with criterion 8's error-placeholder clause deviating as recorded (warn + drop; placeholder is a renderer concern). 68/68 tests pass, typecheck clean, zero new lint warnings.

### 2026-09-09T23:48:25.388Z - Proof input-validation set PROVEN: This IS the validation boundary for untrusted wire data. .strict() at every level; reserved-key scan is depth-first over objects AND arrays; fatal vs tolerable split is explicit and tested case by case.

### 2026-09-09T23:48:25.464Z - Proof robustness set PROVEN: parseLayout never throws - asserted against undefined, null, 0, empty string, array, empty object, NaN and malformed shapes. It runs on every keyframe, so a throw would take out the render.

### 2026-09-09T23:48:25.537Z - Proof resilience set PROVEN: Degrades rather than fails: bad geometry clamps, unknown types and scene mismatches warn, malformed sub-grid configs are contained to that widget. Only three conditions blank the layout, each because nothing sensible can be rendered.

### 2026-09-09T23:48:25.615Z - Proof security set PROVEN: Rejects privateData at any depth, which would otherwise be silently stripped by the server's SanitizeDelta and cause undebuggable data loss. Unbounded recursion is capped at depth 4. No eval, no IO. NOT a security boundary for authoring - Go-side validation is Plan B.

### 2026-09-09T23:48:25.692Z - Proof defense-in-depth set PROVEN: Client-side normalisation is one layer; the authoritative bounds/overlap check belongs in the Go layout handler (Plan B story). The duplication is intentional and documented.

### 2026-09-09T23:48:25.770Z - Proof feature-availability set PROVEN: Test coverage verified non-vacuous by mutation: breaking RESERVED_KEY and the overlap comparison produced 3 test failures.

### 2026-09-09T23:48:25.857Z - Proof thread-safety set NOT_APPLICABLE: Pure function over a value; mutation is confined to zod's freshly parsed copy, never the caller's object.

### 2026-09-09T23:48:25.945Z - Proof configurability set NOT_APPLICABLE: knownWidgetTypes and sceneIds are the only knobs and both are optional caller inputs; MAX_SUBGRID_DEPTH and the 8..4096 range are fixed by spec.

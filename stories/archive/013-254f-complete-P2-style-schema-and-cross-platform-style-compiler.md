---
id: 013-254f
title: Style schema and cross-platform style compiler
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.219Z"
updated: "2026-09-09T23:42:15.785Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 3
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
started_at: "2026-09-09T23:33:59.233Z"
completed_at: "2026-09-09T23:42:15.785Z"
---

# Style schema and cross-platform style compiler

## Problem Statement

React Native has no background-image property, no multiple backgrounds, no background-size or repeat, no CSS Grid and no calc(). The engine needs a deliberately closed, RN-expressible styling vocabulary that compiles to an RN ViewStyle plus an ordered list of background layers rendered beneath children, rather than pretending a CSS subset exists.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] StyleSchema is a zod object using .strict() so a typo'd style key surfaces as an issue instead of being silently ignored
- [x] Schema covers backgroundColor, backgroundImage, backgroundGradient, border (width/color/style/radius), padding, opacity, boxShadow and overflow, and nothing else
- [x] compileStyle returns viewStyle plus an ordered layers array; layer order is gradient then image then children
- [x] boxShadow rejects unitless lengths, because RN 0.79 requires units in the string
- [x] Only boxShadow is emitted for shadows; no iOS shadowColor/shadowOffset and no Android elevation
- [x] Per-corner border radius expands correctly and padding accepts both a scalar and a per-edge form
- [x] Tests cover each of the above including a rejected unknown key and a rejected unitless boxShadow

## Files

- client/layout/schema/style.ts
- client/layout/style/compile.ts
- client/layout/style/compile.node-test.ts

## Proof

- [x] [completeness] Completeness (All 8 criteria checked. 49/49 tests pass （14 new）, typecheck clean, lint adds zero warnings.)
- [x] [feature-availability] Feature availability (Verified in a real Metro bundle, not just Node: compileStyle wired into app/board/spike.tsx temporarily and exported for web and iOS; the gradient-layer probe rendered ok （1）.)
- [x] [robustness] Robustness (compileStyle（undefined） returns {viewStyle:{}, layers:[]} rather than throwing. compileStyle assumes already-parsed input; rejection paths are tested through StyleSchema.safeParse.)
- [~] [resilience] Resilience (No IO, no network, no failure modes beyond validation which is covered under Input validation.)
- [~] [security] Security (Pure value transformation over already-sanitized wire data. No trust boundary, no IO, no eval.)
- [~] [defense-in-depth] Defense in depth (Single pure module; server-side validation of layout data is Plan B story scope.)
- [x] [input-validation] Input validation (.strict（） at every nesting level; rejects unknown keys, unitless boxShadow, opacity outside 0..1, gradients under 2 colours, and negative padding/radius. All covered by tests.)
- [~] [thread-safety] Thread safety (Pure functions, no shared mutable state.)
- [x] [configurability] Configurability (The style vocabulary IS the configuration surface, and it is deliberately closed: a key RN cannot honour is worse than no key.)

## QA

49/49 tests, typecheck clean, zero new lint warnings. Additionally proven inside a real Metro bundle on web and iOS, not only in Node.

## Work Log

### 2026-09-09T23:42:01.381Z - StyleSchema (zod 4.5.4, .strict() at EVERY nesting level, not just the top) plus compileStyle returning {viewStyle, layers}. Gradients and background images compile to absolutely-positioned layers rather than style props, because RN has no background-image property; layer order is gradient then image, children on top. boxShadow is the only shadow emitted - no iOS shadow*/elevation - and the schema regex requires a unit because RN 0.79 rejects unitless lengths. Scalar radius/padding fan out to all four corner/edge props; the shorthand is never emitted so a partial per-side value stays unambiguous for the Lua override layer later. Review fixes I made on top of the agent's work: added .nonnegative() to padding and all radius forms (RN's behaviour for negatives is platform-dependent) plus a test covering all four rejection paths. Deviation from the plan: backgroundImage.uri is z.string() not z.string().url(), so data: and relative asset URIs are not rejected; plan updated to match. 14 tests.


### 2026-09-09T23:42:14.939Z - Proof completeness set PROVEN: All 8 criteria checked. 49/49 tests pass (14 new), typecheck clean, lint adds zero warnings.

### 2026-09-09T23:42:15.029Z - Proof feature-availability set PROVEN: Verified in a real Metro bundle, not just Node: compileStyle wired into app/board/spike.tsx temporarily and exported for web and iOS; the gradient-layer probe rendered ok (1).

### 2026-09-09T23:42:15.117Z - Proof input-validation set PROVEN: .strict() at every nesting level; rejects unknown keys, unitless boxShadow, opacity outside 0..1, gradients under 2 colours, and negative padding/radius. All covered by tests.

### 2026-09-09T23:42:15.202Z - Proof robustness set PROVEN: compileStyle(undefined) returns {viewStyle:{}, layers:[]} rather than throwing. compileStyle assumes already-parsed input; rejection paths are tested through StyleSchema.safeParse.

### 2026-09-09T23:42:15.296Z - Proof configurability set PROVEN: The style vocabulary IS the configuration surface, and it is deliberately closed: a key RN cannot honour is worse than no key.

### 2026-09-09T23:42:15.384Z - Proof security set NOT_APPLICABLE: Pure value transformation over already-sanitized wire data. No trust boundary, no IO, no eval.

### 2026-09-09T23:42:15.481Z - Proof resilience set NOT_APPLICABLE: No IO, no network, no failure modes beyond validation which is covered under Input validation.

### 2026-09-09T23:42:15.587Z - Proof defense-in-depth set NOT_APPLICABLE: Single pure module; server-side validation of layout data is Plan B story scope.

### 2026-09-09T23:42:15.676Z - Proof thread-safety set NOT_APPLICABLE: Pure functions, no shared mutable state.

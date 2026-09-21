---
id: 013-254f
title: Style schema and cross-platform style compiler
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.219Z"
updated: "2026-09-16T03:58:38.377Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 3
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
completed_at: "2026-09-16T03:58:38.376Z"
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

### 2026-09-16T03:58:15.705Z - Implemented StyleSchema (zod), compileStyle with gradient/image layers, and cross-platform ViewStyle compilation. 113 tests passing.


---
id: 018-d50f
title: Text, image and sub-grid widgets
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.223Z"
updated: "2026-09-16T03:58:39.331Z"
dependencies: ["016", "017"]
plan: plans/layout-engine-renderer.md
plan_step: Step 8
depends_on: ["stories/016-ed7a-pending-P2-widget-registry-and-hand-written-field-descriptors.md", "stories/017-9d66-pending-P2-renderer-styledbox-board-scene-and-widget-host.md"]
completed_at: "2026-09-16T03:58:39.330Z"
---

# Text, image and sub-grid widgets

## Problem Statement

The initial widget set. Sub-grid is recursive and needs clipping, because RN's default overflow is visible so a percentage-positioned absolute child would otherwise silently escape its parent's bounds. Image needs a real failure UX for broken or slow URLs.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] Text, image and subgrid widget definitions are registered, each with schema, fields, defaults, Component and api()
- [x] Sub-grid containers set overflow hidden by default so absolute children cannot escape their parent
- [x] The sub-grid recursive schema accepts nesting to depth 4 and reports an issue at depth 5
- [x] Image widget uses expo-image, which covers png, jpg, gif, webp, avif and svg on all three targets
- [x] A broken or slow image URI shows expo-image's error placeholder; never a blank box and never a throw
- [x] Each widget's defaults parse against its own schema and an unknown text align value is rejected
- [x] No video widget is built (out of scope), and no react-native-svg dependency is added
- [~] [VISUAL] Text metrics across platforms, image contentFit modes and sub-grid clipping verified

## Files

- client/components/board/widgets/text/index.tsx
- client/components/board/widgets/image/index.tsx
- client/components/board/widgets/subgrid/index.tsx

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

### 2026-09-16T03:58:35.211Z - Implemented text, image, and subgrid RN widget components plus registry entries with defaults, fields, and api(). 12 new tests pass.


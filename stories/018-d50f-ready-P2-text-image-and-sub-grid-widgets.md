---
id: 018-d50f
title: Text, image and sub-grid widgets
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.223Z"
updated: "2026-09-09T19:05:27.715Z"
dependencies: ["016", "017"]
plan: plans/layout-engine-renderer.md
plan_step: Step 8
depends_on: ["stories/016-ed7a-pending-P2-widget-registry-and-hand-written-field-descriptors.md", "stories/017-9d66-pending-P2-renderer-styledbox-board-scene-and-widget-host.md"]
---

# Text, image and sub-grid widgets

## Problem Statement

The initial widget set. Sub-grid is recursive and needs clipping, because RN's default overflow is visible so a percentage-positioned absolute child would otherwise silently escape its parent's bounds. Image needs a real failure UX for broken or slow URLs.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] Text, image and subgrid widget definitions are registered, each with schema, fields, defaults, Component and api()
- [ ] Sub-grid containers set overflow hidden by default so absolute children cannot escape their parent
- [ ] The sub-grid recursive schema accepts nesting to depth 4 and reports an issue at depth 5
- [ ] Image widget uses expo-image, which covers png, jpg, gif, webp, avif and svg on all three targets
- [ ] A broken or slow image URI shows expo-image's error placeholder; never a blank box and never a throw
- [ ] Each widget's defaults parse against its own schema and an unknown text align value is rejected
- [ ] No video widget is built (out of scope), and no react-native-svg dependency is added
- [ ] [VISUAL] Text metrics across platforms, image contentFit modes and sub-grid clipping verified

## Files

- client/components/board/widgets/text/index.tsx
- client/components/board/widgets/image/index.tsx
- client/components/board/widgets/subgrid/index.tsx

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


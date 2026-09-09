---
id: 017-9d66
title: "Renderer: StyledBox, board, scene and widget host"
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.222Z"
updated: "2026-09-09T19:05:27.608Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 7
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
---

# Renderer: StyledBox, board, scene and widget host

## Problem Statement

The compiled style and percentage geometry need RN components to render them. Because every delta produces a fresh deep-cloned game object, unkeyed children would re-reconcile the entire widget tree on every websocket message, so memoisation and stable ids are a correctness-adjacent performance requirement rather than an optimisation.

## Acceptance Criteria

- [ ] StyledBox paints compiled background layers as absolutely-filled siblings beneath children, since RN has no background-image property
- [ ] The board container is position relative and every widget is absolutely positioned with a percentage box; there is no flex layout inside the grid
- [ ] WidgetHost is wrapped in React.memo and keyed by widget id
- [ ] stage.currentScene selects which layout scene entry renders
- [ ] Issues returned by parseLayout render into a dev-only overlay, never to the player
- [ ] A route exists at client/app/board/index.tsx that renders a layout end to end
- [ ] [VISUAL] Gradient rendering, border radii, background-layer ordering and percentage box alignment verified at both 8x8 and 4096x4096
- [ ] VERIFY: cd client && pnpm run typecheck

## Files

- client/components/board/styled-box.tsx
- client/components/board/board-view.tsx
- client/components/board/scene-view.tsx
- client/components/board/widget-host.tsx
- client/app/board/index.tsx

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


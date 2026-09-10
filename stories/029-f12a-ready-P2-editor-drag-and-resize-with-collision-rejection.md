---
id: 029-f12a
title: "Editor: drag and resize with collision rejection"
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.966Z"
updated: "2026-09-10T00:58:09.019Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 5
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
---

# Editor: drag and resize with collision rejection

## Problem Statement

A host needs to move and resize widgets directly on the board. Emitting per frame would be a delta storm, forcing a full deep-clone-and-replay on every connected client for every frame of a drag.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm run typecheck
- [ ] Exactly ONE op is emitted on gesture end, never per frame
- [ ] Gesture position lives in Reanimated shared values on the UI thread; only the committed rect crosses to JS via runOnJS
- [ ] A move that fails canPlace snaps back rather than being sent
- [ ] GestureHandlerRootView wraps the app root in app/_layout.tsx, which it currently does not - without it gestures silently do nothing on every platform including web
- [ ] Stays on Gesture.Pan() plus GestureDetector, since RNGH 3's hook API needs RN >= 0.82 and this is 0.79
- [ ] The grid-line overlay is drawn only when cols*rows <= 4096; at 4096x4096 it would be 8192 lines, so dimensions are shown as text instead
- [ ] [VISUAL] Drag feel, snap threshold, resize-handle hit targets (minimum 44pt on touch) and rejection feedback verified by a human

## Files

- client/components/board/editor/drag-resize.tsx
- client/app/_layout.tsx
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


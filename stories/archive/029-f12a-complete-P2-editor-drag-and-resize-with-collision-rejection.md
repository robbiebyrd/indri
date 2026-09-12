---
id: 029-f12a
title: "Editor: drag and resize with collision rejection"
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.966Z"
updated: "2026-09-12T02:33:30.367Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 5
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
started_at: "2026-09-12T01:39:12.984Z"
completed_at: "2026-09-12T02:33:30.367Z"
---

# Editor: drag and resize with collision rejection

## Problem Statement

A host needs to move and resize widgets directly on the board. Emitting per frame would be a delta storm, forcing a full deep-clone-and-replay on every connected client for every frame of a drag.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm run typecheck
- [x] Exactly ONE op is emitted on gesture end, never per frame
- [x] Gesture position lives in Reanimated shared values on the UI thread; only the committed rect crosses to JS via runOnJS
- [x] A move that fails canPlace snaps back rather than being sent
- [x] GestureHandlerRootView wraps the app root in app/_layout.tsx, which it currently does not - without it gestures silently do nothing on every platform including web
- [x] Stays on Gesture.Pan() plus GestureDetector, since RNGH 3's hook API needs RN >= 0.82 and this is 0.79
- [x] The grid-line overlay is drawn only when cols*rows <= 4096; at 4096x4096 it would be 8192 lines, so dimensions are shown as text instead
- [REJECTED] [VISUAL] Drag feel, snap threshold, resize-handle hit targets (minimum 44pt on touch) and rejection feedback verified by a human ([VISUAL] Drag feel, snap threshold, resize-handle hit targets and rejection feedback need a human on a real screen. The pure snapping math is unit-tested; the gesture layer is not and cannot be without a renderer.)

## Files

- client/components/board/editor/drag-resize.tsx
- client/app/_layout.tsx
- client/app/board/index.tsx

## Proof

- [x] [completeness] Completeness (7 of 8 criteria checked; criterion 8 is [VISUAL] and rejected with a reason. 270/270 tests, typecheck clean, lint down to 5 warnings.)
- [x] [feature-availability] Feature availability (GestureHandlerRootView now wraps the app root; without it every gesture silently does nothing on every platform. Verified the editor reaches the web bundle by grepping the emitted output for EditorOverlay, Palette, ConfigPanel, snapMove and snapResize.)
- [x] [robustness] Robustness (A move that fails canPlace snaps back and is not sent, reusing setPlacement's return value rather than duplicating the check. The frame is a transient offset from the server's rect, so it can never hold a second copy of state.)
- [x] [resilience] Resilience (Exactly ONE op on gesture end, never per frame - a per-frame op would make every connected client deep-clone and replay the whole game state once per frame of a drag. Grid lines are suppressed past 4096 cells, where 8190 line views would render as a solid block anyway.)
- [x] [security] Security (Host gating uses game.players[userId].host, the same flag the server authorizes on, and is documented as a UX gate only - a non-host who calls the action anyway is rejected server-side.)
- [x] [defense-in-depth] Defense in depth (Local canPlace, then the Go per-op decoder, then validateLayout on the resulting document.)
- [x] [input-validation] Input validation (snapMove and snapResize clamp at grid edges and floor the span at one cell; the pure snapping math is unit-tested including the clamping cases.)
- [x] [thread-safety] Thread safety (Gesture state stays on the UI thread in shared values; only the committed rect crosses to JS via runOnJS, once, at the end.)
- [~] [configurability] Configurability (Driven by layout data and the registry; no separate configuration surface.)

## QA

- [ ] 270/270 tests, typecheck clean, lint 6 -> 5 warnings
- [ ] Editor confirmed present in the emitted web bundle
- [ ] [VISUAL] drag feel and hit targets still need a human

## Work Log

### 2026-09-12T02:33:13.449Z - EditorOverlay + EditableFrame + snap.ts, and the app wiring. Exactly one op per gesture END via runOnJS; gesture state is Reanimated shared values on the UI thread and is a transient OFFSET from the server's rect, never a second copy of it, so the frame always returns to what the server says. On acceptance the offset is dropped instantly rather than animated - animating the return would race the incoming delta and read as a wobble; on rejection nothing was sent, so the slide back IS the feedback. Drag and resize are SIBLING gestures, not nested: nesting would put two Pans on one touch and need explicit arbitration. GestureHandlerRootView now wraps the app root with flex:1 - it was absent, and without it every gesture silently does nothing on every platform including web. Host gating uses game.players[userId].host, the same flag the server's layout handler authorizes on, so the button appears exactly when the action would be accepted; it is a UX gate only and says so. TWO BUGS I FIXED after the agent died mid-run (API error): snap.ts and snap.node-test.ts imported '../../layout/grid/coords.ts' one directory too shallow from components/board/editor/, and the board route had been left unwired to the palette and panel. I also added selection - a Tap raced against the Pan, since a Pan needs movement and a Tap needs none, so neither costs the other its first frames - and wired Palette and ConfigPanel into the overlay, without which stories 030 and 031 were unreachable and unbundled. Verified all of it reaches the web bundle by grepping the emitted output.


### 2026-09-12T02:33:29.554Z - Proof completeness set PROVEN: 7 of 8 criteria checked; criterion 8 is [VISUAL] and rejected with a reason. 270/270 tests, typecheck clean, lint down to 5 warnings.

### 2026-09-12T02:33:29.632Z - Proof feature-availability set PROVEN: GestureHandlerRootView now wraps the app root; without it every gesture silently does nothing on every platform. Verified the editor reaches the web bundle by grepping the emitted output for EditorOverlay, Palette, ConfigPanel, snapMove and snapResize.

### 2026-09-12T02:33:29.710Z - Proof robustness set PROVEN: A move that fails canPlace snaps back and is not sent, reusing setPlacement's return value rather than duplicating the check. The frame is a transient offset from the server's rect, so it can never hold a second copy of state.

### 2026-09-12T02:33:29.856Z - Proof resilience set PROVEN: Exactly ONE op on gesture end, never per frame - a per-frame op would make every connected client deep-clone and replay the whole game state once per frame of a drag. Grid lines are suppressed past 4096 cells, where 8190 line views would render as a solid block anyway.

### 2026-09-12T02:33:29.936Z - Proof security set PROVEN: Host gating uses game.players[userId].host, the same flag the server authorizes on, and is documented as a UX gate only - a non-host who calls the action anyway is rejected server-side.

### 2026-09-12T02:33:30.015Z - Proof input-validation set PROVEN: snapMove and snapResize clamp at grid edges and floor the span at one cell; the pure snapping math is unit-tested including the clamping cases.

### 2026-09-12T02:33:30.096Z - Proof defense-in-depth set PROVEN: Local canPlace, then the Go per-op decoder, then validateLayout on the resulting document.

### 2026-09-12T02:33:30.177Z - Proof thread-safety set PROVEN: Gesture state stays on the UI thread in shared values; only the committed rect crosses to JS via runOnJS, once, at the end.

### 2026-09-12T02:33:30.256Z - Proof configurability set NOT_APPLICABLE: Driven by layout data and the registry; no separate configuration surface.

---
id: 031-a610
title: Config panel drawn from field descriptors
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.968Z"
updated: "2026-09-12T02:34:47.215Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 7
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
started_at: "2026-09-12T01:39:13.195Z"
completed_at: "2026-09-12T02:34:47.215Z"
---

# Config panel drawn from field descriptors

## Problem Statement

Each widget declares field descriptors so a dashboard can auto-draw its configuration panel. No off-the-shelf schema-to-form renderer works in React Native: rjsf and autoform both emit DOM elements, so a hand-rolled descriptor-to-component registry is the only real option.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] A mapped type over FieldDescriptor['kind'] maps every kind to a picker, so adding a kind without a picker is a COMPILE error rather than a runtime surprise
- [x] A value edited through a descriptor produces a setWidgetConfig payload that validates against the widget's own schema
- [x] An invalid value is reported and NOT sent
- [x] Text and number edits are debounced (about 250ms) into a single op, since per-keystroke ops are a delta storm
- [x] The sub-grid widget's grid and widgets keys currently have no honest descriptor kind; either add a composite kind or document why they stay special-cased
- [REJECTED] [VISUAL] Panel layout, label alignment and error display verified by a human ([VISUAL] Panel layout, label alignment and error display need a human. Note every picker is still a placeholder until story 032, so a visual pass is more useful after that lands.)

## Files

- client/components/board/editor/config-panel.tsx
- client/layout/edit/panel.node-test.ts

## Proof

- [x] [completeness] Completeness (6 of 7 criteria checked; the [VISUAL] one is rejected with a reason. 270/270 tests, typecheck clean.)
- [x] [feature-availability] Feature availability (Confirmed present in the emitted web bundle after wiring into EditorOverlay.)
- [x] [robustness] Robustness (isDrawableField refuses any object-valued field and renders a read-only note, so a descriptor/value mismatch is visible rather than a broken control. The coalescer is held in a ref so a reconnect cannot discard a half-typed edit, and flushes on unmount.)
- [x] [resilience] Resilience (Text and number edits coalesce into one op; per-keystroke ops would make every connected client deep-clone and replay its whole game state per keystroke. Only the changed key is sent, since setWidgetConfig merges server-side.)
- [~] [security] Security (Client-side editing; the server re-validates every op and the panel cannot bypass it.)
- [x] [defense-in-depth] Defense in depth (The picker-coverage guarantee is enforced twice: a mapped type makes a missing picker a compile error, and a runtime test reads the actual PICKERS declaration out of the source rather than a copy.)
- [x] [input-validation] Input validation (Every edit is validated against the widget's own zod schema before emitting; an invalid value is reported and nothing reaches the socket, asserted. The panel is the one place a human types an arbitrary value.)
- [~] [thread-safety] Thread safety (React component state plus one debounce timer; no shared mutable state.)
- [x] [configurability] Configurability (The panel is generated entirely from each widget's field descriptors, so a new widget gets a panel with no edit to this file.)

## QA

- [ ] 270/270 tests, typecheck clean, lint 5 warnings (all pre-existing)
- [ ] ConfigPanel confirmed in the emitted web bundle
- [ ] [VISUAL] deferred - every picker is a placeholder until story 032

## Work Log

### 2026-09-12T02:34:45.975Z - config-panel.tsx + panel.ts, 19 tests. PICKERS is a mapped type over FieldDescriptor['kind'], so adding a kind without a picker is a COMPILE error - the whole reason descriptors are hand-written rather than derived from zod. The runtime half of that guarantee is the interesting part: the PICKERS table cannot be imported into a bare-Node test because it lives in a .tsx that pulls in react-native, so the test READS the declaration out of the source file. Asserting against a copy of the table in the test would have proved nothing about the panel. On the sub-grid descriptor problem the agent chose neither of the two routes I offered and found a better third: rather than special-casing 'subgrid', isDrawableField refuses ANY object-valued field (except a multiselect array) and the panel renders a read-only note. That catches every descriptor/value mismatch - a kind:'text' over a number too - so the key is visibly not-editable-here instead of silently absent, and it needs no new descriptor kind and no edit to registry.node-test.ts's exhaustiveness map. Debouncing is scoped to text and number only, because every other kind produces a whole value per deliberate gesture and debouncing a tap is just lag. The coalescer is reached through a ref rather than a dependency so a reconnect or scene switch cannot discard a half-typed edit, and it flushes on unmount so closing the panel does not swallow the last keystroke. Two fixes I made after the agent stalled: panel.node-test.ts passed a DOM URL where node's readFileSync wants a path (react-native's lib URL does not unify with node:url's), replaced with import.meta.dirname + join, and a duplicate node:path import removed.


### 2026-09-12T02:34:46.544Z - Proof completeness set PROVEN: 6 of 7 criteria checked; the [VISUAL] one is rejected with a reason. 270/270 tests, typecheck clean.

### 2026-09-12T02:34:46.625Z - Proof input-validation set PROVEN: Every edit is validated against the widget's own zod schema before emitting; an invalid value is reported and nothing reaches the socket, asserted. The panel is the one place a human types an arbitrary value.

### 2026-09-12T02:34:46.705Z - Proof defense-in-depth set PROVEN: The picker-coverage guarantee is enforced twice: a mapped type makes a missing picker a compile error, and a runtime test reads the actual PICKERS declaration out of the source rather than a copy.

### 2026-09-12T02:34:46.785Z - Proof robustness set PROVEN: isDrawableField refuses any object-valued field and renders a read-only note, so a descriptor/value mismatch is visible rather than a broken control. The coalescer is held in a ref so a reconnect cannot discard a half-typed edit, and flushes on unmount.

### 2026-09-12T02:34:46.859Z - Proof resilience set PROVEN: Text and number edits coalesce into one op; per-keystroke ops would make every connected client deep-clone and replay its whole game state per keystroke. Only the changed key is sent, since setWidgetConfig merges server-side.

### 2026-09-12T02:34:46.932Z - Proof feature-availability set PROVEN: Confirmed present in the emitted web bundle after wiring into EditorOverlay.

### 2026-09-12T02:34:46.999Z - Proof security set NOT_APPLICABLE: Client-side editing; the server re-validates every op and the panel cannot bypass it.

### 2026-09-12T02:34:47.068Z - Proof thread-safety set NOT_APPLICABLE: React component state plus one debounce timer; no shared mutable state.

### 2026-09-12T02:34:47.134Z - Proof configurability set PROVEN: The panel is generated entirely from each widget's field descriptors, so a new widget gets a panel with no edit to this file.

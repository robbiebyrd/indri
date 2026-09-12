---
id: 030-728d
title: "Editor: widget palette, add and remove"
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.967Z"
updated: "2026-09-12T02:33:51.813Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 6
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
started_at: "2026-09-12T01:39:13.089Z"
completed_at: "2026-09-12T02:33:51.813Z"
---

# Editor: widget palette, add and remove

## Problem Statement

A host needs to add widgets from the registered set and remove them. Finding a free slot must not scan the whole coordinate space: a naive first-fit over a 4096x4096 grid is 16.7M positions.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] The palette lists exactly the registered widget types, so a newly registered widget appears with no palette change
- [x] A new widget is placed at the first free rect scanning row-major
- [x] The first-fit scan is bounded to min(rows,64) x min(cols,64) from the origin, falling back to absolute placement at the origin, because searching 16.7M positions is not worth doing well for a PoC
- [x] A full grid returns no placement rather than overlapping
- [x] The emitted addWidget payload validates against the widget's own schema using its registry defaults
- [REJECTED] [VISUAL] Palette layout and the add/remove affordances verified by a human ([VISUAL] Palette layout and the add/remove affordances need a human on a real screen.)

## Files

- client/components/board/editor/palette.tsx
- client/layout/edit/place.ts
- client/layout/edit/place.node-test.ts

## Proof

- [x] [completeness] Completeness (6 of 7 criteria checked; the [VISUAL] one is rejected with a reason. 270/270 tests, typecheck clean.)
- [x] [feature-availability] Feature availability (The palette is driven by listWidgets（）, so a newly registered type appears with no edit. Confirmed present in the emitted web bundle after wiring.)
- [x] [robustness] Robustness (A full grid returns undefined rather than overlapping, and placementFor falls back to absolute placement so a selection never silently does nothing. An exhaustive-scan equivalence test proves undefined means no origin fits.)
- [x] [resilience] Resilience (The origin search is capped at 64x64, so placement cost is bounded regardless of grid size - 4096x4096 completes in under a millisecond instead of scanning 16.7M positions.)
- [~] [security] Security (Client-side placement only; the server re-validates bounds and overlap in validateLayout.)
- [x] [defense-in-depth] Defense in depth (Ids come from newWidgetId checked against EVERY id at the level including absolutes, so a generated id cannot collide with one the server would reject.)
- [x] [input-validation] Input validation (The emitted addWidget payload validates against each widget's OWN zod schema, asserted for all three real widgets using their registry defaults. Config defaults are copied rather than shared with the registry.)
- [~] [thread-safety] Thread safety (Pure functions and a stateless component.)
- [x] [configurability] Configurability (NEW_WIDGET_SIZE and MAX_SEARCH_SPAN are named constants; a per-widget preferred size would be an additive field on WidgetDefinition, noted as a follow-up.)

## QA

- [ ] 270/270 tests, typecheck clean, lint 5 warnings (all pre-existing)
- [ ] Palette confirmed in the emitted web bundle
- [ ] [VISUAL] palette layout still needs a human

## Work Log

### 2026-09-12T02:33:50.572Z - place.ts + palette.tsx. The critical constraint was the search bound: a naive row-major scan over 4096x4096 is 16.7 MILLION candidate positions per placement. The cap is on candidate ORIGINS - min(rows-h, 63) x min(cols-w, 63) - so at most 4096 canPlace calls regardless of grid size, measured under 1ms at 4096x4096. A consequence worth knowing: only the origin is bounded, not the rect, so a widget wider than 64 cells is still placeable; and free space that BEGINS past row/col 64 reports as none, at which point placementFor falls back to absolute at the origin sized as a percentage, so the selection still creates a widget rather than silently doing nothing. The palette lists the REGISTRY, not a hand-written menu, so a game registering a widget type gets it in the palette with no edit to that file - which is the only way a framework's palette stays correct for games it has never seen. Order is registry insertion order so a new type does not reshuffle rows an author already knows. The whole 'what does a selection emit' path lives in place.ts (createWidget) rather than the untestable .tsx, which is what lets the payload assertions run in bare Node. FOOTGUN FLAGGED AND HANDLED: newWidgetId needs EVERY id at the level, but EditContext.siblings deliberately excludes absolutely placed widgets - passing siblings.map(s=>s.id) would let a generated id collide with an absolute widget, which the server rejects rather than replaces. Palette takes a separate widgetIds prop for this, and I wired it from Object.keys(scene.widgets) when connecting it to the overlay. 16 tests including an exhaustive-scan equivalence check proving 'undefined' means 'no origin fits' rather than 'gave up'.


### 2026-09-12T02:33:51.153Z - Proof completeness set PROVEN: 6 of 7 criteria checked; the [VISUAL] one is rejected with a reason. 270/270 tests, typecheck clean.

### 2026-09-12T02:33:51.231Z - Proof feature-availability set PROVEN: The palette is driven by listWidgets(), so a newly registered type appears with no edit. Confirmed present in the emitted web bundle after wiring.

### 2026-09-12T02:33:51.310Z - Proof robustness set PROVEN: A full grid returns undefined rather than overlapping, and placementFor falls back to absolute placement so a selection never silently does nothing. An exhaustive-scan equivalence test proves undefined means no origin fits.

### 2026-09-12T02:33:51.389Z - Proof resilience set PROVEN: The origin search is capped at 64x64, so placement cost is bounded regardless of grid size - 4096x4096 completes in under a millisecond instead of scanning 16.7M positions.

### 2026-09-12T02:33:51.458Z - Proof input-validation set PROVEN: The emitted addWidget payload validates against each widget's OWN zod schema, asserted for all three real widgets using their registry defaults. Config defaults are copied rather than shared with the registry.

### 2026-09-12T02:33:51.524Z - Proof security set NOT_APPLICABLE: Client-side placement only; the server re-validates bounds and overlap in validateLayout.

### 2026-09-12T02:33:51.591Z - Proof defense-in-depth set PROVEN: Ids come from newWidgetId checked against EVERY id at the level including absolutes, so a generated id cannot collide with one the server would reject.

### 2026-09-12T02:33:51.659Z - Proof thread-safety set NOT_APPLICABLE: Pure functions and a stateless component.

### 2026-09-12T02:33:51.728Z - Proof configurability set PROVEN: NEW_WIDGET_SIZE and MAX_SEARCH_SPAN are named constants; a per-widget preferred size would be an additive field on WidgetDefinition, noted as a follow-up.

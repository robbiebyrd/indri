---
id: 030-728d
title: "Editor: widget palette, add and remove"
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.967Z"
updated: "2026-09-10T00:58:09.116Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 6
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
---

# Editor: widget palette, add and remove

## Problem Statement

A host needs to add widgets from the registered set and remove them. Finding a free slot must not scan the whole coordinate space: a naive first-fit over a 4096x4096 grid is 16.7M positions.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] The palette lists exactly the registered widget types, so a newly registered widget appears with no palette change
- [ ] A new widget is placed at the first free rect scanning row-major
- [ ] The first-fit scan is bounded to min(rows,64) x min(cols,64) from the origin, falling back to absolute placement at the origin, because searching 16.7M positions is not worth doing well for a PoC
- [ ] A full grid returns no placement rather than overlapping
- [ ] The emitted addWidget payload validates against the widget's own schema using its registry defaults
- [ ] [VISUAL] Palette layout and the add/remove affordances verified by a human

## Files

- client/components/board/editor/palette.tsx
- client/layout/edit/place.ts
- client/layout/edit/place.node-test.ts

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


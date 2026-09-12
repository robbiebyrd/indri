---
id: 032-0be8
title: Field pickers for every descriptor kind
status: in_progress
priority: P2
type: feature
created: "2026-09-10T00:58:00.969Z"
updated: "2026-09-12T02:35:49.726Z"
dependencies: ["031"]
plan: plans/layout-authoring-editor.md
plan_step: Step 8
depends_on: ["stories/031-a610-pending-P2-config-panel-drawn-from-field-descriptors.md"]
started_at: "2026-09-12T02:35:49.726Z"
---

# Field pickers for every descriptor kind

## Problem Statement

Eight descriptor kinds need cross-platform controls. There is no DOM, and the repo already has a dependency-free Select built for exactly this reason, so no picker library should be added.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] Pickers exist for text, number, color, boolean, date, select, multiselect and uri
- [ ] components/display/select.tsx is reused for select and extended for multiselect rather than adding a picker library
- [ ] Parse and format logic lives in pure helpers beside the components so it is testable without a renderer, matching the core/renderer split used throughout Plan A
- [ ] Tested pure helpers cover: colour parse and normalise (#rgb to #rrggbb, invalid rejected), number clamp to min/max, date ISO round-trip, multiselect dedupe and order stability
- [ ] No DOM inputs anywhere
- [ ] [VISUAL] Every picker verified by a human on web, iOS and Android, especially colour and date which have no shared cross-platform primitive

## Files

- client/components/board/editor/pickers/
- client/components/display/select.tsx

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


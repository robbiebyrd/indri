---
id: 025-6b2f
title: "Layout op envelope: types and decoding"
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.941Z"
updated: "2026-09-10T00:58:08.591Z"
dependencies: []
plan: plans/layout-authoring-editor.md
plan_step: Step 1
---

# Layout op envelope: types and decoding

## Problem Statement

The layout action is one action with many ops rather than one action per operation, so the wire payload needs a decoded envelope before anything can validate or apply it. Keeping it one action keeps the protocol surface and the Storer interface small, and lets a single Mutate plus events.Diff produce granular deltas for free.

## Acceptance Criteria

- [ ] VERIFY: go test ./internal/handlers/actions/layout/
- [ ] An Op struct decodes every op in the plan's vocabulary table: addWidget, removeWidget, setPlacement, setWidgetConfig, setStyle, setGrid, setScript
- [ ] A missing required field produces a specific error naming that field, not a generic decode failure
- [ ] An unknown op value is rejected
- [ ] An unknown top-level key is rejected
- [ ] setPlacement covers both move and resize, since they write the same field and a separate resizeWidget op would be two names for one write
- [ ] Errors wrap with %w and lowercase context per repo convention
- [ ] Tests are table-driven, matching the style of internal/services/mutation/mutation_test.go

## Files

- internal/handlers/actions/layout/op.go
- internal/handlers/actions/layout/op_test.go

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


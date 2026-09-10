---
id: 026-0ae1
title: Structural layout validation in Go
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.963Z"
updated: "2026-09-10T00:58:08.711Z"
dependencies: ["025"]
plan: plans/layout-authoring-editor.md
plan_step: Step 2
depends_on: ["stories/025-6b2f-pending-P2-layout-op-envelope-types-and-decoding.md"]
---

# Structural layout validation in Go

## Problem Statement

Game data is map[string]interface{} with zero server-side schema, so the Go handler is the only place the overlap and bounds rules can be enforced authoritatively. The client-side validator from Plan A is UX feedback; the client is not a security boundary. This duplication across two languages is unavoidable and intentional.

## Acceptance Criteria

- [ ] VERIFY: go test -race ./internal/handlers/actions/layout/
- [ ] Table-driven tests cover: grid dims below 8 and above 4096, negative/zero/fractional spans, a rect exceeding grid bounds, two overlapping non-absolute widgets, two overlapping ABSOLUTE widgets (allowed), a pixel value in absolute placement (rejected), sub-grid at depth 4 (allowed) and 5 (rejected), a privateData key nested three levels deep (rejected), a widget count over 300, a serialised layout over 256KB
- [ ] Validation runs AFTER the op has been applied in memory, because only the resulting document reveals whether an overlap now exists
- [ ] Absolute placements are excluded from the overlap set as both subject and obstacle
- [ ] The AABB comparison and the 8..4096 / depth / count constants are kept textually identical to the TypeScript versions so a future reader sees the deliberate pair
- [ ] Widget config contents are NOT validated: the server does not know what a text widget is, and structural validation is the whole remit

## Files

- internal/handlers/actions/layout/validate.go
- internal/handlers/actions/layout/validate_test.go

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


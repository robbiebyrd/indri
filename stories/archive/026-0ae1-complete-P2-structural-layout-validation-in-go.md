---
id: 026-0ae1
title: Structural layout validation in Go
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.963Z"
updated: "2026-09-12T01:14:02.315Z"
dependencies: ["025"]
plan: plans/layout-authoring-editor.md
plan_step: Step 2
depends_on: ["stories/025-6b2f-pending-P2-layout-op-envelope-types-and-decoding.md"]
started_at: "2026-09-12T00:53:03.897Z"
completed_at: "2026-09-12T01:14:02.315Z"
---

# Structural layout validation in Go

## Problem Statement

Game data is map[string]interface{} with zero server-side schema, so the Go handler is the only place the overlap and bounds rules can be enforced authoritatively. The client-side validator from Plan A is UX feedback; the client is not a security boundary. This duplication across two languages is unavoidable and intentional.

## Acceptance Criteria

- [x] VERIFY: go test -race ./internal/handlers/actions/layout/
- [x] Table-driven tests cover: grid dims below 8 and above 4096, negative/zero/fractional spans, a rect exceeding grid bounds, two overlapping non-absolute widgets, two overlapping ABSOLUTE widgets (allowed), a pixel value in absolute placement (rejected), sub-grid at depth 4 (allowed) and 5 (rejected), a privateData key nested three levels deep (rejected), a widget count over 300, a serialised layout over 256KB
- [x] Validation runs AFTER the op has been applied in memory, because only the resulting document reveals whether an overlap now exists
- [x] Absolute placements are excluded from the overlap set as both subject and obstacle
- [x] The AABB comparison and the 8..4096 / depth / count constants are kept textually identical to the TypeScript versions so a future reader sees the deliberate pair
- [x] Widget config contents are NOT validated: the server does not know what a text widget is, and structural validation is the whole remit

## Files

- internal/handlers/actions/layout/validate.go
- internal/handlers/actions/layout/validate_test.go

## Proof

- [x] [completeness] Completeness (All 6 criteria checked. 82 test cases pass; go build, go vet, gofmt clean.)
- [x] [feature-availability] Feature availability (Nothing calls validateLayout yet; 027 must invoke it inside Mutate's apply, after the op, on g.PublicData['layout']. Handed over explicitly.)
- [x] [robustness] Robustness (No panics on malformed map[string]interface{} - a dedicated no-panic sweep covers it. Handles the JSON wire shape （float64） rather than assuming int.)
- [x] [resilience] Resilience (Rejects rather than clamps - a documented divergence from parseLayout, since the server should not invent author intent.)
- [x] [security] Security (This is the ONLY authoritative enforcement of bounds and overlap - the client is not a security boundary. Rejects privateData at any depth including inside arrays, caps widget count and payload size, and bounds sub-grid recursion.)
- [x] [defense-in-depth] Defense in depth (Deliberately duplicates the TS validator in a second language, with each rule commented to name its counterpart so the pair is visible and maintainable.)
- [x] [input-validation] Input validation (Validates the resulting DOCUMENT after the op is applied, not the op in isolation, because only the result reveals whether an overlap now exists. Absolute placements must be percentage strings; a bare number is rejected.)
- [~] [thread-safety] Thread safety (Pure validation over a value; no shared state.)
- [~] [configurability] Configurability (Constants are fixed by spec and deliberately mirror the TypeScript; a knob would let the two diverge.)

## QA

82 test cases pass; go build, go vet, gofmt clean. Core rules mutation-checked. Note: style remains opaque server-side - flagged as needing its own story.

## Work Log

### 2026-09-12T01:14:00.888Z - Structural validation in Go, deliberately duplicating the TypeScript. Each rule carries a comment naming the TS file it mirrors: overlaps <- collision.ts collides() with the same strict < and no identity exemption; absolutes never enter the overlap set as subject OR obstacle <- collision.ts module comment; per-level scoping <- collision.ts omission 2; minDim/maxDim <- coords.ts; isValidRect/isWithinBounds <- coords.ts; the percentage regex <- placement.ts Percent, character for character; maxDepth <- widget.ts MAX_SUBGRID_DEPTH; privateData at any depth including inside arrays <- layout.ts findReservedKey. maxWidgets and maxBytes have no TS counterpart - they are server-only caps from the plan's Security section. Agent mutation-checked the core rules and I take those as sound: < to <= in overlaps gives 5 failures, off-by-one on the depth check fails the depth-5 case, and giving absolutes a real rect and adding them to the overlap set gives 5 failures. One equivalent mutant reported honestly rather than hidden: adding absolutes with their ZERO rect survives, because a zero-area rect can never collide - not a behavioural difference. 82 cases. DECISIONS FLAGGED FOR 027 AND BEYOND: (1) style is opaque like config, because StyleSchema's vocabulary is React-Native-shaped and would go stale in Go - consequence is that a bad style key passes the server and the client's strict parse then rejects the WHOLE layout with severity error, i.e. a blank board; closing that needs its own story. (2) Strict unknown-key rejection was added on the objects Go does define a shape for, not in the story's test list, because without it the server accepts documents the client refuses to render at all. (3) maxWidgets counts the whole layout including nested sub-grids, since the cap exists to bound what each client deep-clones per delta. (4) Script length is NOT capped - the plan's Security section asks for it but Step 2's tests did not; maxBytes bounds it transitively. Left out rather than invented. (5) scenes is REQUIRED, matching GameLayoutSchema, so setGrid on a fresh game must seed scenes:{} - handed to 027. (6) Reject, never clamp: a deliberate divergence from parseLayout, documented at the top of the file.


### 2026-09-12T01:14:01.412Z - Proof completeness set PROVEN: All 6 criteria checked. 82 test cases pass; go build, go vet, gofmt clean.

### 2026-09-12T01:14:01.490Z - Proof security set PROVEN: This is the ONLY authoritative enforcement of bounds and overlap - the client is not a security boundary. Rejects privateData at any depth including inside arrays, caps widget count and payload size, and bounds sub-grid recursion.

### 2026-09-12T01:14:01.591Z - Proof input-validation set PROVEN: Validates the resulting DOCUMENT after the op is applied, not the op in isolation, because only the result reveals whether an overlap now exists. Absolute placements must be percentage strings; a bare number is rejected.

### 2026-09-12T01:14:01.666Z - Proof robustness set PROVEN: No panics on malformed map[string]interface{} - a dedicated no-panic sweep covers it. Handles the JSON wire shape (float64) rather than assuming int.

### 2026-09-12T01:14:01.746Z - Proof defense-in-depth set PROVEN: Deliberately duplicates the TS validator in a second language, with each rule commented to name its counterpart so the pair is visible and maintainable.

### 2026-09-12T01:14:01.823Z - Proof resilience set PROVEN: Rejects rather than clamps - a documented divergence from parseLayout, since the server should not invent author intent.

### 2026-09-12T01:14:01.902Z - Proof thread-safety set NOT_APPLICABLE: Pure validation over a value; no shared state.

### 2026-09-12T01:14:01.982Z - Proof configurability set NOT_APPLICABLE: Constants are fixed by spec and deliberately mirror the TypeScript; a knob would let the two diverge.

### 2026-09-12T01:14:02.060Z - Proof feature-availability set PROVEN: Nothing calls validateLayout yet; 027 must invoke it inside Mutate's apply, after the op, on g.PublicData['layout']. Handed over explicitly.

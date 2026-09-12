---
id: 025-6b2f
title: "Layout op envelope: types and decoding"
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.941Z"
updated: "2026-09-12T01:13:39.144Z"
dependencies: []
plan: plans/layout-authoring-editor.md
plan_step: Step 1
started_at: "2026-09-12T00:24:27.034Z"
completed_at: "2026-09-12T01:13:39.144Z"
---

# Layout op envelope: types and decoding

## Problem Statement

The layout action is one action with many ops rather than one action per operation, so the wire payload needs a decoded envelope before anything can validate or apply it. Keeping it one action keeps the protocol surface and the Storer interface small, and lets a single Mutate plus events.Diff produce granular deltas for free.

## Acceptance Criteria

- [x] VERIFY: go test ./internal/handlers/actions/layout/
- [x] An Op struct decodes every op in the plan's vocabulary table: addWidget, removeWidget, setPlacement, setWidgetConfig, setStyle, setGrid, setScript
- [x] A missing required field produces a specific error naming that field, not a generic decode failure
- [x] An unknown op value is rejected
- [x] An unknown top-level key is rejected
- [x] setPlacement covers both move and resize, since they write the same field and a separate resizeWidget op would be two names for one write
- [x] Errors wrap with %w and lowercase context per repo convention
- [x] Tests are table-driven, matching the style of internal/services/mutation/mutation_test.go

## Files

- internal/handlers/actions/layout/op.go
- internal/handlers/actions/layout/op_test.go

## Proof

- [x] [completeness] Completeness (All 8 criteria checked. 50 test cases pass; go build, go vet and gofmt clean.)
- [x] [feature-availability] Feature availability (All seven ops in the plan's vocabulary decode, including all three scopes for setStyle and setScript.)
- [x] [robustness] Robustness (No panics on any malformed map[string]interface{}; every failure is a returned error. Input is untrusted wire data, so defensive type assertions throughout.)
- [~] [resilience] Resilience (Pure decoding; no IO, no external dependency.)
- [x] [security] Security (Decoding only; authorization is story 027. Unknown-key rejection means a typo'd field cannot be silently ignored, which is the same reasoning as the client schema's .strict（）.)
- [x] [defense-in-depth] Defense in depth (Structural decoding here, semantic validation in 026, authorization in 027 - three separate gates rather than one combined check.)
- [x] [input-validation] Input validation (This IS the input boundary for the action. Per-op allowlists reject unknown keys, wrong types, empty required strings and invalid scope combinations. Every failure names the offending field or op.)
- [~] [thread-safety] Thread safety (Pure function over a map; no shared state.)
- [~] [configurability] Configurability (The op vocabulary is the protocol and is deliberately fixed.)

## QA

50 test cases pass; go build, go vet, gofmt clean. Note: the package is not registered until 027, so the repo-wide coverage test is red until then by design.

## Work Log

### 2026-09-12T01:13:37.467Z - Op envelope for the single layout action. opSpecs is the source of truth: each op declares its required and optional wire field names, and decodeFields rejects any key the spec does not allow before filling anything. Two keys are exempt because they belong to the message rather than an op: op (the discriminator) and code (read separately by RequireGameCode in 027); action needs no exemption since DecodeMessageWithAction already deletes it. Side effect worth knowing: strictness is PER-OP, not global - setGrid carrying a sceneId is rejected as an unknown field, which catches 'right field, wrong op' rather than just typos. Decisions where the spec left room: an empty required string is treated as missing (sceneId: '' names the field rather than silently accepting an empty id), with source exempt since an empty script is meaningful - that is why it is a *string. An explicit JSON null for a required field is a type error rather than a missing field; arguable, so it is pinned by a test. Scope violations split by direction: a scope missing an address it needs names the field, a scope carrying one it must not have reports the scope. Sentinels stay unexported because everything that classifies a decode failure lives in this package; transports only propagate. 50 cases. Rejection cases assert BOTH errors.Is on the sentinel and that the message text names the field, since a sentinel alone would not catch a generic message. One case pins that resizeWidget is NOT an op, per the plan's explicit instruction not to add one.


### 2026-09-12T01:13:38.158Z - Proof completeness set PROVEN: All 8 criteria checked. 50 test cases pass; go build, go vet and gofmt clean.

### 2026-09-12T01:13:38.241Z - Proof input-validation set PROVEN: This IS the input boundary for the action. Per-op allowlists reject unknown keys, wrong types, empty required strings and invalid scope combinations. Every failure names the offending field or op.

### 2026-09-12T01:13:38.321Z - Proof robustness set PROVEN: No panics on any malformed map[string]interface{}; every failure is a returned error. Input is untrusted wire data, so defensive type assertions throughout.

### 2026-09-12T01:13:38.395Z - Proof security set PROVEN: Decoding only; authorization is story 027. Unknown-key rejection means a typo'd field cannot be silently ignored, which is the same reasoning as the client schema's .strict().

### 2026-09-12T01:13:38.462Z - Proof defense-in-depth set PROVEN: Structural decoding here, semantic validation in 026, authorization in 027 - three separate gates rather than one combined check.

### 2026-09-12T01:13:38.534Z - Proof resilience set NOT_APPLICABLE: Pure decoding; no IO, no external dependency.

### 2026-09-12T01:13:38.605Z - Proof thread-safety set NOT_APPLICABLE: Pure function over a map; no shared state.

### 2026-09-12T01:13:38.675Z - Proof configurability set NOT_APPLICABLE: The op vocabulary is the protocol and is deliberately fixed.

### 2026-09-12T01:13:38.751Z - Proof feature-availability set PROVEN: All seven ops in the plan's vocabulary decode, including all three scopes for setStyle and setScript.

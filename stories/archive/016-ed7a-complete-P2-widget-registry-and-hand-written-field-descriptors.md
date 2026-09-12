---
id: 016-ed7a
title: Widget registry and hand-written field descriptors
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.221Z"
updated: "2026-09-10T00:01:18.942Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 6
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
started_at: "2026-09-09T23:51:54.999Z"
completed_at: "2026-09-10T00:01:18.942Z"
---

# Widget registry and hand-written field descriptors

## Problem Statement

Each widget must declare its config shape so a dashboard can auto-draw a configuration panel. Zod introspection is not viable: v4 moved ._def to ._zod.def and broke real consumers, and zod's own library-author guidance forbids reaching into internals. Descriptors are therefore hand-written, and a bidirectional test is what keeps them in sync with the validation schema.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] FieldDescriptor is a discriminated union over kinds: text, number, color, boolean, date, select, multiselect, uri
- [x] WidgetDefinition carries type, zod schema, fields, defaults, Component and an api() returning the functions Lua may call
- [x] A test asserts bidirectionally that every key in a widget's config schema has a matching field descriptor AND every descriptor has a matching schema key
- [x] Registering a duplicate widget type throws
- [x] Each widget's defaults validate against its own schema
- [x] No code introspects zod internals (._def or ._zod.def) anywhere
- [x] The uri field kind accepts an accept discriminator of image or video, so adding a video widget later needs no schema change

## Files

- client/layout/registry/fields.ts
- client/layout/registry/registry.ts
- client/layout/registry/registry.node-test.ts

## Proof

- [x] [completeness] Completeness (All 8 criteria checked. 87/87 tests, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (knownWidgetTypes（） plugs into parseLayout's optional set; an unknown type produces a warning, asserted end to end.)
- [x] [robustness] Robustness (getWidget on an unregistered type returns undefined rather than throwing. knownWidgetTypes returns a fresh copy so callers cannot mutate the registry through it. Mutation-checked: neutering the assertion helper produced 7 failures.)
- [~] [resilience] Resilience (No IO or external dependency; failure modes are programmer errors surfaced at registration or test time.)
- [~] [security] Security (Pure in-memory registry over developer-authored definitions; no wire data, no IO, no eval.)
- [x] [defense-in-depth] Defense in depth (Two independent guards on descriptor/schema drift: a compile-time Record<FieldKind, true> exhaustiveness map and a runtime assertion, so a new field kind fails the build AND the tests.)
- [x] [input-validation] Input validation (Duplicate widget type throws; duplicate field-descriptor key within a definition throws; defaults must validate against the definition's own schema; a non-strict schema is rejected by the strictness sentinel rather than silently passing.)
- [~] [thread-safety] Thread safety (Single-threaded module state populated at startup; clearRegistry exists only for test isolation.)
- [x] [configurability] Configurability (The registry IS the extension point; listWidgets preserves insertion order so a UI built on it does not reshuffle.)

## QA

87/87 tests, typecheck clean, zero new lint warnings. Mutation-checked: neutering assertDescriptorsMatchSchema produced 7 failures.

## Work Log

### 2026-09-10T00:01:16.376Z - fields.ts (FieldDescriptor union + FieldKind) and registry.ts (WidgetDefinition, register/get/list/knownWidgetTypes/clearRegistry, assertDescriptorsMatchSchema). 18 tests. Component and api() are deliberately OMITTED from WidgetDefinition - they belong to 017/018 and 020, and empty placeholders now would be speculative; both are purely additive later. The interesting part is enumerating config keys WITHOUT touching zod internals: descriptor->schema is exact and behavioural (safeParse a probe object and read the public unrecognized_keys issue), guarded by a strictness sentinel so the check cannot go vacuous on a non-strict schema; schema->descriptor uses Object.keys(defaults), which is pinned rather than free-floating because a strict schema cannot parse defaults unless every REQUIRED key is present and none is fictitious. Documented limit: an optional key absent from BOTH descriptors and defaults is invisible - it must be forgotten twice. Constraint that lands on story 018: C extends Record<string, unknown>, so widget configs must be derived via z.infer, not declared as an interface (TS only gives implicit index signatures to type aliases). Verified the tests are not vacuous: neutering assertDescriptorsMatchSchema produced 7 failures, then restored to 87/87.


### 2026-09-10T00:01:17.584Z - Proof completeness set PROVEN: All 8 criteria checked. 87/87 tests, typecheck clean, zero new lint warnings.

### 2026-09-10T00:01:17.728Z - Proof feature-availability set PROVEN: knownWidgetTypes() plugs into parseLayout's optional set; an unknown type produces a warning, asserted end to end.

### 2026-09-10T00:01:17.869Z - Proof input-validation set PROVEN: Duplicate widget type throws; duplicate field-descriptor key within a definition throws; defaults must validate against the definition's own schema; a non-strict schema is rejected by the strictness sentinel rather than silently passing.

### 2026-09-10T00:01:18.006Z - Proof robustness set PROVEN: getWidget on an unregistered type returns undefined rather than throwing. knownWidgetTypes returns a fresh copy so callers cannot mutate the registry through it. Mutation-checked: neutering the assertion helper produced 7 failures.

### 2026-09-10T00:01:18.159Z - Proof configurability set PROVEN: The registry IS the extension point; listWidgets preserves insertion order so a UI built on it does not reshuffle.

### 2026-09-10T00:01:18.271Z - Proof defense-in-depth set PROVEN: Two independent guards on descriptor/schema drift: a compile-time Record<FieldKind, true> exhaustiveness map and a runtime assertion, so a new field kind fails the build AND the tests.

### 2026-09-10T00:01:18.476Z - Proof security set NOT_APPLICABLE: Pure in-memory registry over developer-authored definitions; no wire data, no IO, no eval.

### 2026-09-10T00:01:18.616Z - Proof resilience set NOT_APPLICABLE: No IO or external dependency; failure modes are programmer errors surfaced at registration or test time.

### 2026-09-10T00:01:18.777Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded module state populated at startup; clearRegistry exists only for test isolation.

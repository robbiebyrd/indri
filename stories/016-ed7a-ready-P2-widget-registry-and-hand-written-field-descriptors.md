---
id: 016-ed7a
title: Widget registry and hand-written field descriptors
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.221Z"
updated: "2026-09-09T19:05:27.499Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 6
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
---

# Widget registry and hand-written field descriptors

## Problem Statement

Each widget must declare its config shape so a dashboard can auto-draw a configuration panel. Zod introspection is not viable: v4 moved ._def to ._zod.def and broke real consumers, and zod's own library-author guidance forbids reaching into internals. Descriptors are therefore hand-written, and a bidirectional test is what keeps them in sync with the validation schema.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] FieldDescriptor is a discriminated union over kinds: text, number, color, boolean, date, select, multiselect, uri
- [ ] WidgetDefinition carries type, zod schema, fields, defaults, Component and an api() returning the functions Lua may call
- [ ] A test asserts bidirectionally that every key in a widget's config schema has a matching field descriptor AND every descriptor has a matching schema key
- [ ] Registering a duplicate widget type throws
- [ ] Each widget's defaults validate against its own schema
- [ ] No code introspects zod internals (._def or ._zod.def) anywhere
- [ ] The uri field kind accepts an accept discriminator of image or video, so adding a video widget later needs no schema change

## Files

- client/layout/registry/fields.ts
- client/layout/registry/registry.ts
- client/layout/registry/registry.node-test.ts

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


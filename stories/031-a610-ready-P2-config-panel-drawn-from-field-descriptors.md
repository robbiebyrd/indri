---
id: 031-a610
title: Config panel drawn from field descriptors
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.968Z"
updated: "2026-09-10T00:58:09.218Z"
dependencies: ["028"]
plan: plans/layout-authoring-editor.md
plan_step: Step 7
depends_on: ["stories/028-7ac1-pending-P2-client-layout-mutation-sender.md"]
---

# Config panel drawn from field descriptors

## Problem Statement

Each widget declares field descriptors so a dashboard can auto-draw its configuration panel. No off-the-shelf schema-to-form renderer works in React Native: rjsf and autoform both emit DOM elements, so a hand-rolled descriptor-to-component registry is the only real option.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] A mapped type over FieldDescriptor['kind'] maps every kind to a picker, so adding a kind without a picker is a COMPILE error rather than a runtime surprise
- [ ] A value edited through a descriptor produces a setWidgetConfig payload that validates against the widget's own schema
- [ ] An invalid value is reported and NOT sent
- [ ] Text and number edits are debounced (about 250ms) into a single op, since per-keystroke ops are a delta storm
- [ ] The sub-grid widget's grid and widgets keys currently have no honest descriptor kind; either add a composite kind or document why they stay special-cased
- [ ] [VISUAL] Panel layout, label alignment and error display verified by a human

## Files

- client/components/board/editor/config-panel.tsx
- client/layout/edit/panel.node-test.ts

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


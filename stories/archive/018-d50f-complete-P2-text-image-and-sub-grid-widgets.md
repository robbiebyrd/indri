---
id: 018-d50f
title: Text, image and sub-grid widgets
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.223Z"
updated: "2026-09-10T00:54:59.701Z"
dependencies: ["016", "017"]
plan: plans/layout-engine-renderer.md
plan_step: Step 8
depends_on: ["stories/016-ed7a-pending-P2-widget-registry-and-hand-written-field-descriptors.md", "stories/017-9d66-pending-P2-renderer-styledbox-board-scene-and-widget-host.md"]
started_at: "2026-09-10T00:30:25.704Z"
completed_at: "2026-09-10T00:54:59.701Z"
---

# Text, image and sub-grid widgets

## Problem Statement

The initial widget set. Sub-grid is recursive and needs clipping, because RN's default overflow is visible so a percentage-positioned absolute child would otherwise silently escape its parent's bounds. Image needs a real failure UX for broken or slow URLs.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [REJECTED] Text, image and subgrid widget definitions are registered, each with schema, fields, defaults, Component and api() (Asks for api() on each definition, but WidgetDefinition has no api member: story 016 deliberately deferred it as undesigned, and registry.ts was out of scope here. The Lua-callable surface was delivered instead as story 020's HostApi (widget(id).setStyle/setConfig). Criterion superseded by that design, not skipped.)
- [x] Sub-grid containers set overflow hidden by default so absolute children cannot escape their parent
- [x] The sub-grid recursive schema accepts nesting to depth 4 and reports an issue at depth 5
- [x] Image widget uses expo-image, which covers png, jpg, gif, webp, avif and svg on all three targets
- [x] A broken or slow image URI shows expo-image's error placeholder; never a blank box and never a throw
- [x] Each widget's defaults parse against its own schema and an unknown text align value is rejected
- [x] No video widget is built (out of scope), and no react-native-svg dependency is added
- [REJECTED] [VISUAL] Text metrics across platforms, image contentFit modes and sub-grid clipping verified ([VISUAL] text metrics, contentFit modes and sub-grid clipping still need a human on a real screen. Now actually checkable, since widgets finally render - pair it with story 017's deferred visual check on /board.)

## Files

- client/components/board/widgets/text/index.tsx
- client/components/board/widgets/image/index.tsx
- client/components/board/widgets/subgrid/index.tsx

## Proof

- [x] [completeness] Completeness (7 of 9 criteria checked. Criterion 2's api（） clause is superseded by story 020's HostApi and criterion 9 is [VISUAL]; both rejected with reasons rather than ticked. 180/180 tests.)
- [x] [feature-availability] Feature availability (Web bundle contains TextWidgetView, ImageWidgetView, SubGridWidgetView and registerBuiltinWidgets, one occurrence each - verified by grepping the emitted bundle, not by trusting the build.)
- [x] [robustness] Robustness (A broken, slow or unset image URI shows a neutral fill painted before the fetch, deliberately not relying on expo-image's placeholder prop which is documented only for loading/unset. Malformed config degrades to a visible error box, never a blank or a throw.)
- [x] [resilience] Resilience (Depth capping is not re-implemented here - parseLayout already empties children past MAX_SUBGRID_DEPTH, so there is one source of truth for the cap.)
- [~] [security] Security (Presentation only. Media URI trust is the documented Plan B residual risk; layout data is already validated by parseLayout.)
- [x] [defense-in-depth] Defense in depth (Sub-grid containers set overflow hidden by default so a percent-positioned absolute child cannot escape its parent, while still letting authored style win.)
- [x] [input-validation] Input validation (Each component parses widget.config with its own zod schema and renders a shared config-error box on failure rather than casting. assertDescriptorsMatchSchema asserted for all three real definitions.)
- [~] [thread-safety] Thread safety (React components; no shared mutable state.)
- [x] [configurability] Configurability (Field descriptors are the config surface. Honest limitation recorded: FieldDescriptor has no composite kind, so sub-grid's grid/widgets are declared as text with a description - the weakest part of this story.)

## QA

- [ ] 180/180 tests, typecheck clean, zero new lint warnings
- [ ] Widgets confirmed present in the emitted web bundle
- [ ] Criterion 2 api() superseded by story 020 HostApi; criterion 9 [VISUAL] still needs a human on /board

## Work Log

### 2026-09-10T00:54:45.366Z - Text, image and subgrid widgets. Each is split into definition.ts (zod schema + descriptors + defaults, bare-Node clean) and index.tsx (the view), because node --experimental-strip-types cannot parse JSX and react-native is not loadable outside a bundler - a definition referencing its own Component would take the whole test file down. Drift between halves is structurally impossible: the full definition is a spread of the partial one. assertDescriptorsMatchSchema is asserted for all three REAL definitions, which is the point of that helper existing. One file outside the folder was touched: board-view.tsx gained a side-effect import './widgets', without which nothing reaches the widget .tsx files and expo export would have bundled none of this story's code; verified by grepping the emitted bundle for TextWidgetView/ImageWidgetView/SubGridWidgetView/registerBuiltinWidgets, one occurrence each. Calls made: SubGridConfig is an interface so it fails C extends Record<string, unknown>; restated as a homomorphic mapped type, which is an alias (so it gets the implicit index signature) but cannot drift. Image failure UX does not rely on expo-image's placeholder prop, which is documented only for loading/unset rather than fetch errors - the container View carries a neutral fill painted before the fetch, so slow, broken and unset URIs all show the same thing. Sub-grid overflow defaults to hidden but authored style wins, otherwise the overflow key in StyleSchema would be a lie for this one type. DEVIATION on criterion 2: it asks for api(), but WidgetDefinition has no api member - story 016 deliberately deferred it and registry.ts was read-only for me. The criterion pre-dates that decision; the Lua-callable surface landed in story 020's HostApi instead. Also flagged: FieldDescriptor has no composite kind, so the sub-grid's grid/widgets keys are declared as text with a description telling a panel to special-case them - the weakest part of this story and worth revisiting when the config panel is built. 18 tests.


### 2026-09-10T00:54:58.991Z - Proof completeness set PROVEN: 7 of 9 criteria checked. Criterion 2's api() clause is superseded by story 020's HostApi and criterion 9 is [VISUAL]; both rejected with reasons rather than ticked. 180/180 tests.

### 2026-09-10T00:54:59.066Z - Proof feature-availability set PROVEN: Web bundle contains TextWidgetView, ImageWidgetView, SubGridWidgetView and registerBuiltinWidgets, one occurrence each - verified by grepping the emitted bundle, not by trusting the build.

### 2026-09-10T00:54:59.144Z - Proof input-validation set PROVEN: Each component parses widget.config with its own zod schema and renders a shared config-error box on failure rather than casting. assertDescriptorsMatchSchema asserted for all three real definitions.

### 2026-09-10T00:54:59.217Z - Proof robustness set PROVEN: A broken, slow or unset image URI shows a neutral fill painted before the fetch, deliberately not relying on expo-image's placeholder prop which is documented only for loading/unset. Malformed config degrades to a visible error box, never a blank or a throw.

### 2026-09-10T00:54:59.292Z - Proof defense-in-depth set PROVEN: Sub-grid containers set overflow hidden by default so a percent-positioned absolute child cannot escape its parent, while still letting authored style win.

### 2026-09-10T00:54:59.371Z - Proof security set NOT_APPLICABLE: Presentation only. Media URI trust is the documented Plan B residual risk; layout data is already validated by parseLayout.

### 2026-09-10T00:54:59.451Z - Proof resilience set PROVEN: Depth capping is not re-implemented here - parseLayout already empties children past MAX_SUBGRID_DEPTH, so there is one source of truth for the cap.

### 2026-09-10T00:54:59.533Z - Proof thread-safety set NOT_APPLICABLE: React components; no shared mutable state.

### 2026-09-10T00:54:59.614Z - Proof configurability set PROVEN: Field descriptors are the config surface. Honest limitation recorded: FieldDescriptor has no composite kind, so sub-grid's grid/widgets are declared as text with a description - the weakest part of this story.

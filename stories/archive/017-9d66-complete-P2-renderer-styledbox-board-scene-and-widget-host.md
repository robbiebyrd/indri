---
id: 017-9d66
title: "Renderer: StyledBox, board, scene and widget host"
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.222Z"
updated: "2026-09-10T00:29:05.789Z"
dependencies: ["015"]
plan: plans/layout-engine-renderer.md
plan_step: Step 7
depends_on: ["stories/015-dd9e-pending-P2-layout-schema-with-defensive-non-throwing-parse.md"]
started_at: "2026-09-10T00:15:17.150Z"
completed_at: "2026-09-10T00:29:05.789Z"
---

# Renderer: StyledBox, board, scene and widget host

## Problem Statement

The compiled style and percentage geometry need RN components to render them. Because every delta produces a fresh deep-cloned game object, unkeyed children would re-reconcile the entire widget tree on every websocket message, so memoisation and stable ids are a correctness-adjacent performance requirement rather than an optimisation.

## Acceptance Criteria

- [x] StyledBox paints compiled background layers as absolutely-filled siblings beneath children, since RN has no background-image property
- [x] The board container is position relative and every widget is absolutely positioned with a percentage box; there is no flex layout inside the grid
- [x] WidgetHost is wrapped in React.memo and keyed by widget id
- [x] stage.currentScene selects which layout scene entry renders
- [x] Issues returned by parseLayout render into a dev-only overlay, never to the player
- [x] A route exists at client/app/board/index.tsx that renders a layout end to end
- [REJECTED] [VISUAL] Gradient rendering, border radii, background-layer ordering and percentage box alignment verified at both 8x8 and 4096x4096 ([VISUAL] and genuinely unverified: no widgets are registered until story 018, so the board renders empty and a visual check would prove nothing. Must be verified by a human on /board after 018 lands.)
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- client/components/board/styled-box.tsx
- client/components/board/board-view.tsx
- client/components/board/scene-view.tsx
- client/components/board/widget-host.tsx
- client/app/board/index.tsx

## Proof

- [x] [completeness] Completeness (7 of 8 criteria checked; criterion 7 is [VISUAL] and explicitly rejected rather than ticked, pending a human look after story 018 registers widgets. 114/114 tests, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (Web AND iOS bundles export clean with the component tree; the /board static route emits 19.2 kB and renders its empty state, confirmed by grepping the exported HTML rather than trusting the build.)
- [x] [robustness] Robustness (An unregistered widget type or one without a Component renders a visible placeholder, never nothing and never a crash. The route distinguishes 'no game' from 'game has no layout' instead of a blank screen. A degenerate grid producing a non-percentage dimension collapses to 0 rather than silently stretching a widget across its parent.)
- [x] [resilience] Resilience (parseLayout never throws and its issues surface in a __DEV__-only overlay; a malformed layout degrades to a reported issue with the board still rendering.)
- [~] [security] Security (Presentation only. Layout data is already sanitized server-side and validated by parseLayout; media URI trust is a documented Plan B residual risk.)
- [~] [defense-in-depth] Defense in depth (Rendering layer; validation lives in parseLayout （015） and, for authoring, in the Go handler （Plan B）.)
- [x] [input-validation] Input validation (parseLayout is called with knownWidgetTypes（） so unknown types are reported; placement values are narrowed before reaching RN style props.)
- [~] [thread-safety] Thread safety (React components; no shared mutable state outside React's own model.)
- [~] [configurability] Configurability (Driven entirely by layout data; no separate configuration surface.)

## QA

- [ ] 114/114 tests, typecheck clean, zero new lint warnings
- [ ] Web and iOS bundles export with the component tree; /board renders its empty state
- [ ] [VISUAL] criterion 7 NOT verified - deferred to a human check after story 018 registers widgets

## Work Log

### 2026-09-10T00:28:51.410Z - StyledBox / WidgetHost / SceneView / BoardView plus the /board route, and one additive change to registry.ts adding an optional Component and WidgetRenderProps (016 left it out as speculative; it is needed now). Type-only React imports, so registry tests still run in bare Node. Two pure helpers extracted and unit-tested since no React renderer exists: placement.ts (grid -> toPercentBox, absolute passed straight through) and layers.ts (gradient tuple coercion, resizeMode -> contentFit). 12 tests. Notable finds by the agent, both kept: (1) toPercentBox returns plain string which is not assignable to RN's DimensionValue, so a degenerate grid could produce 'Infinity%' - narrowed with a regex that collapses non-percentages to 0, because RN silently ignores a bad dimension and would leave the widget stretched across its parent; (2) expo-image has no equivalent of resizeMode 'repeat' (contentFit is object-fit shaped and cannot tile), so it degrades to 'none' and that is pinned by a test as a decision rather than a bug. Verified beyond unit tests: web AND iOS bundles export clean with the component tree, and the /board static route renders its empty state rather than blanking. Criterion 7 is [VISUAL] and deliberately NOT ticked - nothing is registered to render until story 018, so a look now would show an empty board. It is rejected with a reason and should be verified once 018 lands. Known limitation recorded: BoardView's memo will rarely bail out because GameStateParser deep-clones the whole game per message, giving data.layout fresh identity every time; id-keying still preserves component instances. That is the delta-retention issue from story 023, not something this step can fix.


### 2026-09-10T00:29:05.065Z - Proof completeness set PROVEN: 7 of 8 criteria checked; criterion 7 is [VISUAL] and explicitly rejected rather than ticked, pending a human look after story 018 registers widgets. 114/114 tests, typecheck clean, zero new lint warnings.

### 2026-09-10T00:29:05.140Z - Proof feature-availability set PROVEN: Web AND iOS bundles export clean with the component tree; the /board static route emits 19.2 kB and renders its empty state, confirmed by grepping the exported HTML rather than trusting the build.

### 2026-09-10T00:29:05.218Z - Proof robustness set PROVEN: An unregistered widget type or one without a Component renders a visible placeholder, never nothing and never a crash. The route distinguishes 'no game' from 'game has no layout' instead of a blank screen. A degenerate grid producing a non-percentage dimension collapses to 0 rather than silently stretching a widget across its parent.

### 2026-09-10T00:29:05.296Z - Proof resilience set PROVEN: parseLayout never throws and its issues surface in a __DEV__-only overlay; a malformed layout degrades to a reported issue with the board still rendering.

### 2026-09-10T00:29:05.372Z - Proof security set NOT_APPLICABLE: Presentation only. Layout data is already sanitized server-side and validated by parseLayout; media URI trust is a documented Plan B residual risk.

### 2026-09-10T00:29:05.453Z - Proof input-validation set PROVEN: parseLayout is called with knownWidgetTypes() so unknown types are reported; placement values are narrowed before reaching RN style props.

### 2026-09-10T00:29:05.533Z - Proof defense-in-depth set NOT_APPLICABLE: Rendering layer; validation lives in parseLayout (015) and, for authoring, in the Go handler (Plan B).

### 2026-09-10T00:29:05.612Z - Proof thread-safety set NOT_APPLICABLE: React components; no shared mutable state outside React's own model.

### 2026-09-10T00:29:05.697Z - Proof configurability set NOT_APPLICABLE: Driven entirely by layout data; no separate configuration surface.

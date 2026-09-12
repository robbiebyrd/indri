---
id: 028-7ac1
title: Client layout mutation sender
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.965Z"
updated: "2026-09-12T01:32:35.317Z"
dependencies: ["027"]
plan: plans/layout-authoring-editor.md
plan_step: Step 4
depends_on: ["stories/027-7e5e-pending-P1-host-only-layout-action-handler-reachable-from-bot.md"]
started_at: "2026-09-12T01:30:11.704Z"
completed_at: "2026-09-12T01:32:35.317Z"
---

# Client layout mutation sender

## Problem Statement

The editor needs to emit layout ops over the existing socket. A local canPlace check avoids a doomed round trip, but it is UX only: the server re-validates and is the authority, so nothing may be applied optimistically.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] Each op builder emits exactly the wire shape in the plan's vocabulary table
- [x] A move that fails canPlace is NOT sent
- [x] A move onto an absolute widget IS sent, because absolute widgets never block placement
- [x] A send before the socket opens is dropped with a warning, matching MessageHandler.send's existing behaviour
- [x] Nothing is applied optimistically; the delta is the only confirmation
- [x] Widget ids for addWidget are generated client-side as short stable strings and never collide with an existing id in the scene

## Files

- client/layout/edit/ops.ts
- client/layout/edit/ops.node-test.ts

## Proof

- [x] [completeness] Completeness (All 7 criteria checked. 219/219 tests, typecheck clean, lint unchanged at 6 warnings.)
- [x] [feature-availability] Feature availability (Absolute placements always send, since absolutes are exempt from collision as both subject and obstacle - asserted by a move onto space an absolute widget occupies.)
- [x] [robustness] Robustness (Mutation-verified: removing the collision guard fails two tests. A move does not collide with the widget's own current position, which is exactly what a drag does. newWidgetId cannot collide with an existing id.)
- [x] [resilience] Resilience (A send before the socket opens is dropped with a warning by MessageHandler.send, which these builders delegate to rather than reimplementing.)
- [x] [security] Security (The local canPlace check is explicitly documented as UX only, not a boundary; validateLayout in Go is authoritative. Nothing is applied optimistically, so a client cannot diverge from server state by trusting its own edit.)
- [x] [defense-in-depth] Defense in depth (Three gates before a layout changes: this local check, the Go per-op decoder, and validateLayout on the resulting document.)
- [x] [input-validation] Input validation (Wire shapes asserted with deepStrictEqual against the Go decoder's per-op allowlists, including the board-scope rule that neither id may be present. A widget-scoped op with no widgetId throws rather than sending a malformed message.)
- [~] [thread-safety] Thread safety (Pure builders over a socket; no shared mutable state.)
- [~] [configurability] Configurability (The op vocabulary is the protocol and is fixed by the server.)

## QA

219/219 tests, typecheck clean, lint unchanged. Collision guard mutation-verified (2 failures without it).

## Work Log

### 2026-09-12T01:32:33.945Z - Builders for the layout action, written to mirror internal/handlers/actions/layout/op.go exactly - its decoder rejects unknown keys PER OP, so an extra or misspelled field is a rejected edit rather than something ignored. That is why the wire-shape tests are deepStrictEqual rather than partial matches. Nothing is applied optimistically: the published delta is the only confirmation, so the editing host learns about its own edit exactly the way every other player does. A test asserts the builders never mutate the context they were given. The local canPlace check is UX only and documented as such - it avoids a doomed round trip, and validateLayout in Go remains the boundary. Mutation-verified: removing the guard fails 'a move that collides is not sent' and 'an out-of-bounds move is not sent'. Two subtleties the Go decoder forced: a board-scoped setStyle/setScript must carry NEITHER sceneId nor widgetId (sending one unconditionally would fail every board edit), and an empty script source must still be sent because that is how a script is cleared. A widget-scoped op with no widgetId throws rather than sending a malformed message - a programming error, not a runtime one. newWidgetId checks against the ids already present rather than trusting randomness, because addWidget on an existing id is an error server-side rather than a replace; ids stay short and alphanumeric since they appear in dotted delta paths. 12 tests, 219 total.


### 2026-09-12T01:32:34.585Z - Proof completeness set PROVEN: All 7 criteria checked. 219/219 tests, typecheck clean, lint unchanged at 6 warnings.

### 2026-09-12T01:32:34.664Z - Proof input-validation set PROVEN: Wire shapes asserted with deepStrictEqual against the Go decoder's per-op allowlists, including the board-scope rule that neither id may be present. A widget-scoped op with no widgetId throws rather than sending a malformed message.

### 2026-09-12T01:32:34.743Z - Proof security set PROVEN: The local canPlace check is explicitly documented as UX only, not a boundary; validateLayout in Go is authoritative. Nothing is applied optimistically, so a client cannot diverge from server state by trusting its own edit.

### 2026-09-12T01:32:34.823Z - Proof robustness set PROVEN: Mutation-verified: removing the collision guard fails two tests. A move does not collide with the widget's own current position, which is exactly what a drag does. newWidgetId cannot collide with an existing id.

### 2026-09-12T01:32:34.902Z - Proof feature-availability set PROVEN: Absolute placements always send, since absolutes are exempt from collision as both subject and obstacle - asserted by a move onto space an absolute widget occupies.

### 2026-09-12T01:32:34.983Z - Proof resilience set PROVEN: A send before the socket opens is dropped with a warning by MessageHandler.send, which these builders delegate to rather than reimplementing.

### 2026-09-12T01:32:35.060Z - Proof defense-in-depth set PROVEN: Three gates before a layout changes: this local check, the Go per-op decoder, and validateLayout on the resulting document.

### 2026-09-12T01:32:35.144Z - Proof thread-safety set NOT_APPLICABLE: Pure builders over a socket; no shared mutable state.

### 2026-09-12T01:32:35.226Z - Proof configurability set NOT_APPLICABLE: The op vocabulary is the protocol and is fixed by the server.

---
id: 028-7ac1
title: Client layout mutation sender
status: ready
priority: P2
type: feature
created: "2026-09-10T00:58:00.965Z"
updated: "2026-09-10T00:58:08.914Z"
dependencies: ["027"]
plan: plans/layout-authoring-editor.md
plan_step: Step 4
depends_on: ["stories/027-7e5e-pending-P1-host-only-layout-action-handler-reachable-from-bot.md"]
---

# Client layout mutation sender

## Problem Statement

The editor needs to emit layout ops over the existing socket. A local canPlace check avoids a doomed round trip, but it is UX only: the server re-validates and is the authority, so nothing may be applied optimistically.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] Each op builder emits exactly the wire shape in the plan's vocabulary table
- [ ] A move that fails canPlace is NOT sent
- [ ] A move onto an absolute widget IS sent, because absolute widgets never block placement
- [ ] A send before the socket opens is dropped with a warning, matching MessageHandler.send's existing behaviour
- [ ] Nothing is applied optimistically; the delta is the only confirmation
- [ ] Widget ids for addWidget are generated client-side as short stable strings and never collide with an existing id in the scene

## Files

- client/layout/edit/ops.ts
- client/layout/edit/ops.node-test.ts

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


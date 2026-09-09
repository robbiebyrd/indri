---
id: 021-9370
title: Lua to WebSocket bridge
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.224Z"
updated: "2026-09-09T19:05:28.053Z"
dependencies: ["020"]
plan: plans/layout-engine-renderer.md
plan_step: Step 11
depends_on: ["stories/020-2b59-pending-P2-lua-host-api-presentation-override-layer-and-event.md"]
---

# Lua to WebSocket bridge

## Problem Statement

Lua needs to send interaction events to the server and receive game-specific messages back, closing the loop through the existing delta pipeline. MessageHandler already accepts extra parsers, so this needs no change to its internals.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] indri.send(action, payload) emits the correct wire shape through MessageHandler.send, verified with a fake socket
- [ ] An inbound message dispatches to registered Lua handlers
- [ ] An unknown action is ignored rather than thrown
- [ ] A send before the socket opens is dropped with a warning, matching MessageHandler.send's existing behaviour
- [ ] The bridge registers via the existing parsers array extension point, with no modification to MessageHandler internals
- [ ] The framework reserves no Lua-specific action names; games name their own actions as tic-tac-toe's move does
- [ ] Payloads reject cyclic tables and tables deeper than 8 levels, rather than letting JSON.stringify throw inside a Lua callback

## Files

- client/layout/lua/bridge.ts
- client/layout/lua/bridge.node-test.ts
- client/services/message-handler.ts

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


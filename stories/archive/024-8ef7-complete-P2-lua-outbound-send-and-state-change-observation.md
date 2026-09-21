---
id: 024-8ef7
title: Lua outbound send and state-change observation
status: complete
priority: P2
type: feature
created: "2026-09-09T19:22:29.957Z"
updated: "2026-09-16T04:03:51.396Z"
dependencies: ["020"]
plan: plans/layout-engine-renderer.md
plan_step: Step 11
depends_on: ["stories/020-2b59-ready-P2-lua-host-api-presentation-override-layer-and-event.md"]
completed_at: "2026-09-16T04:03:51.395Z"
---

# Lua outbound send and state-change observation

## Problem Statement

Lua is asymmetric by design: scripts SEND actions to the server, but they do not receive websocket messages. Inbound information reaches a script only as an observed change to the reduced game state. This keeps one source of truth and stops a script desynchronising from authoritative state by interpreting a raw message differently from the reducer.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] indri.send(action, payload) emits the correct wire shape through MessageHandler.send, verified with a fake socket
- [x] A send before the socket opens is dropped with a warning, matching MessageHandler.send existing behaviour
- [x] There is NO inbound message dispatch to Lua and NO new parser registered in MessageHandler.parsers
- [x] A new reduced game state fires stateChanged exactly once per applied update
- [x] stateChanged fires from the already-reduced gameState rather than the raw delta, so a script never observes a partially applied update
- [x] The framework reserves no action names; a game names its own outbound actions exactly as tic-tac-toe move does
- [x] Payloads reject cyclic tables and tables nested deeper than 8 levels instead of letting JSON.stringify throw inside a Lua callback

## Files

- client/layout/lua/bridge.ts
- client/layout/lua/bridge.node-test.ts

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-09-16T04:03:51.308Z - Implemented bridge.ts (toPlainJson + attachBridge) and bridge.node-test.ts (8 tests). Fixed host-api.ts indri.send to use lua_next traversal and switched fengari-interop to createRequire. All 121 tests pass.


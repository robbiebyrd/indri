---
id: 020-2b59
title: Lua host API, presentation override layer and event bubbling
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.224Z"
updated: "2026-09-16T03:58:39.706Z"
dependencies: ["019"]
plan: plans/layout-engine-renderer.md
plan_step: Step 10
depends_on: ["stories/019-c6b2-pending-P2-lua-runtime-load-run-and-guard-chunks.md"]
completed_at: "2026-09-16T03:58:39.705Z"
---

# Lua host API, presentation override layer and event bubbling

## Problem Statement

Lua must drive behaviour without owning state. The server's delta stream is authoritative, so Lua writes go to a local override layer composited over the server layout at render time, and the only path to server state is indri.send. Without this separation a script and the delta stream would fight each other.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] HostApi exposes send, state, board.setStyle, scene.setStyle, widget(id) setters, on and log, and nothing else
- [x] widget(id).setText writes to the override map and NOT to game state, asserted by a test
- [x] state() returns a deep-frozen snapshot, so a script cannot mutate authoritative state through a live reference
- [x] Overrides are cleared on keyframe only; a delta does not clear them
- [x] Event bubbling runs widget then scene then game, and stops when a handler returns false
- [x] A handler registered for a removed widget is dropped
- [x] Re-entrancy is handled: a host callback invoked from inside a Lua handler does not start a second dispatch
- [x] One long-lived Lua state per session with registered callbacks; scripts are NOT re-evaluated on every delta
- [x] A script changed by a delta re-instantiates that scope's chunk and discards its prior globals

## Files

- client/layout/lua/host-api.ts
- client/layout/lua/overrides.ts
- client/layout/lua/events.ts
- client/layout/lua/host-api.node-test.ts

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

### 2026-09-16T03:58:35.386Z - Implemented LuaSession with OverrideMap, EventRegistry, onKeyframe/onStateChange lifecycle, emit with re-entrancy guard, and installHostApi. All 9 tests pass.


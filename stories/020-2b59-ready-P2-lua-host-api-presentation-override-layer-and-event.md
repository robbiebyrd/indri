---
id: 020-2b59
title: Lua host API, presentation override layer and event bubbling
status: ready
priority: P2
type: feature
created: "2026-09-09T19:04:59.224Z"
updated: "2026-09-09T19:05:27.938Z"
dependencies: ["019"]
plan: plans/layout-engine-renderer.md
plan_step: Step 10
depends_on: ["stories/019-c6b2-pending-P2-lua-runtime-load-run-and-guard-chunks.md"]
---

# Lua host API, presentation override layer and event bubbling

## Problem Statement

Lua must drive behaviour without owning state. The server's delta stream is authoritative, so Lua writes go to a local override layer composited over the server layout at render time, and the only path to server state is indri.send. Without this separation a script and the delta stream would fight each other.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] HostApi exposes send, state, board.setStyle, scene.setStyle, widget(id) setters, on and log, and nothing else
- [ ] widget(id).setText writes to the override map and NOT to game state, asserted by a test
- [ ] state() returns a deep-frozen snapshot, so a script cannot mutate authoritative state through a live reference
- [ ] Overrides are cleared on keyframe only; a delta does not clear them
- [ ] Event bubbling runs widget then scene then game, and stops when a handler returns false
- [ ] A handler registered for a removed widget is dropped
- [ ] Re-entrancy is handled: a host callback invoked from inside a Lua handler does not start a second dispatch
- [ ] One long-lived Lua state per session with registered callbacks; scripts are NOT re-evaluated on every delta
- [ ] A script changed by a delta re-instantiates that scope's chunk and discards its prior globals

## Files

- client/layout/lua/host-api.ts
- client/layout/lua/overrides.ts
- client/layout/lua/events.ts
- client/layout/lua/host-api.node-test.ts

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


---
id: 020-2b59
title: Lua host API, presentation override layer and event bubbling
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.224Z"
updated: "2026-09-10T00:55:28.950Z"
dependencies: ["019"]
plan: plans/layout-engine-renderer.md
plan_step: Step 10
depends_on: ["stories/019-c6b2-pending-P2-lua-runtime-load-run-and-guard-chunks.md"]
started_at: "2026-09-10T00:30:25.811Z"
completed_at: "2026-09-10T00:55:28.950Z"
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

- [x] [completeness] Completeness (All 10 criteria checked. 180/180 tests, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (One long-lived state with registered callbacks; scripts are not re-evaluated per delta. instantiate（scope, src） is the entry point for a script changed by a delta.)
- [x] [robustness] Robustness (Each handler runs in its own top-level guard, so one script's error or budget exhaustion cannot stop the chain; errors are collected and reported. Re-entrancy is guarded and tested. A handler for a removed widget is dropped.)
- [x] [resilience] Resilience (Overrides cleared on keyframe ONLY, never on a delta - both paths tested. mergeOverrides does not mutate the parsed layout, which is shared with the renderer.)
- [x] [security] Security (fengari-interop rejected because interop proxies expose the JS prototype chain （obj.constructor.constructor reaches the Function constructor and escapes the sandbox）; plain lua_pushcfunction used, no JS object visible to scripts. state（） is a proxy table with __newindex raiser and __metatable set, because __newindex is skipped for existing keys and a plain table would let state.code = 'HACKED' succeed silently. Independently mutation-verified: disabling the barrier fails a test. Six distinct write attacks covered.)
- [x] [defense-in-depth] Defense in depth (Layers: libraries absent from the bundle, dangerous globals nil'd, per-scope _ENV, read-only proxy over state, per-handler instruction budget, and style validated with StyleSchema at the Lua boundary. Residual channel documented: _G is still writable.)
- [x] [input-validation] Input validation (Style patches from Lua go through StyleSchema.safeParse at the boundary where untrusted input arrives. Payload marshalling caps depth at 8, which doubles as cycle detection so JSON.stringify never throws inside a callback; state marshalling uses a separate depth of 32 since truncating authoritative state would silently lie.)
- [~] [thread-safety] Thread safety (Single Lua state, synchronous on the JS thread. Re-entrancy is the analogous hazard here and is explicitly guarded.)
- [x] [configurability] Configurability (HostDeps injects send, activeSceneId and log, so this module has no dependency on MessageHandler; story 024 supplies the real socket.)

## QA

180/180 tests, typecheck clean, zero new lint warnings. Read-only state barrier independently mutation-verified. Residual cross-script channel via _G documented, not fixed.

## Work Log

### 2026-09-10T00:55:12.755Z - host-api.ts / overrides.ts / events.ts, 48 tests. The architectural point holds: Lua writes go to an override layer composited over the server layout at render time, never to game state, so scripts and the delta stream cannot fight. Two security findings worth keeping. (1) fengari-interop was REJECTED, not merely unused: its value is handing JS objects to Lua as proxies, and an interop-wrapped object exposes its JS prototype chain, so obj.constructor.constructor reaches the Function constructor and walks straight out of the sandbox state.ts spends fifty lines building. Plain lua_pushcfunction instead; no JS object is ever visible to a script, only marshalled values. (2) state() had to be a PROXY table, not the data table with a __newindex barrier, because __newindex is skipped for a key that already exists - state.code = 'HACKED' on a plain table would have succeeded silently. __metatable is set so the protection cannot be swapped out, and state.ts had already removed rawset, which is the other way around a metatable. I independently mutation-checked this: disabling the __newindex barrier fails a test. Design calls beyond the brief: bubbling alone breaks stateChanged (a scene or widget script would never hear it if the target were the board), so the bus has dispatch (targeted, bubbles, stoppable) for widgetPress AND broadcast (every scope, deliberately not stoppable, so no script can suppress another's copy of an announcement) for stateChanged/sceneChanged. Widget overrides are keyed scene-first because ids are unique per scene and a flat map would leak one scene's title patch onto another's. Each handler runs in its own top-level guard so one greedy script cannot starve the handlers above it. Two real fengari bugs found: lua_getmetatable returns a boolean not 0/1 (every table marshal was failing), and a try/finally settop inside a host function pops the error message luaL_error just pushed, so every raised error reached the client as 'lua error with no string representation'. RESIDUAL ISSUE FLAGGED, NOT FIXED: per-scope _ENV isolates globals, but _G itself is still writable, so _G.x = 1 remains a deliberate cross-script channel.


### 2026-09-10T00:55:28.225Z - Proof completeness set PROVEN: All 10 criteria checked. 180/180 tests, typecheck clean, zero new lint warnings.

### 2026-09-10T00:55:28.304Z - Proof security set PROVEN: fengari-interop rejected because interop proxies expose the JS prototype chain (obj.constructor.constructor reaches the Function constructor and escapes the sandbox); plain lua_pushcfunction used, no JS object visible to scripts. state() is a proxy table with __newindex raiser and __metatable set, because __newindex is skipped for existing keys and a plain table would let state.code = 'HACKED' succeed silently. Independently mutation-verified: disabling the barrier fails a test. Six distinct write attacks covered.

### 2026-09-10T00:55:28.387Z - Proof defense-in-depth set PROVEN: Layers: libraries absent from the bundle, dangerous globals nil'd, per-scope _ENV, read-only proxy over state, per-handler instruction budget, and style validated with StyleSchema at the Lua boundary. Residual channel documented: _G is still writable.

### 2026-09-10T00:55:28.458Z - Proof input-validation set PROVEN: Style patches from Lua go through StyleSchema.safeParse at the boundary where untrusted input arrives. Payload marshalling caps depth at 8, which doubles as cycle detection so JSON.stringify never throws inside a callback; state marshalling uses a separate depth of 32 since truncating authoritative state would silently lie.

### 2026-09-10T00:55:28.536Z - Proof robustness set PROVEN: Each handler runs in its own top-level guard, so one script's error or budget exhaustion cannot stop the chain; errors are collected and reported. Re-entrancy is guarded and tested. A handler for a removed widget is dropped.

### 2026-09-10T00:55:28.617Z - Proof resilience set PROVEN: Overrides cleared on keyframe ONLY, never on a delta - both paths tested. mergeOverrides does not mutate the parsed layout, which is shared with the renderer.

### 2026-09-10T00:55:28.696Z - Proof feature-availability set PROVEN: One long-lived state with registered callbacks; scripts are not re-evaluated per delta. instantiate(scope, src) is the entry point for a script changed by a delta.

### 2026-09-10T00:55:28.776Z - Proof thread-safety set NOT_APPLICABLE: Single Lua state, synchronous on the JS thread. Re-entrancy is the analogous hazard here and is explicitly guarded.

### 2026-09-10T00:55:28.856Z - Proof configurability set PROVEN: HostDeps injects send, activeSceneId and log, so this module has no dependency on MessageHandler; story 024 supplies the real socket.

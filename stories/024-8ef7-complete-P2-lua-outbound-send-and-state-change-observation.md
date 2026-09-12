---
id: 024-8ef7
title: Lua outbound send and state-change observation
status: complete
priority: P2
type: feature
created: "2026-09-09T19:22:29.957Z"
updated: "2026-09-12T00:40:46.330Z"
dependencies: ["020"]
plan: plans/layout-engine-renderer.md
plan_step: Step 11
depends_on: ["stories/020-2b59-ready-P2-lua-host-api-presentation-override-layer-and-event.md"]
started_at: "2026-09-12T00:24:26.768Z"
completed_at: "2026-09-12T00:40:46.330Z"
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

- [x] [completeness] Completeness (All 8 criteria checked. 196/196 tests, typecheck clean, zero new lint warnings, web bundle exports.)
- [x] [feature-availability] Feature availability (The bridge drives the REAL MessageHandler over a fake WebSocket in tests, not a stand-in, which is what makes the drop-before-open and parsers-unchanged assertions meaningful.)
- [x] [robustness] Robustness (A send before the socket opens is dropped with a warning. Each observer is wrapped so a throwing one cannot cost the next its notification. dispose（） is idempotent.)
- [x] [resilience] Resilience (Scripts are re-instantiated only when their source string differs; retain runs before instantiation so a dropped scope loses its handlers. Overrides clear on keyframe only, asserted through the bridge.)
- [x] [security] Security (action is spread last so a payload key named 'action' cannot rename the message; mutation-verified by inverting the spread, which fails a test. No inbound channel exists, so a script cannot be driven by a crafted message.)
- [x] [defense-in-depth] Defense in depth (Absence of an inbound path is the primary control; the action-spread guard is the second, in case a future change adds one.)
- [x] [input-validation] Input validation (Lua payloads are depth-capped at 8, which doubles as cycle detection so JSON.stringify never throws inside a callback. collectScripts shape-checks raw wire data rather than trusting it.)
- [~] [thread-safety] Thread safety (Single-threaded; observers fan out synchronously after the reducer dispatch.)
- [x] [configurability] Configurability (LuaBridgeOptions injects socket, state, log, onError and instructionBudget; no hard-coded dependency on MessageHandler.)

## QA

196/196 tests, typecheck clean, zero new lint warnings, web bundle exports. Action-spoofing guard independently mutation-verified. Gap flagged: nothing fires sceneChanged yet.

## Work Log

### 2026-09-12T00:40:44.925Z - LuaBridge + useLuaBridge hook + a small observer seam on MessageHandler. The asymmetry holds: scripts send outbound, and receive nothing - inbound information arrives only as stateChanged over the reduced game state. No parser was added to MessageHandler.parsers and a test pins that. Seam: updateGameState() is the single funnel both keyframe() and update() pass through, so it became updateGameState(kind) with an observer set fanned out AFTER the reducer dispatch over the same object. That kind argument is unavoidable - nothing in the reduced Game distinguishes a resync from an increment, so observing React's gameState alone cannot satisfy the overrides-clear rule. No Lua knowledge entered message-handler.ts; LuaBridge takes narrow structural interfaces (OutboundSocket, StateSource) that MessageHandler satisfies structurally, so the dependency runs one way. Security call worth keeping: send spreads action LAST ({...payload, action}), not first as the plan snippet had it, so a payload key named 'action' cannot rename the message. I mutation-verified this - inverting the spread fails a test. Ordering call: syncScripts runs before applyGameState, so a script introduced by an update hears the stateChanged that introduced it; the documented cost is that a chunk's top-level code sees the previous snapshot and anything it paints at top level is wiped by the keyframe's override clear. The alternative costs a new script its first state entirely, which is the harder bug to spot. GAP FLAGGED, not silently filled: nothing fires sceneChanged yet. The bridge is its natural owner but the story's deliverables did not mention it - needs picking up in 022. 16 tests, 196 total.


### 2026-09-12T00:40:45.617Z - Proof completeness set PROVEN: All 8 criteria checked. 196/196 tests, typecheck clean, zero new lint warnings, web bundle exports.

### 2026-09-12T00:40:45.696Z - Proof security set PROVEN: action is spread last so a payload key named 'action' cannot rename the message; mutation-verified by inverting the spread, which fails a test. No inbound channel exists, so a script cannot be driven by a crafted message.

### 2026-09-12T00:40:45.771Z - Proof input-validation set PROVEN: Lua payloads are depth-capped at 8, which doubles as cycle detection so JSON.stringify never throws inside a callback. collectScripts shape-checks raw wire data rather than trusting it.

### 2026-09-12T00:40:45.848Z - Proof robustness set PROVEN: A send before the socket opens is dropped with a warning. Each observer is wrapped so a throwing one cannot cost the next its notification. dispose() is idempotent.

### 2026-09-12T00:40:45.923Z - Proof resilience set PROVEN: Scripts are re-instantiated only when their source string differs; retain runs before instantiation so a dropped scope loses its handlers. Overrides clear on keyframe only, asserted through the bridge.

### 2026-09-12T00:40:46.005Z - Proof feature-availability set PROVEN: The bridge drives the REAL MessageHandler over a fake WebSocket in tests, not a stand-in, which is what makes the drop-before-open and parsers-unchanged assertions meaningful.

### 2026-09-12T00:40:46.081Z - Proof defense-in-depth set PROVEN: Absence of an inbound path is the primary control; the action-spread guard is the second, in case a future change adds one.

### 2026-09-12T00:40:46.154Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded; observers fan out synchronously after the reducer dispatch.

### 2026-09-12T00:40:46.228Z - Proof configurability set PROVEN: LuaBridgeOptions injects socket, state, log, onError and instructionBudget; no hard-coded dependency on MessageHandler.

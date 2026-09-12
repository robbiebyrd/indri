---
id: 019-c6b2
title: "Lua runtime: load, run and guard chunks"
status: complete
priority: P2
type: feature
created: "2026-09-09T19:04:59.223Z"
updated: "2026-09-10T00:28:22.083Z"
dependencies: ["012"]
plan: plans/layout-engine-renderer.md
plan_step: Step 9
depends_on: ["stories/012-8d40-pending-P1-spike-prove-fengari-bundles-and-runs-under-metro-o.md"]
started_at: "2026-09-10T00:15:17.259Z"
completed_at: "2026-09-10T00:28:22.083Z"
---

# Lua runtime: load, run and guard chunks

## Problem Statement

Lua runs synchronously on the UI thread, so a runaway script freezes the app and a script error must never break rendering. The runtime needs error capture as values rather than throws, and a best-effort instruction budget. Depends on the fengari spike proving the platform first.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] runChunk returns a result value on both syntax and runtime errors instead of throwing, surfacing the Lua message
- [x] A script error keeps the last good presentation and reports into a dev console overlay, mirroring the Go router's recover-per-handler stance
- [x] An infinite loop is aborted by the instruction budget within the budget's order of magnitude
- [x] A second script runs normally after a first one errored, proving no poisoned state
- [x] io, os, require and load are nil inside the runtime
- [x] The Lua stack is drained on both success and error paths, so a long session does not leak a stack slot per event
- [x] Documentation states plainly that the instruction budget is best-effort, not a hard limit, because count hooks are per-coroutine and cannot interrupt inside a host function

## Files

- client/layout/lua/runtime.ts
- client/layout/lua/runtime.node-test.ts

## Proof

- [x] [completeness] Completeness (All 8 criteria checked. 114/114 tests, typecheck clean, zero new lint warnings.)
- [x] [feature-availability] Feature availability (Confirmed on device via story 012 before this was built; guard（） is the documented seam for 020 and 024.)
- [x] [robustness] Robustness (Errors returned as values on both syntax and runtime paths; state usable after an error; dispose（） idempotent and run（）-after-dispose fails cleanly; an onError that itself throws is caught rather than converting a handled error into an unhandled one.)
- [x] [resilience] Resilience (Stack depth returns to baseline across 600 mixed runs. Mutation-verified independently: removing the settop in the finally produced 2 failures. A leaked slot per event would be an unbounded leak in a long session.)
- [x] [security] Security (Sandbox intact through the runtime: io/os/require/load still nil. Runaway loop aborted by the count hook in ~1ms. Limits of the budget documented rather than overstated.)
- [x] [defense-in-depth] Defense in depth (Three layers: libraries absent from the bundle, dangerous base globals nil'd on the state, and an instruction budget on execution.)
- [x] [input-validation] Input validation (Script source is arbitrary text and is never trusted: load failures and runtime failures are both values. Constructor rejects a non-positive or NaN budget with RangeError, since that is a wiring error not a script error.)
- [~] [thread-safety] Thread safety (One state, single-threaded, synchronous on the JS thread. Per-coroutine hook caveat documented; coroutine is not opened.)
- [x] [configurability] Configurability (instructionBudget and onError are the only knobs; the safe-library set stays fixed because a knob there would be a sandbox hole.)

## QA

114/114 tests, typecheck clean, zero new lint warnings. Stack balancing independently mutation-verified (2 failures without the restore).

## Work Log

### 2026-09-10T00:28:20.232Z - LuaRuntime over one long-lived sandboxed state. The design call worth keeping: a single guard<T>(op) seam that resets the instruction budget, restores the stack, and converts BOTH Lua errors and JS throws into values - run() is a thin wrapper over it, and 020/024 push their host table and dispatch events inside a guard without redesigning the class. Budget honesty: it bounds VM instructions only, cannot interrupt a host function (string.rep(s,1e9) is one instruction), does not bound time or memory, is per-coroutine, and can overshoot by one hook interval. All documented in source with no hard-limit language. The per-call reset is load-bearing, not decorative: fengari's L.hookcount keeps decrementing across separate pcalls, so a long-lived runtime would otherwise die once its cumulative total was spent. Real latent bug found and fixed along the way: lua_tostring returns null for a table, so error({code=1}) would have thrown a TypeError OUT of the runtime through a naive to_jsstring(null); the fengari.d.ts declaration was wrong and state.node-test.ts shared the assumption. Both corrected, test added. Also handled dispose() called from inside a guard callback - lua_close frees the stack, so an unconditional settop in the finally would throw past every guarantee guard makes. I independently mutation-checked the stack balancing: commenting out the settop in the finally produced 2 failures. 15 tests.


### 2026-09-10T00:28:21.071Z - Proof completeness set PROVEN: All 8 criteria checked. 114/114 tests, typecheck clean, zero new lint warnings.

### 2026-09-10T00:28:21.158Z - Proof robustness set PROVEN: Errors returned as values on both syntax and runtime paths; state usable after an error; dispose() idempotent and run()-after-dispose fails cleanly; an onError that itself throws is caught rather than converting a handled error into an unhandled one.

### 2026-09-10T00:28:21.244Z - Proof resilience set PROVEN: Stack depth returns to baseline across 600 mixed runs. Mutation-verified independently: removing the settop in the finally produced 2 failures. A leaked slot per event would be an unbounded leak in a long session.

### 2026-09-10T00:28:21.337Z - Proof security set PROVEN: Sandbox intact through the runtime: io/os/require/load still nil. Runaway loop aborted by the count hook in ~1ms. Limits of the budget documented rather than overstated.

### 2026-09-10T00:28:21.439Z - Proof input-validation set PROVEN: Script source is arbitrary text and is never trusted: load failures and runtime failures are both values. Constructor rejects a non-positive or NaN budget with RangeError, since that is a wiring error not a script error.

### 2026-09-10T00:28:21.519Z - Proof feature-availability set PROVEN: Confirmed on device via story 012 before this was built; guard() is the documented seam for 020 and 024.

### 2026-09-10T00:28:21.639Z - Proof defense-in-depth set PROVEN: Three layers: libraries absent from the bundle, dangerous base globals nil'd on the state, and an instruction budget on execution.

### 2026-09-10T00:28:21.774Z - Proof thread-safety set NOT_APPLICABLE: One state, single-threaded, synchronous on the JS thread. Per-coroutine hook caveat documented; coroutine is not opened.

### 2026-09-10T00:28:21.879Z - Proof configurability set PROVEN: instructionBudget and onError are the only knobs; the safe-library set stays fixed because a knob there would be a sandbox hole.

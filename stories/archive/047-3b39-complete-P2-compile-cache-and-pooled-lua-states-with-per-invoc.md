---
id: 047-3b39
title: Compile cache and pooled Lua states with per-invocation isolation
status: complete
priority: P2
type: feature
created: "2026-09-12T01:27:37.161Z"
updated: "2026-09-13T17:39:56.163Z"
dependencies: ["046"]
plan: plans/lua-game-scripting.md
plan_step: Step 2
depends_on: ["stories/046-f0ec-pending-P1-hardened-gopher-lua-state-with-a-stripped-standard.md"]
completed_at: "2026-09-13T17:39:56.163Z"
---

# Compile cache and pooled Lua states with per-invocation isolation

## Problem Statement

LState is not goroutine-safe and creating one per request is wasteful, but a pooled state carries globals and module tables between invocations, and an interrupted state carries a contaminated stack.

## Acceptance Criteria

- [x] Chunks compile once to a FunctionProto and are shared across states, while LFunctions are never shared
- [x] A handler runs with a fresh environment table whose __index falls through to the real globals, applied with SetFEnv
- [x] A global assigned during one invocation is invisible to the next invocation on the same pooled state
- [x] A state is discarded rather than returned to the pool after a deadline kill, an ApiErrorPanic or a stack overflow
- [x] Required module tables are frozen so a script cannot leak mutations to the next invocation
- [x] Fifty concurrent invocations pass under the race detector
- [x] VERIFY: go test -race ./internal/services/lua/

## Files

- internal/services/lua/compile.go
- internal/services/lua/pool.go

## Proof

- [x] [completeness] Completeness (7 of 7 criteria; compile cache, pool, per-invocation env, contamination discard and freeze all covered by tests with no database)
- [~] [feature-availability] Feature availability (Runtime plumbing with no entry point yet; stories 050 and 051 consume the pool)
- [x] [robustness] Robustness (A state is discarded after a deadline kill, an ApiErrorPanic or stack exhaustion, and reused after ordinary script errors; each path has a test)
- [x] [resilience] Resilience (The pool rebuilds after a killed state is discarded, retains at most maxIdle, and newPool builds one state eagerly so a broken prepare fails at boot rather than on first request)
- [x] [security] Security (Freezing closes _G.x, _G.string = evil, package.loaded._G and the table.insert raw-write escape; 12 write routes to shared state are refused with the shared value proven intact)
- [x] [defense-in-depth] Defense in depth (__newindex alone only guards absent keys, so string.format = evil would still have landed; emptying each frozen table into a hidden backing table is what makes overwrites hit the guard)
- [~] [input-validation] Input validation (No external input surface; script values are validated at the marshalling boundary in story 048)
- [x] [thread-safety] Thread safety (50 goroutines by 10 invocations each returning the expected value under -race; the pool is a mutex-guarded slice rather than sync.Pool so shutdown is deterministic)
- [x] [configurability] Configurability (maxIdle bounds retained states and the prepare hook is caller-supplied)

## QA

Verified independently: build, vet, full suite green under -race, and the no-database CI guard still reports no unexpected skips. Confirmed freezeTable empties into a hidden backing table (so overwriting an existing key is refused, not just adding one) and that method syntax on strings still works. 7/7 criteria.

## Work Log

### 2026-09-13T17:38:36.955Z - Added internal/services/lua/compile.go (compile + protoCache: parse.Parse then lua.Compile, memoised per chunk name with source comparison so a changed script never serves stale bytecode) and pool.go (statePool over a mutex-guarded slice, not sync.Pool, so shutdown closes every LState deterministically; per-invocation isolation via SetFEnv on a fresh env table whose metatable __index falls through to the real globals; instantiate() makes a per-state LFunction from the shared proto and nothing caches an LFunction across states; spoils() latches a state as unusable after a deadline kill, an ApiErrorPanic or stack/registry exhaustion, and release() closes it instead of pooling it; freezeState() empties the globals table, package, package.loaded/preload/loaders and every module into hidden backing tables reached through __index, because Lua consults __newindex only for absent keys, so a metatable alone would not stop string.format = evil). Two supporting fixes found while testing: table.insert writes through the array part with a raw set, so it is wrapped to refuse a frozen table; and OpenString makes the string table its own metatable with a raw __index pointing at itself, so freezing it broke method syntax until a dedicated string metatable was installed. sandbox.go's delegate() gained an nret parameter (was hardcoded to 1) so the table.insert guard can reuse it. Tests: compile_test.go and pool_test.go, stdlib testing, table-driven, no database. They prove one proto compiled once and reused across states with distinct LFunctions, a global assigned in one invocation invisible to the next invocation on the same state object, twelve routes to shared state all refused with the shared value intact, libraries still usable, discard after deadline/panic/overflow with the killed state closed and not handed out again, reuse after an ordinary script error, and 50 goroutines x 10 invocations each returning 1 under -race. go build ./..., go vet ./... and go test -race ./... are green.


### 2026-09-13T17:39:55.206Z - Proof completeness set PROVEN: 7 of 7 criteria; compile cache, pool, per-invocation env, contamination discard and freeze all covered by tests with no database

### 2026-09-13T17:39:55.275Z - Proof feature-availability set NOT_APPLICABLE: Runtime plumbing with no entry point yet; stories 050 and 051 consume the pool

### 2026-09-13T17:39:55.346Z - Proof robustness set PROVEN: A state is discarded after a deadline kill, an ApiErrorPanic or stack exhaustion, and reused after ordinary script errors; each path has a test

### 2026-09-13T17:39:55.420Z - Proof resilience set PROVEN: The pool rebuilds after a killed state is discarded, retains at most maxIdle, and newPool builds one state eagerly so a broken prepare fails at boot rather than on first request

### 2026-09-13T17:39:55.499Z - Proof security set PROVEN: Freezing closes _G.x, _G.string = evil, package.loaded._G and the table.insert raw-write escape; 12 write routes to shared state are refused with the shared value proven intact

### 2026-09-13T17:39:55.580Z - Proof defense-in-depth set PROVEN: __newindex alone only guards absent keys, so string.format = evil would still have landed; emptying each frozen table into a hidden backing table is what makes overwrites hit the guard

### 2026-09-13T17:39:55.659Z - Proof input-validation set NOT_APPLICABLE: No external input surface; script values are validated at the marshalling boundary in story 048

### 2026-09-13T17:39:55.736Z - Proof thread-safety set PROVEN: 50 goroutines by 10 invocations each returning the expected value under -race; the pool is a mutex-guarded slice rather than sync.Pool so shutdown is deterministic

### 2026-09-13T17:39:55.816Z - Proof configurability set PROVEN: maxIdle bounds retained states and the prepare hook is caller-supplied

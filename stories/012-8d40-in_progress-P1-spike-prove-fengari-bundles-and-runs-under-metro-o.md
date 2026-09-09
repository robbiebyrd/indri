---
id: 012-8d40
title: "SPIKE: prove fengari bundles and runs under Metro on web and native"
status: in_progress
priority: P1
type: task
created: "2026-09-09T19:04:59.218Z"
updated: "2026-09-09T19:23:36.144Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 2
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
started_at: "2026-09-09T19:23:36.143Z"
---

# SPIKE: prove fengari bundles and runs under Metro on web and native

## Problem Statement

No published evidence exists that fengari works under Metro/Hermes. Core fengari declares Node-only deps (readline-sync, tmp) and its io/os libs are documented Node-only, so luaL_openlibs is the likely break point. Its lua_sethook callback signature is also unverified. If this fails on native, the entire Lua half of the plan is in question and tic-tac-toe migration must fall back to declarative binding. This gates four downstream stories.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] createSandboxedState() opens only base, string, table and math via luaL_requiref, and never calls luaL_openlibs
- [x] load, loadstring, dofile, rawset and rawget are explicitly set to nil after opening base, because luaopen_base installs them and selective requiref alone is not a sandbox
- [x] A node-test asserts io, os, require, load, loadstring, dofile and debug all evaluate to nil inside the sandbox
- [ ] [MANUAL] A throwaway app/board/spike.tsx renders a Lua-computed value on web via pnpm run web
- [ ] [MANUAL] The same spike screen renders the Lua-computed value on a real iOS or Android device
- [x] The lua_sethook / LUA_MASKCOUNT callback signature is determined and written into the plan file, or recorded as unavailable with the fallback guard named
- [x] If Metro fails to resolve a Node builtin, the exact module is recorded and a resolver.extraNodeModules shim in metro.config.js is attempted before declaring failure
- [ ] The web/native result is reported explicitly so downstream stories know whether Lua is cross-platform or web-only

## Files

- client/layout/lua/state.ts
- client/layout/lua/state.node-test.ts
- client/app/board/spike.tsx

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

### 2026-09-09T19:23:36.256Z - Node-side spike complete; on-device run is all that remains. Findings: (1) lua_sethook(L, fn, LUA_MASKCOUNT, n) WORKS in fengari 0.1.5 - callback signature is (L, ar), luaL_error from it aborts the chunk with LUA_ERRRUN, infinite loop killed in ~1ms, LUA_MASKCOUNT=8. (2) Avoiding the fengari barrel does NOT shrink the module graph: lstrlib.js:76 and ltablib.js:50 both require lualib.js which unconditionally pulls loslib/loadlib/ldblib. Submodule imports load the IDENTICAL 38 modules as require('fengari'), verified by comparing require.cache. So the sandbox is 'never opened', not 'never shipped' - I initially got this wrong and corrected it. (3) Submodule imports are actively harmful: fengari modules are mutually circular and lauxlib captures LUA_REGISTRYINDEX from lua.js at eval time; importing lauxlib before lua makes every luaL_requiref fail with 'upvalue index too large'. Using the barrel fixes it and also silences the circular-dep warnings. (4) luaopen_base installs load/loadstring/dofile/loadfile, so selective requiref is not a sandbox by itself - those are nil'd explicitly, along with the raw* family since rawset would bypass the metatable protecting story 020's read-only state view. (5) Metro shims written for fs, os, path, child_process, util, tmp, readline-sync; sprintf-js deliberately NOT shimmed since lstrlib genuinely uses it. RN defines a global process, so fengari takes its Node branch on device - the shims are required, not optional.


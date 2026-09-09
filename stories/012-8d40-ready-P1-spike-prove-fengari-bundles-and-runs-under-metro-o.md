---
id: 012-8d40
title: "SPIKE: prove fengari bundles and runs under Metro on web and native"
status: ready
priority: P1
type: task
created: "2026-09-09T19:04:59.218Z"
updated: "2026-09-09T19:05:27.052Z"
dependencies: ["011"]
plan: plans/layout-engine-renderer.md
plan_step: Step 2
depends_on: ["stories/011-e75c-pending-P1-make-pnpm-test-discover-every-node-test-file-add-e.md"]
---

# SPIKE: prove fengari bundles and runs under Metro on web and native

## Problem Statement

No published evidence exists that fengari works under Metro/Hermes. Core fengari declares Node-only deps (readline-sync, tmp) and its io/os libs are documented Node-only, so luaL_openlibs is the likely break point. Its lua_sethook callback signature is also unverified. If this fails on native, the entire Lua half of the plan is in question and tic-tac-toe migration must fall back to declarative binding. This gates four downstream stories.

## Acceptance Criteria

- [ ] VERIFY: cd client && pnpm test
- [ ] createSandboxedState() opens only base, string, table and math via luaL_requiref, and never calls luaL_openlibs
- [ ] load, loadstring, dofile, rawset and rawget are explicitly set to nil after opening base, because luaopen_base installs them and selective requiref alone is not a sandbox
- [ ] A node-test asserts io, os, require, load, loadstring, dofile and debug all evaluate to nil inside the sandbox
- [ ] [MANUAL] A throwaway app/board/spike.tsx renders a Lua-computed value on web via pnpm run web
- [ ] [MANUAL] The same spike screen renders the Lua-computed value on a real iOS or Android device
- [ ] The lua_sethook / LUA_MASKCOUNT callback signature is determined and written into the plan file, or recorded as unavailable with the fallback guard named
- [ ] If Metro fails to resolve a Node builtin, the exact module is recorded and a resolver.extraNodeModules shim in metro.config.js is attempted before declaring failure
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


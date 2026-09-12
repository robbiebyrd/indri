---
id: 011-e75c
title: Make pnpm test discover every node-test file, add engine deps
status: complete
priority: P1
type: chore
created: "2026-09-09T19:04:59.195Z"
updated: "2026-09-09T19:13:05.253Z"
dependencies: []
plan: plans/layout-engine-renderer.md
plan_step: Step 1
started_at: "2026-09-09T19:09:58.223Z"
completed_at: "2026-09-09T19:13:05.253Z"
---

# Make pnpm test discover every node-test file, add engine deps

## Problem Statement

client/package.json runs test as a hardcoded single path (services/game-state-parser.node-test.ts) and CI runs pnpm test. Every new *.node-test.ts written for the layout engine would silently never run, making the whole plan's test coverage a false signal. Also adds the dependencies the engine needs.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] package.json test script uses Node's built-in runner with a glob over **/*.node-test.ts
- [x] game-state-parser.node-test.ts is converted to node:test test() plus node:assert, with all 6 existing assertions preserved unchanged including the prototype-pollution one
- [x] A new throwaway *.node-test.ts in a different directory is discovered and run, proving the glob works
- [x] zod, fengari and fengari-interop are added via pnpm add; expo-linear-gradient is added via npx expo install so SDK 53 pins the version
- [x] pnpm run typecheck passes

## Files

- client/package.json
- client/services/game-state-parser.node-test.ts

## Proof

- [x] [completeness] Completeness (All 6 criteria checked; pnpm test 6/6 pass, pnpm run typecheck exit 0.)
- [x] [feature-availability] Feature availability (Glob discovery proven: probe test in client/layout/ was found and run （7 tests）, vs 6 after removal. Previously only services/game-state-parser.node-test.ts ran.)
- [x] [robustness] Robustness (Verified green on Node 25.2.1 and Node 22.22.1 （mise）, matching the CI pin of node-version 22. --experimental-strip-types and --disable-warning both exist on 22.)
- [~] [resilience] Resilience (Build tooling change; no runtime failure modes introduced.)
- [x] [security] Security (Three new runtime deps （zod 4.5.4, fengari 0.1.5, fengari-interop 0.1.4） plus expo-linear-gradient 14.1.5. pnpm reported: Lockfile passes supply-chain policies. fengari is the deliberate choice over wasmoon; sandboxing of it is story 012, not this one.)
- [~] [defense-in-depth] Defense in depth (No trust boundary touched by a test script and dependency install.)
- [~] [input-validation] Input validation (No user or wire input; the script takes a fixed glob.)
- [~] [thread-safety] Thread safety (No concurrency introduced. Node's test runner isolates files in child processes by default.)
- [~] [configurability] Configurability (No configuration surface; the glob is intentionally fixed so new tests are picked up without opt-in.)

## QA

pnpm test green on Node 25 and Node 22.22.1 (CI pin); pnpm run typecheck clean; all 9 original parser assertions preserved and passing; glob discovery proven with a second-directory test before removal.

## Work Log

### 2026-09-09T19:12:39.775Z - Test script now uses Node's built-in runner with a **/*.node-test.ts glob plus --disable-warning=MODULE_TYPELESS_PACKAGE_JSON (adding type:module would break eslint.config.js, which is CommonJS). Converted game-state-parser.node-test.ts to node:test + node:assert/strict, preserving all 9 assertions across its 6 blocks. Verified glob discovery with a throwaway test in client/layout/ (7 tests found), then removed it. Installed zod 4.5.4, fengari 0.1.5, fengari-interop 0.1.4 via pnpm add and expo-linear-gradient 14.1.5 via npx expo install (SDK 53 pin). Verified green on Node 25 AND on Node 22.22.1 via mise, since CI pins node-version 22.


### 2026-09-09T19:13:01.672Z - Proof completeness set PROVEN: All 6 criteria checked; pnpm test 6/6 pass, pnpm run typecheck exit 0.

### 2026-09-09T19:13:01.744Z - Proof feature-availability set PROVEN: Glob discovery proven: probe test in client/layout/ was found and run (7 tests), vs 6 after removal. Previously only services/game-state-parser.node-test.ts ran.

### 2026-09-09T19:13:01.819Z - Proof robustness set PROVEN: Verified green on Node 25.2.1 and Node 22.22.1 (mise), matching the CI pin of node-version 22. --experimental-strip-types and --disable-warning both exist on 22.

### 2026-09-09T19:13:01.892Z - Proof security set PROVEN: Three new runtime deps (zod 4.5.4, fengari 0.1.5, fengari-interop 0.1.4) plus expo-linear-gradient 14.1.5. pnpm reported: Lockfile passes supply-chain policies. fengari is the deliberate choice over wasmoon; sandboxing of it is story 012, not this one.

### 2026-09-09T19:13:01.977Z - Proof resilience set NOT_APPLICABLE: Build tooling change; no runtime failure modes introduced.

### 2026-09-09T19:13:02.055Z - Proof defense-in-depth set NOT_APPLICABLE: No trust boundary touched by a test script and dependency install.

### 2026-09-09T19:13:02.137Z - Proof input-validation set NOT_APPLICABLE: No user or wire input; the script takes a fixed glob.

### 2026-09-09T19:13:02.219Z - Proof thread-safety set NOT_APPLICABLE: No concurrency introduced. Node's test runner isolates files in child processes by default.

### 2026-09-09T19:13:02.298Z - Proof configurability set NOT_APPLICABLE: No configuration surface; the glob is intentionally fixed so new tests are picked up without opt-in.

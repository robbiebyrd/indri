---
id: 072-9af4
title: Stop remounting every widget once the Lua bridge attaches
status: complete
priority: P2
type: fix
created: "2026-09-13T18:49:29.774Z"
updated: "2026-09-13T18:56:15.545Z"
dependencies: []
completed_at: "2026-09-13T18:56:15.545Z"
---

# Stop remounting every widget once the Lua bridge attaches

## Problem Statement

client/components/board/widget-host.tsx PressTarget returns a Fragment when onPress is undefined and a Pressable otherwise. useLuaBridge creates the bridge in an effect, so onPress is undefined on first render and defined after, which flips the element type at that position and makes React unmount and remount every widget subtree on the board. Harmless for todays stateless text widget and a landmine for any stateful one. Separately host-api.ts types state() as Readonly which is shallow, while its doc comment and the runtime deepFreeze both promise deep immutability, so mutating a nested field type-checks.

## Acceptance Criteria

- [x] PressTarget keeps a stable element type across the bridge attaching, branching only on the sub-grid type which never changes for a mounted widget
- [x] A sub-grid is still never a press target
- [x] state() and the snapshot field are typed with a deep readonly so mutating a nested field is a type error
- [x] No new client dependency is added that is not already declared in client/package.json
- [x] VERIFY: cd client && pnpm run typecheck && pnpm test

## Files

- client/components/board/widget-host.tsx
- client/layout/lua/host-api.ts

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; the element type is now fixed for the life of a mounted subtree and state（） is deeply readonly)
- [x] [feature-availability] Feature availability (A sub-grid is still never a press target, and the doc comment now records why the branch must not read onPress)
- [x] [robustness] Robustness (PressTarget branches only on type, which cannot change for a mounted widget, so the bridge attaching can no longer flip Fragment to Pressable and remount every widget)
- [~] [resilience] Resilience (Presentation-layer reconciliation fix with no failure path of its own)
- [~] [security] Security (No authorisation or input surface; the deep-readonly type hardens an internal contract rather than a trust boundary)
- [x] [defense-in-depth] Defense in depth (The ts-expect-error test was shown to be a real guard: reverting to Readonly makes tsc fail with TS2578 unused directive, so typecheck enforces the contract and the test run enforces the runtime freeze)
- [~] [input-validation] Input validation (No external input)
- [~] [thread-safety] Thread safety (Single-threaded React render path)
- [~] [configurability] Configurability (No configuration; type-fest was already a declared devDependency and no dependency was added)

## QA

Verified independently: client typecheck clean and 350/350 tests pass. The agent declined to write a test for the remount because Node type-stripping cannot parse JSX and there is no React renderer, and said so rather than writing coverage that only looked real. 5/5.

## Work Log

### 2026-09-13T18:54:57.578Z - Fixed both defects in client/ only. (1) widget-host.tsx PressTarget now branches on type === SUBGRID_TYPE alone and always renders Pressable for non-subgrid widgets with onPress?.(id), so the element type at that tree position is fixed for the life of the widget and the bridge attaching no longer remounts every widget subtree. Sub-grids still return a Fragment and are never press targets. (2) host-api.ts HostApi.state() and the private snapshot field are now ReadonlyDeep<Game> from type-fest (already a devDependency; type-only import, left in devDependencies). Added a test in host-api.node-test.ts that pairs @ts-expect-error on a nested write with assert.throws(TypeError), so the type refusal is enforced by tsc and the runtime freeze by the test run; verified it is a real guard by temporarily reverting to Readonly<Game>, which makes tsc report the directive unused. NOT TESTED: the remount itself and the sub-grid exclusion are React reconciliation/JSX properties; the client has no React renderer and node --experimental-strip-types cannot parse .tsx, so no test in this repo can observe them. Verified: pnpm run typecheck clean, pnpm test 347/347 pass, pnpm install --frozen-lockfile in sync, pnpm run lint 0 errors (5 pre-existing warnings elsewhere), go build ./... clean.


### 2026-09-13T18:56:14.843Z - Proof completeness set PROVEN: 5 of 5 criteria; the element type is now fixed for the life of a mounted subtree and state() is deeply readonly

### 2026-09-13T18:56:14.922Z - Proof feature-availability set PROVEN: A sub-grid is still never a press target, and the doc comment now records why the branch must not read onPress

### 2026-09-13T18:56:14.997Z - Proof robustness set PROVEN: PressTarget branches only on type, which cannot change for a mounted widget, so the bridge attaching can no longer flip Fragment to Pressable and remount every widget

### 2026-09-13T18:56:15.073Z - Proof resilience set NOT_APPLICABLE: Presentation-layer reconciliation fix with no failure path of its own

### 2026-09-13T18:56:15.151Z - Proof security set NOT_APPLICABLE: No authorisation or input surface; the deep-readonly type hardens an internal contract rather than a trust boundary

### 2026-09-13T18:56:15.226Z - Proof defense-in-depth set PROVEN: The ts-expect-error test was shown to be a real guard: reverting to Readonly makes tsc fail with TS2578 unused directive, so typecheck enforces the contract and the test run enforces the runtime freeze

### 2026-09-13T18:56:15.305Z - Proof input-validation set NOT_APPLICABLE: No external input

### 2026-09-13T18:56:15.384Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded React render path

### 2026-09-13T18:56:15.463Z - Proof configurability set NOT_APPLICABLE: No configuration; type-fest was already a declared devDependency and no dependency was added

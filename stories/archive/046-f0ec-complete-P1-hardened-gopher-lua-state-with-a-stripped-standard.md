---
id: 046-f0ec
title: Hardened gopher-lua state with a stripped standard library
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:37.160Z"
updated: "2026-09-13T17:18:26.303Z"
dependencies: ["045"]
plan: plans/lua-game-scripting.md
plan_step: Step 1
depends_on: ["stories/045-795e-pending-P1-thread-context-and-action-name-through-dispatch-mu.md"]
completed_at: "2026-09-13T17:18:26.303Z"
---

# Hardened gopher-lua state with a stripped standard library

## Problem Statement

gopher-lua ships no sandbox mode. Dangerous globals must be removed by hand after OpenBase, and the context deadline cannot interrupt a single long library call, so string.rep and string.format need their own caps.

## Acceptance Criteria

- [x] gopher-lua v1.1.1 is added to go.mod and lua.Options uses only fields that exist
- [x] OpenPackage is opened before OpenBase, and only package, base, table, string and math are opened
- [x] dofile, loadfile, load, loadstring, collectgarbage, print, setmetatable, getmetatable, rawset, rawget and newproxy are all LNil after construction
- [x] The os, io and debug tables are absent
- [x] package.path and package.cpath are cleared and package.loadlib is nil
- [x] A script that loops forever is killed by the context deadline
- [x] string.rep and string.format are wrapped with output size caps so an allocation bomb errors instead of allocating
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/sandbox.go
- go.mod

## Proof

- [x] [completeness] Completeness (8 of 8 criteria; four additional globals stripped beyond the brief （setfenv, getfenv, module, _printregs） each with a documented reason)
- [~] [feature-availability] Feature availability (State constructor with no entry point yet; stories 047 and 050 consume it)
- [x] [robustness] Robustness (A test caught a real bug where the format bound was only checked inside the directive branch, so a pure-literal format string over the cap slipped through; bomb cases assert failure in under a second so a cap that allocated first would fail)
- [~] [resilience] Resilience (In-process VM construction with no I/O and no external dependency that can fail)
- [x] [security] Security (os, io and debug never opened; eleven base globals plus four extras set to nil; package.path and cpath emptied and loadlib nilled, closing the LUA_PATH-seeded filesystem searcher)
- [x] [defense-in-depth] Defense in depth (SetContext cannot interrupt a single long library call （gopher-lua 521）, so the string caps are the real timeout on those paths rather than a convenience)
- [x] [input-validation] Input validation (string.rep and string.format check output size before delegating; star widths are rejected because no static check can bound a width taken from an argument)
- [~] [thread-safety] Thread safety (Constructs one LState per caller and shares none; pooling and per-invocation isolation are story 047)
- [x] [configurability] Configurability (maxStringBytes is a parameter with a documented default, mirroring the budget pattern in marshal.go)

## QA

Verified independently: build, vet green; go test -race ./internal/services/lua/ green; full suite green. Baselined the threat before capping it - Go already guards huge single widths, the real bomb is many sub-1e6 widths. 8/8 criteria.

## Work Log

### 2026-09-13T16:56:59.594Z - Added internal/services/lua/sandbox.go and sandbox_test.go. newState(maxStringBytes) builds an lua.LState with SkipOpenLibs and only package, base, table, string, math opened, package first so PreloadModule works later. Strips the 11 required globals plus setfenv, getfenv, module and _printregs, clears package.path/cpath and nils package.loadlib. capStringLib wraps string.rep and string.format with pre-call output-size checks that RaiseError before delegating, since a single Go library call is not interruptible by the context deadline (gopher-lua #521). rep is checked by division so the product never overflows; format is bounded by scanning literal bytes plus the larger of each field's width and precision, and rejects '*' widths, which no static check can bound. Patching the string table also covers method syntax because OpenString reuses it as the string metatable __index. Doc comments record the two accepted limitations: no heap cap (gopher-lua #230) and a loose format bound that cannot see argument size. Tests are stdlib, table-driven, DB-free: stripped globals, absent os/io/debug/coroutine/channel, sealed package plus a require that finds nothing, a context deadline killing an infinite loop asserted via ctx.Err() and elapsed time, six allocation-bomb shapes each asserted to fail in under a second so a cap that allocated first would fail, ten negative controls proving ordinary rep and format still work, a catchable-from-pcall check, a non-default cap value check, and a direct table-driven test of checkFormat. That last test caught a real gap: the sum check only ran inside the directive branch, so a literal-only format string over the cap passed; the check now runs once over the whole bound. go vet and go test -race on the package are green. go build ./... and go test ./... cannot be green right now: internal/repo/game is mid-refactor by a concurrent story (untracked core.go, modified game.go, s.ctx indirection errors) and every failure cascades from it. Nothing in internal/services/lua imports it.


### 2026-09-13T17:18:25.320Z - Proof completeness set PROVEN: 8 of 8 criteria; four additional globals stripped beyond the brief (setfenv, getfenv, module, _printregs) each with a documented reason

### 2026-09-13T17:18:25.396Z - Proof feature-availability set NOT_APPLICABLE: State constructor with no entry point yet; stories 047 and 050 consume it

### 2026-09-13T17:18:25.471Z - Proof robustness set PROVEN: A test caught a real bug where the format bound was only checked inside the directive branch, so a pure-literal format string over the cap slipped through; bomb cases assert failure in under a second so a cap that allocated first would fail

### 2026-09-13T17:18:25.550Z - Proof resilience set NOT_APPLICABLE: In-process VM construction with no I/O and no external dependency that can fail

### 2026-09-13T17:18:25.626Z - Proof security set PROVEN: os, io and debug never opened; eleven base globals plus four extras set to nil; package.path and cpath emptied and loadlib nilled, closing the LUA_PATH-seeded filesystem searcher

### 2026-09-13T17:18:25.704Z - Proof defense-in-depth set PROVEN: SetContext cannot interrupt a single long library call (gopher-lua 521), so the string caps are the real timeout on those paths rather than a convenience

### 2026-09-13T17:18:25.779Z - Proof input-validation set PROVEN: string.rep and string.format check output size before delegating; star widths are rejected because no static check can bound a width taken from an argument

### 2026-09-13T17:18:25.856Z - Proof thread-safety set NOT_APPLICABLE: Constructs one LState per caller and shares none; pooling and per-invocation isolation are story 047

### 2026-09-13T17:18:25.936Z - Proof configurability set PROVEN: maxStringBytes is a parameter with a documented default, mirroring the budget pattern in marshal.go

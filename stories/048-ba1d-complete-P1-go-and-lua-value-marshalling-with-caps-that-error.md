---
id: 048-ba1d
title: Go and Lua value marshalling with caps that error
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:37.162Z"
updated: "2026-09-12T01:43:42.171Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 3
completed_at: "2026-09-12T01:43:42.170Z"
---

# Go and Lua value marshalling with caps that error

## Problem Statement

Absence in a returned table means deletion, so a silently truncated conversion would look like a delete. Arbitrary Lua keys also break the dotted delta path protocol and Mongo field-name rules.

## Acceptance Criteria

- [x] A round trip preserves nested maps, dense arrays, integers versus floats, and nil
- [x] Depth and node-count caps return an error and never silently truncate
- [x] Cyclic tables, functions and userdata are rejected
- [x] Keys containing a dot or a dollar sign, or starting with a dollar sign, are rejected for delta-path and Mongo field-name safety
- [x] Integers outside the exactly representable JSON range of two to the fifty-third are rejected
- [x] Array versus object is decided by LTable.Len plus an empty hash part, not by ForEach
- [x] Empty-table semantics are stable and documented so events.Diff does not report a spurious change
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/marshal.go

## Proof

- [x] [completeness] Completeness (8 of 8 acceptance criteria checked; 25 top-level table-driven tests; VERIFY passed)
- [~] [feature-availability] Feature availability (Internal conversion package with no entry point yet; Step 6 （story 051） wires it into the router)
- [x] [robustness] Robustness (Rejects cyclic tables, functions, userdata, threads, channels, NaN and Inf, and nil inside an array; a typed-nil return bug on the error path was found by test and fixed in production code)
- [~] [resilience] Resilience (Pure in-memory value conversion with no I/O, no network and no external dependency that can fail)
- [x] [security] Security (TestFromLua_RejectsUnsafeKeys covers x.privateData, the delta-path forgery found by the security review; keys with a dot, a dollar sign or a NUL are refused)
- [x] [defense-in-depth] Defense in depth (Key validation runs in both directions, so state Go cannot safely publish also cannot reach a script)
- [x] [input-validation] Input validation (Depth and node caps that error rather than truncate （assertion proves a failed conversion returns nil）, key charset rules, and integer range limited to plus or minus 2 to the 53rd)
- [~] [thread-safety] Thread safety (Pure functions over caller-owned values with no shared mutable state; LState confinement is story 047)
- [x] [configurability] Configurability (Budget struct carries caller-supplied depth and node maxima)

## QA

Verified independently: build, vet and full suite green under -race. Depth-cap test asserts a failed conversion returns nil, proving no truncation. Unsafe-key test covers the x.privateData path forgery from the security review. 8/8 criteria checked.

## Work Log

### 2026-09-12T01:41:22.332Z - Built internal/services/lua/marshal.go: toLua(L, any) and fromLua(LValue, *budget) over the JSON domain (map[string]interface{}, []interface{}, string, bool, float64, nil), plus a budget struct carrying depth/nodes and their maxima (defaults 32/10000). Caps always return an error and never truncate, because absence in a returned table later means deletion. Array vs object is decided by LTable.Len() compared against the total entry count (ForEach only counts, it never classifies); empty tables are objects, which is stable across round trips and matches gopher-json. Rejects: cyclic tables (path-tracked seen set), functions/userdata/threads/channels, keys containing '.', '$' or NUL and empty keys, non-string/non-integer keys, integers outside +/-2^53, NaN/Inf, nil inside an array, and unsupported Go types. Numbers always come back as float64 so reflect.DeepEqual against events.ToMap output holds. 25 table-driven stdlib tests in marshal_test.go, no testify and no database; green under go build, go vet, go test and go test -race. Tests caught one real bug: tableFromLua returned a typed-nil map/slice inside a non-nil any on the error path, so a failed conversion tested as non-nil.


### 2026-09-12T01:43:39.802Z - Proof completeness set PROVEN: 8 of 8 acceptance criteria checked; 25 top-level table-driven tests; VERIFY passed

### 2026-09-12T01:43:39.879Z - Proof feature-availability set NOT_APPLICABLE: Internal conversion package with no entry point yet; Step 6 (story 051) wires it into the router

### 2026-09-12T01:43:39.951Z - Proof robustness set PROVEN: Rejects cyclic tables, functions, userdata, threads, channels, NaN and Inf, and nil inside an array; a typed-nil return bug on the error path was found by test and fixed in production code

### 2026-09-12T01:43:40.030Z - Proof resilience set NOT_APPLICABLE: Pure in-memory value conversion with no I/O, no network and no external dependency that can fail

### 2026-09-12T01:43:40.109Z - Proof security set PROVEN: TestFromLua_RejectsUnsafeKeys covers x.privateData, the delta-path forgery found by the security review; keys with a dot, a dollar sign or a NUL are refused

### 2026-09-12T01:43:40.188Z - Proof defense-in-depth set PROVEN: Key validation runs in both directions, so state Go cannot safely publish also cannot reach a script

### 2026-09-12T01:43:40.267Z - Proof input-validation set PROVEN: Depth and node caps that error rather than truncate (assertion proves a failed conversion returns nil), key charset rules, and integer range limited to plus or minus 2 to the 53rd

### 2026-09-12T01:43:40.347Z - Proof thread-safety set NOT_APPLICABLE: Pure functions over caller-owned values with no shared mutable state; LState confinement is story 047

### 2026-09-12T01:43:40.422Z - Proof configurability set PROVEN: Budget struct carries caller-supplied depth and node maxima

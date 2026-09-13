---
id: 052-f8ac
title: indri.mutate as the transaction bracket
status: complete
priority: P1
type: feature
created: "2026-09-12T01:27:37.176Z"
updated: "2026-09-13T18:29:12.762Z"
dependencies: ["051", "067"]
plan: plans/lua-game-scripting.md
plan_step: Step 7
depends_on: ["stories/051-0e8c-pending-P2-script-handler-reachable-through-the-router.md", "stories/067-4fbf-pending-P1-game-and-lua-fidelity-with-structural-invariants-e.md"]
started_at: "2026-09-13T18:11:16.597Z"
completed_at: "2026-09-13T18:29:12.762Z"
---

# indri.mutate as the transaction bracket

## Problem Statement

Script state edits must run inside Store.Mutate apply so they inherit the existing lock and version fence instead of adding a second concurrency model. mutation.Run currently returns nil for both an aborted attempt and a committed write, so the caller cannot tell them apart.

## Acceptance Criteria

- [x] The Lua callback runs inside the Store.Mutate apply closure
- [x] mutation.Run gains a committed-reporting variant so callers can distinguish abort from commit
- [x] Returning nil performs no write
- [x] Returning a state deep-equal to the input performs no write
- [x] Returning a changed state publishes exactly the changed paths
- [x] A nested indri.mutate on the same game returns a Lua error rather than deadlocking the keyed mutex
- [x] A forced CAS conflict re-runs the callback against reloaded state
- [x] The Lua deadline is asserted at boot to be well under the Redis lock lease
- [x] A script error leaves the document untouched and publishes nothing
- [x] VERIFY: go test -race ./internal/services/lua/ ./internal/repo/game/

## Files

- internal/services/lua/host_mutate.go
- internal/services/mutation/mutation.go
- internal/repo/game/game.go

## Proof

- [x] [completeness] Completeness (All 10 criteria met; indri.mutate, mutation.RunResult, core.MutateResult and the boot assertion all exist with tests, and go test -race is green across every affected package)
- [x] [feature-availability] Feature availability (A script's edit reaches the store and publishes the leaf path data.round （plus updatedAt, which every committed save stamps） — TestMutate_CommitsAChangeAndPublishesOnlyIt against a real MemoryStore)
- [x] [robustness] Robustness (Both no-ops （nil, and a state deep-equal to the input） abort without writing or publishing; a forced CAS conflict re-runs the callback against reloaded state rather than the losing attempt's table. All three mutation-checked)
- [x] [resilience] Resilience (A script error unwinds the mutation with the document untouched and nothing published; the Lua call is protected so the error does not panic through mutation.Run with the game's lock held)
- [x] [security] Security (The game id comes from req.Session.GameID and never the payload. TestMutate_IgnoresAGameIdInThePayload proves a caller in no game cannot edit a game named in their own message; added after a mutant with a payload fallback survived the original tests)
- [x] [defense-in-depth] Defense in depth (boot.checkScriptDeadline refuses a script deadline within 10x of the lock lease, so a script cannot outlive the lease and run beside another instance's attempt on the same game; the version fence remains the second layer)
- [x] [input-validation] Input validation (applyLua rejects a non-table return and a payload over the depth/node budget; checkInvariants （Step 4） still refuses membership and host edits, and the mutation aborts rather than half-writing)
- [x] [thread-safety] Thread safety (A nested indri.mutate raises instead of waiting on the keyed mutex its caller holds; the mutant without the guard hangs for the full 5s test timeout, which is the deadlock the guard exists for. go test -race green)
- [~] [configurability] Configurability (The 100ms deadline and the 10x headroom are constants checked against each other at boot; making either configurable belongs with the capability work in Step 13, not here)

## QA

None — covered by tests in internal/services/lua, internal/services/mutation and internal/services/boot

## Work Log

### 2026-09-13T18:26:16.948Z - Added mutation.RunResult, a committed-reporting variant of Run: Run now delegates to it, so an aborted attempt (false, nil) and a committed write (true, nil) are finally distinguishable — the signal Step 8's effect ledger needs, since apply re-runs on every fence miss and only the committing attempt's effects may be released. core.MutateResult threads that through the game store and is added to the Storer interface; Mutate delegates. New internal/services/lua/host_mutate.go implements indri.mutate(fn): it reads a per-invocation context (ctx, gameID, GameMutator) from the state's Go-side registry rather than capturing it, because the host table is frozen when the state is built and one shared closure serves every invocation that state goes on to run. Invoke installs that context before the call and clears it after. The game id comes from req.Session.GameID and never from the payload. The callback runs inside the store's apply closure, so it inherits the existing lock and version fence rather than adding a second concurrency model; returning nil or a state deep-equal to the input maps to mutation.ErrAbort, and the Lua call is protected so a script error unwinds the mutation without writing or publishing. A nested indri.mutate raises rather than waiting on the keyed mutex its own caller holds. boot.checkScriptDeadline asserts script.InvocationTimeout is at least 10x inside lock.DefaultRedisLeaseTTL, both now named constants. Tests run against game.MemoryStore — the real core, real lock, real fence, real deltas. Mutation-checked five behaviours; two of my own tests were vacuous and were rewritten: the call-context test passed with clearInvocation gutted (Invoke overwrites before every handler, so a second invocation never reads the first's) and the refusal tests passed with a payload gameId fallback added above the session check. Replaced with a direct unit test of clearInvocation and TestMutate_IgnoresAGameIdInThePayload. go build, gofmt and go test -race are green across lua, lua/lib, mutation, boot, repo/game and handlers/...


### 2026-09-13T18:26:32.833Z - Proof completeness set PROVEN: All 10 criteria met; indri.mutate, mutation.RunResult, core.MutateResult and the boot assertion all exist with tests, and go test -race is green across every affected package

### 2026-09-13T18:26:32.921Z - Proof feature-availability set PROVEN: A script's edit reaches the store and publishes the leaf path data.round (plus updatedAt, which every committed save stamps) — TestMutate_CommitsAChangeAndPublishesOnlyIt against a real MemoryStore

### 2026-09-13T18:26:33.010Z - Proof robustness set PROVEN: Both no-ops (nil, and a state deep-equal to the input) abort without writing or publishing; a forced CAS conflict re-runs the callback against reloaded state rather than the losing attempt's table. All three mutation-checked

### 2026-09-13T18:26:33.097Z - Proof resilience set PROVEN: A script error unwinds the mutation with the document untouched and nothing published; the Lua call is protected so the error does not panic through mutation.Run with the game's lock held

### 2026-09-13T18:26:33.202Z - Proof security set PROVEN: The game id comes from req.Session.GameID and never the payload. TestMutate_IgnoresAGameIdInThePayload proves a caller in no game cannot edit a game named in their own message; added after a mutant with a payload fallback survived the original tests

### 2026-09-13T18:26:33.294Z - Proof defense-in-depth set PROVEN: boot.checkScriptDeadline refuses a script deadline within 10x of the lock lease, so a script cannot outlive the lease and run beside another instance's attempt on the same game; the version fence remains the second layer

### 2026-09-13T18:26:33.374Z - Proof input-validation set PROVEN: applyLua rejects a non-table return and a payload over the depth/node budget; checkInvariants (Step 4) still refuses membership and host edits, and the mutation aborts rather than half-writing

### 2026-09-13T18:26:33.454Z - Proof thread-safety set PROVEN: A nested indri.mutate raises instead of waiting on the keyed mutex its caller holds; the mutant without the guard hangs for the full 5s test timeout, which is the deadlock the guard exists for. go test -race green

### 2026-09-13T18:26:33.537Z - Proof configurability set NOT_APPLICABLE: The 100ms deadline and the 10x headroom are constants checked against each other at boot; making either configurable belongs with the capability work in Step 13, not here

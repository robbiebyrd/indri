---
id: 054-5cca
title: indri.reply and indri.send with depth and fan-out budgets
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.569Z"
updated: "2026-09-13T23:17:06.025Z"
dependencies: ["053"]
plan: plans/lua-game-scripting.md
plan_step: Step 9
depends_on: ["stories/053-7102-pending-P1-two-level-effect-ledger-for-script-side-effects.md"]
started_at: "2026-09-13T18:49:46.439Z"
completed_at: "2026-09-13T23:17:06.025Z"
---

# indri.reply and indri.send with depth and fan-out budgets

## Problem Statement

Inline dispatch of a script-emitted event is the largest source of unbounded recursion in every event system surveyed. Separately, boot.handleClientMessage only logs a handler error, so a script error returned as a Go error would be invisible to WebSocket clients while REST and GraphQL callers see it.

## Acceptance Criteria

- [x] reply lands in Result.Responses in order
- [x] send dispatches only after the handler returns and never inline from within it
- [x] A handler that sends its own action terminates at the depth cap with a script error
- [x] Fan-out beyond the per-event cap errors
- [x] Script errors reach the client as a models.WSError inside Result.Responses rather than as the Go error return
- [x] The error response names the script and line for operator-authored scripts, with full detail logged server-side under a correlation id
- [x] Error delivery is asserted on WebSocket, GraphQL and REST
- [x] Calling reply twice either errors or the per-transport divergence is documented, since REST and GraphQL surface only the first response
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/host_io.go

## Proof

- [x] [completeness] Completeness (All 9 criteria met; indri.reply, indri.send, both budgets, the WSError error channel and the three-transport assertion all exist with tests, and go test -race is green across 23 packages)
- [x] [feature-availability] Feature availability (reply lands in Result.Responses in order and send dispatches after the handler returns; driven end to end through router.Dispatch, not through a stub)
- [x] [robustness] Robustness (Budgets counted by walking the ledger, so a mutate retry is not charged twice; a reply queued inside a losing attempt is discarded with it. Mutant M6 proves the ledger-walk is load-bearing)
- [x] [resilience] Resilience (send queues and drains after the handler returns; the inline-dispatch mutant produced a real 5.3s runaway and dropping depth from the derived context let one test observe 42,558 dispatches before the cap caught it)
- [x] [security] Security (A script failure is a models.WSError in Responses, not Invoke's Go error. I re-ran that mutant independently: it fails 6 tests, and on the three transports leaves WebSocket with 0 frames and REST answering 400 with the Lua traceback and a server temp path in the body)
- [x] [defense-in-depth] Defense in depth (Host failures still return a Go error while script failures do not, so an operator fault and a player-visible script fault stay distinguishable; the correlation id keeps the full traceback server-side)
- [x] [input-validation] Input validation (replyDocument rejects a payload it cannot convert, the depth cap refuses beyond 10 and fan-out beyond 32, and each raises a script error rather than silently dropping)
- [x] [thread-safety] Thread safety (One pooled state serves many invocations; the depth-leak mutant returned [1 2] against a wanted [1 1], proving the sequential two-invocation test is not vacuous. go test -race green)
- [~] [configurability] Configurability (The depth and fan-out caps are constants sized against a surveyed reference; per-script tuning belongs with the capability work, not here)

## QA

None — covered by tests in internal/services/lua and internal/services/boot

## Work Log

### 2026-09-13T23:16:31.168Z - Implemented indri.reply and indri.send over the Step 8 ledger, so a reply queued inside a losing mutate attempt is discarded with it. send queues and drains after the handler returns and never dispatches inline; a handler sending its own action stops at the depth cap. Depth rides on context.Context (the only thing crossing the hop between invocations); both budgets are counted by walking the ledger rather than a per-invocation counter, because a counter charges a mutate retry twice and refuses the attempt that commits. The P1 correction is the core: a script's own failure is now a models.WSError in Result.Responses rather than Invoke's Go error, because boot.handleClientMessage only logs a dispatch error so WebSocket drops it silently while REST and GraphQL propagate it; a host failure still returns a Go error. The frame carries the script file, line and message with the full traceback logged under a correlation id. reply is capped at one per invocation, justified as a contract rather than by the transport divergence, which af3c25f has since fixed under 068-f269. send reaches the router via a Dispatcher field wired by the injector, since lua importing router would invert the layering. Verified independently after rebasing onto af3c25f: gofmt clean, go build and go vet clean, go test -race -count=1 ./... green across all 23 packages including the peer's changed REST and GraphQL. Re-ran the error-channel mutant myself: putting the script error back on the Go error return fails 6 tests including TestScriptErrorReachesEveryTransport. Agent reported 16 mutants, all killed. Documented gap: the 32x10 caps do not bound the dispatch tree by themselves; the real bound is that every context derives from the root message's 100ms budget, which is incidental and stops holding if script.InvocationTimeout ever stops deriving from its caller.


### 2026-09-13T23:16:47.063Z - Proof completeness set PROVEN: All 9 criteria met; indri.reply, indri.send, both budgets, the WSError error channel and the three-transport assertion all exist with tests, and go test -race is green across 23 packages

### 2026-09-13T23:16:47.175Z - Proof feature-availability set PROVEN: reply lands in Result.Responses in order and send dispatches after the handler returns; driven end to end through router.Dispatch, not through a stub

### 2026-09-13T23:16:47.301Z - Proof robustness set PROVEN: Budgets counted by walking the ledger, so a mutate retry is not charged twice; a reply queued inside a losing attempt is discarded with it. Mutant M6 proves the ledger-walk is load-bearing

### 2026-09-13T23:16:47.418Z - Proof resilience set PROVEN: send queues and drains after the handler returns; the inline-dispatch mutant produced a real 5.3s runaway and dropping depth from the derived context let one test observe 42,558 dispatches before the cap caught it

### 2026-09-13T23:16:47.531Z - Proof security set PROVEN: A script failure is a models.WSError in Responses, not Invoke's Go error. I re-ran that mutant independently: it fails 6 tests, and on the three transports leaves WebSocket with 0 frames and REST answering 400 with the Lua traceback and a server temp path in the body

### 2026-09-13T23:16:47.680Z - Proof defense-in-depth set PROVEN: Host failures still return a Go error while script failures do not, so an operator fault and a player-visible script fault stay distinguishable; the correlation id keeps the full traceback server-side

### 2026-09-13T23:16:47.807Z - Proof input-validation set PROVEN: replyDocument rejects a payload it cannot convert, the depth cap refuses beyond 10 and fan-out beyond 32, and each raises a script error rather than silently dropping

### 2026-09-13T23:16:47.907Z - Proof thread-safety set PROVEN: One pooled state serves many invocations; the depth-leak mutant returned [1 2] against a wanted [1 1], proving the sequential two-invocation test is not vacuous. go test -race green

### 2026-09-13T23:16:48.010Z - Proof configurability set NOT_APPLICABLE: The depth and fan-out caps are constants sized against a surveyed reference; per-script tuning belongs with the capability work, not here

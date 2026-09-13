---
id: 053-7102
title: Two-level effect ledger for script side effects
status: complete
priority: P1
type: feature
created: "2026-09-12T01:28:12.567Z"
updated: "2026-09-13T18:40:40.540Z"
dependencies: ["052"]
plan: plans/lua-game-scripting.md
plan_step: Step 8
depends_on: ["stories/052-f8ac-pending-P1-indri-mutate-as-the-transaction-bracket.md"]
started_at: "2026-09-13T18:34:10.964Z"
completed_at: "2026-09-13T18:40:40.540Z"
---

# Two-level effect ledger for script side effects

## Problem Statement

The apply closure runs up to ten times on contention, so any side effect performed inline would fire once per attempt. Effects must buffer per attempt and flush only after the write commits, and the guarantee differs by effect kind because a standalone mongod has no multi-document transactions.

## Acceptance Criteria

- [x] Effects queued inside a retried mutate attempt are emitted once per commit, not once per attempt
- [x] Effects from an aborted or errored mutate are dropped
- [x] Effects queued outside mutate flush when the handler returns
- [x] Ordering is preserved across both ledger levels
- [x] Reply and send are documented as best-effort, since a lost frame is recovered by the next refresh keyframe
- [x] Timers are documented as at-least-once and carry an idempotency key, because a lost timer hangs a game forever
- [x] The ledger consumes the committed flag from the mutate variant rather than inferring commit from a nil error
- [x] VERIFY: go test ./internal/services/lua/

## Files

- internal/services/lua/effects.go

## Proof

- [x] [completeness] Completeness (8 of 8 criteria; 12 tests covering both ledger levels, ordering, retry, abort, double flush and per-invocation isolation)
- [~] [feature-availability] Feature availability (Infrastructure with a queueEffect seam; the effect kinds themselves are stories 054 and 057)
- [x] [robustness] Robustness (A flush drains before delivering so a second flush releases nothing, continues past a failed delivery joining errors, discards an attempt left open, and reports an effect queued during a flush rather than stranding it)
- [x] [resilience] Resilience (A flush error is logged rather than returned, matching the existing rule that a fan-out hiccup must not fail a write that already committed)
- [~] [security] Security (No external input or authorisation surface; the ledger only orders and releases effects other stories construct)
- [x] [defense-in-depth] Defense in depth (The abort case is the discriminating test: returning nil yields a nil error with committed false, and an error-only reading of that would wrongly release the effects)
- [x] [input-validation] Input validation (queueEffect refuses when no call is in progress rather than queuing into a ledger nobody will flush)
- [x] [thread-safety] Thread safety (Each invocation gets its own ledger held on the per-call invocation object; suite passes under -race)
- [~] [configurability] Configurability (No configuration; delivery guarantees are a property of the effect kind, documented on the effect interface)

## QA

Verified independently: build, vet, full suite green under -race, no skips. Confirmed 052 already supplies the committed-reporting path (mutation.RunResult, core.MutateResult) so the ledger consumes a real committed flag rather than inferring from a nil error. Confirmed nothing in the package claims exactly-once. 8/8 criteria.

## Work Log

### 2026-09-13T18:39:27.736Z - Two-level effect ledger implemented in internal/services/lua/effects.go: an attempt-level buffer reset at the top of every apply run and a committed-level buffer the attempt is promoted into, with order preserved across both. hostMutate settles the attempt from the committed flag MutateResult (story 052) already returns - commitAttempt on true, discardAttempt on false - so an abort, which returns a nil error, drops its effects. Invoke flushes the committed level when the handler returns and builds actions.Result from it; a handler that unwinds drops its ledger. flush drains before delivering (a second flush releases nothing), continues past a failed delivery and joins the errors, and reports an effect queued during a flush instead of stranding it. queueEffect(L, effect) is the seam stories 054/057 will call. Guarantees documented per kind on the effect interface: reply/send best-effort (a lost frame is replaced by the next refresh keyframe), timers at-least-once with an idempotency key (schedule.CreateEntry.IdempotencyKey); nothing is claimed as exactly-once. effects_test.go drives the real path through a test-only capability installed via the newEngine capability-set seam, with the real MemoryStore, lock, version fence and retry loop: a forced CAS miss queues 'inside' twice and delivers it once. 12 tests, no database, no skips. go build/vet/test -race ./... green.


### 2026-09-13T18:40:37.894Z - Proof completeness set PROVEN: 8 of 8 criteria; 12 tests covering both ledger levels, ordering, retry, abort, double flush and per-invocation isolation

### 2026-09-13T18:40:37.972Z - Proof feature-availability set NOT_APPLICABLE: Infrastructure with a queueEffect seam; the effect kinds themselves are stories 054 and 057

### 2026-09-13T18:40:38.049Z - Proof robustness set PROVEN: A flush drains before delivering so a second flush releases nothing, continues past a failed delivery joining errors, discards an attempt left open, and reports an effect queued during a flush rather than stranding it

### 2026-09-13T18:40:38.132Z - Proof resilience set PROVEN: A flush error is logged rather than returned, matching the existing rule that a fan-out hiccup must not fail a write that already committed

### 2026-09-13T18:40:38.214Z - Proof security set NOT_APPLICABLE: No external input or authorisation surface; the ledger only orders and releases effects other stories construct

### 2026-09-13T18:40:38.296Z - Proof defense-in-depth set PROVEN: The abort case is the discriminating test: returning nil yields a nil error with committed false, and an error-only reading of that would wrongly release the effects

### 2026-09-13T18:40:38.374Z - Proof input-validation set PROVEN: queueEffect refuses when no call is in progress rather than queuing into a ledger nobody will flush

### 2026-09-13T18:40:38.450Z - Proof thread-safety set PROVEN: Each invocation gets its own ledger held on the per-call invocation object; suite passes under -race

### 2026-09-13T18:40:38.529Z - Proof configurability set NOT_APPLICABLE: No configuration; delivery guarantees are a property of the effect kind, documented on the effect interface

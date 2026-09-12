---
id: 056-f2b1
title: Durable schedule store for deferred events
status: complete
priority: P2
type: feature
created: "2026-09-12T01:28:12.571Z"
updated: "2026-09-12T01:44:01.204Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 11
completed_at: "2026-09-12T01:44:01.204Z"
---

# Durable schedule store for deferred events

## Problem Statement

Nothing in the repo schedules deferred work today. Timers must survive restart and work with one or many instances, and Mongo is the only mandatory dependency. There is also no game deletion anywhere in the repo, so a timer for an abandoned game would live forever.

## Acceptance Criteria

- [x] Two concurrent claimers get disjoint entries and no entry is claimed twice
- [x] The claim predicate matches freshly inserted documents, so claimedUntil is initialised on insert or an exists-false branch is present
- [x] An expired lease is re-claimable and a lease renews while a slow handler is still running
- [x] Documents carry an indexed gameId, a scriptVersion, an attempts count, an idempotency key and an explicit pending, leased, done or dead state
- [x] CancelForGame cancels every pending timer for one game
- [x] An absolute TTL on fireAt expires orphans regardless of whether they ever fired
- [x] A permanent failure is marked dead rather than done, so the signal is not lost
- [x] The package declares a Storer interface with a compile-time assertion
- [x] VERIFY: go test ./internal/repo/schedule/

## Files

- internal/repo/schedule/schedule.go
- internal/repo/schedule/interface.go

## Proof

- [x] [completeness] Completeness (9 of 9 acceptance criteria checked; 21 tests; VERIFY passed)
- [~] [feature-availability] Feature availability (Store is not wired into the injector; story 057 （Step 12） owns the scheduler loop that consumes it)
- [x] [robustness] Robustness (Lease ownership rather than the clock is the fence, so a stolen lease returns ErrLeaseLost; Fail additionally fences on attempts; a permanent failure is marked dead rather than done so the signal is not lost)
- [x] [resilience] Resilience (Expired leases are reclaimable, failures retry with capped exponential backoff, and a TTL on fireAt expires orphans because nothing in this repo ever deletes a game)
- [~] [security] Security (No authentication or client input surface; entries are created server-side only, and the action dispatch boundary belongs to story 057)
- [x] [defense-in-depth] Defense in depth (The claim predicate is corrected twice over: claimedUntil is always written explicitly and the filter also carries an exists-false branch, because the failure mode is that no timer in the system ever fires)
- [x] [input-validation] Input validation (Config and entry validation with a pure withDefaults; the zero-value Config is valid)
- [x] [thread-safety] Thread safety (Claim is a single atomic FindOneAndUpdate; TestClaimStateMachine_ConcurrentClaimersAreDisjoint drives 8 goroutines over 200 entries under -race, and a Mongo-backed test races 4 stores over 40 entries)
- [x] [configurability] Configurability (Config carries instance id, lease TTL, max attempts, backoff and orphan grace)

## QA

Verified independently: build, vet and full suite green under -race. claimFilter carries the $exists:false branch so a fresh insert is claimable. TTL index is on fireAt, not firedAt, so an abandoned entry expires. 9/9 criteria checked. 10 tests run without a database, 11 skip without Mongo.

## Work Log

### 2026-09-12T01:41:24.053Z - Added internal/repo/schedule: a MongoDB-backed durable store for deferred game events (schedule.go, interface.go, schedule_test.go). Entries carry gameId (indexed), fireAt, action, payload, scriptVersion, attempts, idempotencyKey and an explicit pending/leased/done/dead state. ClaimDue leases one due entry with an atomic FindOneAndUpdate; the claim predicate initialises claimedUntil on insert AND carries an $exists:false branch, so a freshly inserted entry is claimable (the original design would never have fired anything). RenewLease extends a lease for a slow handler and reports ErrLeaseLost once another instance takes over; an expired lease is re-claimable. Fail retries with exponential backoff and dead-letters once attempts are spent; MarkDead retires an entry that can never run. Cancel/CancelForGame drop pending timers; a TTL index on fireAt (7d grace) expires orphans whether or not they ever fired, since nothing in this repo deletes a game. Storer interface with var _ Storer = (*Store)(nil). Tests split in two: 10 pure state-machine tests that always run (including a concurrent-claimers disjointness test over the production claimable/applyClaim pair) and 11 integration tests that skip without a reachable MongoDB, among them the real-database two-claimer disjointness proof. Verified with go build ./..., go vet ./... and go test -race -count=5 ./internal/repo/schedule/ against a live Mongo.


### 2026-09-12T01:43:56.126Z - Proof completeness set PROVEN: 9 of 9 acceptance criteria checked; 21 tests; VERIFY passed

### 2026-09-12T01:43:56.200Z - Proof feature-availability set NOT_APPLICABLE: Store is not wired into the injector; story 057 (Step 12) owns the scheduler loop that consumes it

### 2026-09-12T01:43:56.273Z - Proof robustness set PROVEN: Lease ownership rather than the clock is the fence, so a stolen lease returns ErrLeaseLost; Fail additionally fences on attempts; a permanent failure is marked dead rather than done so the signal is not lost

### 2026-09-12T01:43:56.346Z - Proof resilience set PROVEN: Expired leases are reclaimable, failures retry with capped exponential backoff, and a TTL on fireAt expires orphans because nothing in this repo ever deletes a game

### 2026-09-12T01:43:56.421Z - Proof security set NOT_APPLICABLE: No authentication or client input surface; entries are created server-side only, and the action dispatch boundary belongs to story 057

### 2026-09-12T01:43:56.501Z - Proof defense-in-depth set PROVEN: The claim predicate is corrected twice over: claimedUntil is always written explicitly and the filter also carries an exists-false branch, because the failure mode is that no timer in the system ever fires

### 2026-09-12T01:43:56.578Z - Proof input-validation set PROVEN: Config and entry validation with a pure withDefaults; the zero-value Config is valid

### 2026-09-12T01:43:56.657Z - Proof thread-safety set PROVEN: Claim is a single atomic FindOneAndUpdate; TestClaimStateMachine_ConcurrentClaimersAreDisjoint drives 8 goroutines over 200 entries under -race, and a Mongo-backed test races 4 stores over 40 entries

### 2026-09-12T01:43:56.734Z - Proof configurability set PROVEN: Config carries instance id, lease TTL, max attempts, backoff and orphan grace

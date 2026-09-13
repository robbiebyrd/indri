---
id: 065-ed08
title: Make the new stateful tests actually run in CI
status: complete
priority: P1
type: chore
created: "2026-09-12T01:28:39.047Z"
updated: "2026-09-13T17:18:45.173Z"
dependencies: ["056"]
plan: plans/lua-game-scripting.md
plan_step: Step 21
depends_on: ["stories/056-f2b1-pending-P2-durable-schedule-store-for-deferred-events.md"]
completed_at: "2026-09-13T17:18:45.173Z"
---

# Make the new stateful tests actually run in CI

## Problem Statement

CI runs on a plain runner with no Mongo or Redis service containers, and game_concurrency_test.go skips when Mongo is unreachable. Every stateful test in this plan would therefore report green in CI without ever running, shipping the whole feature unverified.

## Acceptance Criteria

- [x] Either the CI workflow gains a mongodb service, or in-memory fakes for the game and schedule stores are added
- [x] The in-memory fake approach is preferred, matching the bare-injector precedent in the boot handlers test
- [x] The tests for the mutate bracket, the schedule store and the scheduler fail when the logic is broken on a runner with no local Mongo
- [x] No unexpected skips remain in the new packages
- [x] air include_ext includes lua so editing a script triggers a reload
- [x] VERIFY: go test ./...

## Files

- .github/workflows/ci.yml
- .air.toml

## Proof

- [x] [completeness] Completeness (6 of 6 criteria; game store 1 to 17 tests running without a database, schedule store 21 to 51; guard widened to the whole repo)
- [x] [feature-availability] Feature availability (CI enforces it on every push: a test that skips with no database fails the build unless listed in KNOWN_SKIPS with a reason)
- [x] [robustness] Robustness (Eight deliberate breakages each caught by a named test with no database reachable, including a fence removal, a missing publish, an unsanitised delta and an exhausted-retry misclassification)
- [x] [resilience] Resilience (The suite passes both with a database and against a dead port, so a developer without Mongo and CI see the same result)
- [x] [security] Security (TestMutate_PrivateDataNeverReachesTheDelta now runs in the in-memory backend too, so the sanitisation rule is proved everywhere rather than only where Mongo is up)
- [x] [defense-in-depth] Defense in depth (The memory store refuses the same dollar-set MongoDB refuses on a null parent, so the fake cannot be more permissive than production and hide a real failure)
- [~] [input-validation] Input validation (Test infrastructure and CI configuration; no external input surface)
- [x] [thread-safety] Thread safety (Concurrent lost-update and concurrent-claim tests run under -race against both backends)
- [x] [configurability] Configurability (INDRI_TEST_MONGO_URI selects the backend and KNOWN_SKIPS is an explicit, commented allowlist)

## QA

Verified independently: build, vet and full suite green under -race. Confirmed every skip without a database is a /mongodb subtest. Widened the CI guard from two named packages to ./... with a single documented exception, so a future package cannot silently reintroduce the rot. 6/6 criteria.

## Work Log

### 2026-09-13T17:13:38.493Z - Gave the game and schedule stores in-memory backends so their rules run with no database, and added a mongodb service to CI for the layer a fake cannot stand in for.

Game store: extracted a narrow persistence port (docs) and moved the mutation bracket, the change deltas and every rule built on them onto a shared core that both backends embed. MemoryStore is therefore not a stub restating the rules - it runs the same lock, version fence, retry budget and publish path, over a map instead of a collection. store_contract_test.go asserts each rule once and runs it against both backends.

Schedule store: added MemoryStore driving the existing pure policy (claimable, applyClaim, failureOutcome) plus a new heldBy mirror of leaseFilter, with TestHeldBy_MirrorsLeaseFilter tying the two together. The lifecycle tests moved into storer_contract_test.go and run against both backends; the TTL-index test stays MongoDB-only. Deleted modelStore, which MemoryStore superseded.

Fixed a test that could not fail: TestRun_FenceRecoversWithoutLock passed with the version fence deleted, because its writers never overlapped. It now holds every writer after its first read until all have read, and goes red (got 1, want 8) when the fence is removed.

Run vs skip without a database (INDRI_TEST_MONGO_URI at a dead port), before -> after: game 1 ran / 1 skipped -> 17 ran / 8 skipped; schedule 21 ran / 11 skipped -> 51 ran / 13 skipped. Every remaining skip is a documented /mongodb subtest. With MongoDB reachable: 693 tests and subtests pass under -race, zero skips.

Deliberate breakage, each verified red with no database: mutation.Run ignoring whether the fenced save committed; core.Mutate committing without publishing; ErrAbort treated as a commit; core.Mutate no longer sanitizing the delta (leaked privateData); AddPlayer making every joiner host; claimable rejecting a never-claimed entry; heldBy ignoring the lease owner; failureOutcome marking an exhausted entry done. All restored.

CI: added a mongo:7 service plus a second step that reruns the two store packages against a dead port and fails if anything outside a /mongodb subtest skips. .air.toml include_ext now has lua.


### 2026-09-13T17:18:39.551Z - Proof completeness set PROVEN: 6 of 6 criteria; game store 1 to 17 tests running without a database, schedule store 21 to 51; guard widened to the whole repo

### 2026-09-13T17:18:39.620Z - Proof feature-availability set PROVEN: CI enforces it on every push: a test that skips with no database fails the build unless listed in KNOWN_SKIPS with a reason

### 2026-09-13T17:18:39.690Z - Proof robustness set PROVEN: Eight deliberate breakages each caught by a named test with no database reachable, including a fence removal, a missing publish, an unsanitised delta and an exhausted-retry misclassification

### 2026-09-13T17:18:39.773Z - Proof resilience set PROVEN: The suite passes both with a database and against a dead port, so a developer without Mongo and CI see the same result

### 2026-09-13T17:18:39.853Z - Proof security set PROVEN: TestMutate_PrivateDataNeverReachesTheDelta now runs in the in-memory backend too, so the sanitisation rule is proved everywhere rather than only where Mongo is up

### 2026-09-13T17:18:39.932Z - Proof defense-in-depth set PROVEN: The memory store refuses the same dollar-set MongoDB refuses on a null parent, so the fake cannot be more permissive than production and hide a real failure

### 2026-09-13T17:18:40.012Z - Proof input-validation set NOT_APPLICABLE: Test infrastructure and CI configuration; no external input surface

### 2026-09-13T17:18:40.093Z - Proof thread-safety set PROVEN: Concurrent lost-update and concurrent-claim tests run under -race against both backends

### 2026-09-13T17:18:40.175Z - Proof configurability set PROVEN: INDRI_TEST_MONGO_URI selects the backend and KNOWN_SKIPS is an explicit, commented allowlist

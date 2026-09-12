---
id: 052-f8ac
title: indri.mutate as the transaction bracket
status: pending
priority: P1
type: feature
created: "2026-09-12T01:27:37.176Z"
updated: "2026-09-12T02:41:10.979Z"
dependencies: ["051", "067"]
plan: plans/lua-game-scripting.md
plan_step: Step 7
depends_on: ["stories/051-0e8c-pending-P2-script-handler-reachable-through-the-router.md", "stories/067-4fbf-pending-P1-game-and-lua-fidelity-with-structural-invariants-e.md"]
---

# indri.mutate as the transaction bracket

## Problem Statement

Script state edits must run inside Store.Mutate apply so they inherit the existing lock and version fence instead of adding a second concurrency model. mutation.Run currently returns nil for both an aborted attempt and a committed write, so the caller cannot tell them apart.

## Acceptance Criteria

- [ ] The Lua callback runs inside the Store.Mutate apply closure
- [ ] mutation.Run gains a committed-reporting variant so callers can distinguish abort from commit
- [ ] Returning nil performs no write
- [ ] Returning a state deep-equal to the input performs no write
- [ ] Returning a changed state publishes exactly the changed paths
- [ ] A nested indri.mutate on the same game returns a Lua error rather than deadlocking the keyed mutex
- [ ] A forced CAS conflict re-runs the callback against reloaded state
- [ ] The Lua deadline is asserted at boot to be well under the Redis lock lease
- [ ] A script error leaves the document untouched and publishes nothing
- [ ] VERIFY: go test -race ./internal/services/lua/ ./internal/repo/game/

## Files

- internal/services/lua/host_mutate.go
- internal/services/mutation/mutation.go
- internal/repo/game/game.go

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


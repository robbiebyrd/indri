---
id: 045-795e
title: Thread context and action name through dispatch, mutate and locks
status: complete
priority: P1
type: refactor
created: "2026-09-12T01:27:37.146Z"
updated: "2026-09-12T03:00:39.702Z"
dependencies: []
plan: plans/lua-game-scripting.md
plan_step: Step 0
completed_at: "2026-09-12T03:00:39.701Z"
---

# Thread context and action name through dispatch, mutate and locks

## Problem Statement

actions.Request carries no context and no action name, GameRepo.Mutate uses the boot-time stored context, and lock.InProcess.Acquire checks ctx.Err once then blocks uncancellably on a keyed mutex. A Lua invocation killed by its deadline would leave a goroutine stuck on Mongo or a game lock, and hooks could not see which action fired. Every later step assumes this.

## Acceptance Criteria

- [x] actions.Request carries Context and Action, and router.Dispatch populates both
- [x] GameRepo.Mutate takes a ctx parameter instead of dereferencing the boot-time stored context
- [x] lock.InProcess.Acquire returns ctx.Err when the context is cancelled while already waiting on the keyed mutex, not only before
- [x] Cancelling the context makes an in-flight Mutate return promptly instead of blocking
- [x] The abandoned lock waiter is handed off and released rather than leaked
- [x] VERIFY: go test -race ./internal/handlers/... ./internal/services/lock/ ./internal/repo/game/

## Files

- internal/handlers/actions/handler.go
- internal/handlers/router/act.go
- internal/repo/game/game.go
- internal/services/lock/inprocess.go

## Proof

- [x] [completeness] Completeness (6 of 6 criteria; 13 new tests across router, lock and repo; all four Dispatch call sites and all nine Mutate call sites updated)
- [x] [feature-availability] Feature availability (WebSocket, GraphQL and REST all pass a context through; full suite green)
- [x] [robustness] Robustness (Reverting the lock fix fails both cancellation tests; removing only the handoff goroutine fails the leak test on a permanently dead key. RED was demonstrated, not assumed)
- [x] [resilience] Resilience (A cancelled request returns promptly instead of blocking on Mongo or a keyed mutex; publish keeps the store context so a cancelled request cannot make a committed write invisible to players)
- [~] [security] Security (No authentication, authorisation or input surface changed; Request.Action is set by the dispatcher from the action it is dispatching, not read from client-controlled state)
- [x] [defense-in-depth] Defense in depth (Request.Ctx falls back to context.Background so a handler cannot panic on a nil context)
- [~] [input-validation] Input validation (No new external input; the existing empty-action rejection is unchanged and now covered by TestDispatch_RejectsAnEmptyAction)
- [x] [thread-safety] Thread safety (Lock serialisation, cross-key independence, cancellation-while-waiting, handoff release and idempotent release all tested under -race)
- [~] [configurability] Configurability (No configuration introduced; timeouts come from the caller context)

## QA

Verified independently: build, vet and full suite green under -race. Lock handoff releases the key when the abandoned waiter finally acquires it, pinned by TestInProcessAcquire_ReleasesTheLockTheAbandonedWaiterInherits. Mutate uses the caller context for the lock wait, load and CAS, while publish deliberately keeps the store context so a committed write still reaches players. 6/6 criteria.

## Work Log

### 2026-09-12T02:59:24.534Z - Threaded context and action name through dispatch, Mutate and the in-process lock. actions.Request gains Context and Action plus a Ctx() nil-guard; router.Dispatch/DispatchMessage take a ctx and set Action to the dispatched action, not the lifecycle phase, so received/processed hooks can tell what fired. Store.Mutate(ctx, id, apply) now bounds the lock wait, the load and the CAS save with the caller's context; publish deliberately keeps the store context so a committed write is never left unbroadcast. lock.InProcess.Acquire waits on the keyed mutex in a goroutine and selects over ctx.Done(); when ctx wins, the still-queued goroutine is handed the job of releasing, so the key is not locked forever. Callers updated: graphql resolver (request ctx), rest (r.Context()), boot WS bridge (i.GlobalContext), layout and tictactoe handlers (req.Ctx()), 7 in-repo helpers (store ctx, unchanged behaviour). New tests: internal/handlers/router/act_test.go, internal/services/lock/inprocess_test.go, internal/repo/game/mutate_context_test.go (DB-free), plus two layout handler tests. RED proven by reverting the lock fix (both cancellation tests fail) and by dropping just the handoff goroutine (the leak test fails). go build, go vet and go test -race ./... all green.


### 2026-09-12T03:00:36.250Z - Proof completeness set PROVEN: 6 of 6 criteria; 13 new tests across router, lock and repo; all four Dispatch call sites and all nine Mutate call sites updated

### 2026-09-12T03:00:36.323Z - Proof feature-availability set PROVEN: WebSocket, GraphQL and REST all pass a context through; full suite green

### 2026-09-12T03:00:36.398Z - Proof robustness set PROVEN: Reverting the lock fix fails both cancellation tests; removing only the handoff goroutine fails the leak test on a permanently dead key. RED was demonstrated, not assumed

### 2026-09-12T03:00:36.473Z - Proof resilience set PROVEN: A cancelled request returns promptly instead of blocking on Mongo or a keyed mutex; publish keeps the store context so a cancelled request cannot make a committed write invisible to players

### 2026-09-12T03:00:36.550Z - Proof security set NOT_APPLICABLE: No authentication, authorisation or input surface changed; Request.Action is set by the dispatcher from the action it is dispatching, not read from client-controlled state

### 2026-09-12T03:00:36.630Z - Proof defense-in-depth set PROVEN: Request.Ctx falls back to context.Background so a handler cannot panic on a nil context

### 2026-09-12T03:00:36.707Z - Proof input-validation set NOT_APPLICABLE: No new external input; the existing empty-action rejection is unchanged and now covered by TestDispatch_RejectsAnEmptyAction

### 2026-09-12T03:00:36.785Z - Proof thread-safety set PROVEN: Lock serialisation, cross-key independence, cancellation-while-waiting, handoff release and idempotent release all tested under -race

### 2026-09-12T03:00:36.863Z - Proof configurability set NOT_APPLICABLE: No configuration introduced; timeouts come from the caller context

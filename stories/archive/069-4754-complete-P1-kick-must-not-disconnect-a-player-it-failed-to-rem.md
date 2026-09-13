---
id: 069-4754
title: kick must not disconnect a player it failed to remove
status: complete
priority: P1
type: fix
created: "2026-09-13T18:49:29.771Z"
updated: "2026-09-13T18:56:13.573Z"
dependencies: []
completed_at: "2026-09-13T18:56:13.573Z"
---

# kick must not disconnect a player it failed to remove

## Problem Statement

internal/handlers/actions/kick/handler.go:66 logs the error from GameService.RemovePlayer, discards it, returns a nil error, and force-disconnects the target anyway. The player is cut off from the transport while still a player of record in Mongo, so they cannot rejoin cleanly and cannot play. The comment directly above the return claims the target has still been removed, which is false on that path. The host sees no error because boot only logs dispatch errors.

## Acceptance Criteria

- [x] RemovePlayer failure returns a wrapped error and the target is NOT disconnected
- [x] A successful removal still returns the targets session id in DisconnectIDs
- [x] The misleading comment above the return is corrected
- [x] A test drives a failing RemovePlayer and asserts no DisconnectIDs are returned
- [x] VERIFY: go test -race ./internal/handlers/actions/kick/

## Files

- internal/handlers/actions/kick/handler.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; five cases covering success, failed removal, non-host caller, cross-game target and unauthenticated caller)
- [x] [feature-availability] Feature availability (kick is a registered built-in action and its behaviour on the failure path is now correct rather than silently destructive)
- [x] [robustness] Robustness (The failing removal is genuine: cancelling the store context makes Mutate give up on the game lock exactly as a write during shutdown does)
- [x] [resilience] Resilience (A removal that cannot commit now leaves the player in the game and connected, instead of stranding them disconnected but still a player of record)
- [x] [security] Security (Authorisation order is unchanged and still precedes the mutation: caller in game, caller is host, target session resolved, target in the same game, then removal)
- [x] [defense-in-depth] Defense in depth (The misleading comment asserting the target had been removed is corrected, so the next reader is not told something the code does not do)
- [~] [input-validation] Input validation (No new input surface; the target is resolved server-side from the session store, never from a client-supplied id)
- [x] [thread-safety] Thread safety (Driven against the real MemoryStore which runs the actual lock, fence and retry path; package passes under -race)
- [~] [configurability] Configurability (No configuration involved)

## QA

Verified independently: kick package green under -race. Agent proved the test RED against the old discard-the-error path, and drove a genuine removal failure by cancelling the store context so Mutate gives up on the game lock, rather than faking one. Authorisation still precedes the mutation. 5/5.

## Work Log

### 2026-09-13T18:54:32.917Z - kick now returns fmt.Errorf("removing player %v from game %v: %w") when GameService.RemovePlayer fails and returns no DisconnectIDs, so a target that is still a player of record is never cut off from the transport. Corrected the comment above the successful return. Added a narrow seam in the kick package (games/sessions interfaces + handle(req, games, sessions)); New(i) still stores only the injector so boot's registration tests keep working with a nil ServicesInjector. New handler_test.go runs the real gameRepo.MemoryStore (real lock, version fence, retry, delta) and drives a genuine removal failure by cancelling the store context; also covers successful kick, non-host caller, target in another game and unauthenticated caller. No skips, no database. Verified RED against the old behaviour before restoring the fix. go build ./... && go vet ./... && go test -race ./... all green.


### 2026-09-13T18:56:12.863Z - Proof completeness set PROVEN: 5 of 5 criteria; five cases covering success, failed removal, non-host caller, cross-game target and unauthenticated caller

### 2026-09-13T18:56:12.933Z - Proof feature-availability set PROVEN: kick is a registered built-in action and its behaviour on the failure path is now correct rather than silently destructive

### 2026-09-13T18:56:12.998Z - Proof robustness set PROVEN: The failing removal is genuine: cancelling the store context makes Mutate give up on the game lock exactly as a write during shutdown does

### 2026-09-13T18:56:13.075Z - Proof resilience set PROVEN: A removal that cannot commit now leaves the player in the game and connected, instead of stranding them disconnected but still a player of record

### 2026-09-13T18:56:13.150Z - Proof security set PROVEN: Authorisation order is unchanged and still precedes the mutation: caller in game, caller is host, target session resolved, target in the same game, then removal

### 2026-09-13T18:56:13.231Z - Proof defense-in-depth set PROVEN: The misleading comment asserting the target had been removed is corrected, so the next reader is not told something the code does not do

### 2026-09-13T18:56:13.307Z - Proof input-validation set NOT_APPLICABLE: No new input surface; the target is resolved server-side from the session store, never from a client-supplied id

### 2026-09-13T18:56:13.387Z - Proof thread-safety set PROVEN: Driven against the real MemoryStore which runs the actual lock, fence and retry path; package passes under -race

### 2026-09-13T18:56:13.468Z - Proof configurability set NOT_APPLICABLE: No configuration involved

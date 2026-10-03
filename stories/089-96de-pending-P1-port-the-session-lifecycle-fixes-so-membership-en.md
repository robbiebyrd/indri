---
id: 089-96de
title: Port the session-lifecycle fixes so game membership ends on leave, kick and moving games
status: pending
priority: P1
type: fix
created: "2026-10-03T03:52:28.000Z"
updated: "2026-10-03T03:52:28.000Z"
dependencies: ["081-7e0a", "082-b854"]
plan: plans/main-divergence-integration.md
plan_step: Step 5
depends_on: ["stories/081-7e0a-complete-P1-port-the-player-slot-model-and-migrate-the-user-ke.md", "stories/082-b854-pending-P1-port-user-and-session-backends-across-memory-sqlit.md"]
---

# Port the session-lifecycle fixes so game membership ends on leave, kick and moving games

## Problem Statement

Local `main` merged `fix/game-membership` (443f225, F-01/F-02/F-05) on top of the slot model after `origin/main` was cut. Story 081 ports the slot model and the two narrow slot fixes (92850e4, 9f18199); the rest of that merge is the session lifecycle: a session can outlive its seat, and a session left naming a game keeps receiving that game's broadcasts. It needs `LeaveGame` on every session store, so it follows 082 rather than riding in 081.

## Acceptance Criteria

- [ ] SessionService.LeaveGame takes a session out of its game on every session store (main 8269050, a2aec32)
- [ ] leave takes the session out of the game it leaves (main d3d9fb4)
- [ ] kick takes the target's session out of the game (main 162beed)
- [ ] join and create free the player's seat in their previous game (main fe952f0)
- [ ] reconnect does not resume into a seat the user no longer holds (main c12136d)
- [ ] login drops a seat the user no longer holds (main 30fdb44)
- [ ] game actions and refresh require the caller to hold their seat, resolved from their own connection (main c83b5f2, 9479609 — SeatedCaller, adapted to actions.Request)
- [ ] leave returns its removal error instead of logging it and reporting success (081 review: a stale session now hits this routinely)
- [ ] create validates the team before GameService.New, so a seat that cannot be given leaves no orphaned, holderless game behind (081 review)
- [ ] kick's host check and the removal happen in one Mutate, so a host who leaves between them cannot still kick (081 review; pre-existing, narrow window)
- [ ] VERIFY: go test -race ./internal/repo/session/ ./internal/handlers/...

## Files

- internal/repo/session/
- internal/services/session/
- internal/handlers/utils/
- internal/handlers/actions/

## Proof

- [ ] [completeness] Completeness
- [ ] [security] Security
- [ ] [thread-safety] Thread safety

## Work Log

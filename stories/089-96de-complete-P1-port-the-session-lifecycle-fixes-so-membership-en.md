---
id: 089-96de
title: Port the session-lifecycle fixes so game membership ends on leave, kick and moving games
status: complete
priority: P1
type: fix
created: "2026-10-03T03:52:28.000Z"
updated: "2026-10-04T18:00:52.000Z"
dependencies: ["081-7e0a", "082-b854"]
plan: plans/main-divergence-integration.md
plan_step: Step 5
depends_on: ["stories/081-7e0a-complete-P1-port-the-player-slot-model-and-migrate-the-user-ke.md", "stories/082-b854-complete-P1-port-user-and-session-backends-across-memory-sqlit.md"]
---

# Port the session-lifecycle fixes so game membership ends on leave, kick and moving games

## Problem Statement

Local `main` merged `fix/game-membership` (443f225, F-01/F-02/F-05) on top of the slot model after `origin/main` was cut. Story 081 ports the slot model and the two narrow slot fixes (92850e4, 9f18199); the rest of that merge is the session lifecycle: a session can outlive its seat, and a session left naming a game keeps receiving that game's broadcasts. It needs `LeaveGame` on every session store, so it follows 082 rather than riding in 081.

## Acceptance Criteria

- [x] SessionService.LeaveGame takes a session out of its game on every session store (main 8269050, a2aec32)
- [x] leave takes the session out of the game it leaves (main d3d9fb4)
- [x] kick takes the target's session out of the game (main 162beed)
- [x] join and create free the player's seat in their previous game (main fe952f0)
- [x] reconnect does not resume into a seat the user no longer holds (main c12136d)
- [x] login drops a seat the user no longer holds (main 30fdb44)
- [x] game actions and refresh require the caller to hold their seat, resolved from their own connection (main c83b5f2, 9479609 — SeatedCaller, adapted to actions.Request)
- [x] leave returns its removal error instead of logging it and reporting success (081 review: a stale session now hits this routinely)
- [x] create validates the team before GameService.New, so a seat that cannot be given leaves no orphaned, holderless game behind (081 review)
- [x] kick's host check and the removal happen in one Mutate, so a host who leaves between them cannot still kick (081 review; pre-existing, narrow window)
- [x] VERIFY: go test -race ./internal/repo/session/ ./internal/handlers/...

## Files

- internal/repo/session/
- internal/services/session/
- internal/handlers/utils/
- internal/handlers/actions/

## Proof

- [x] [completeness] Completeness
- [x] [security] Security
- [x] [thread-safety] Thread safety

## Work Log

### 2026-10-04T17:54:08.000Z - IMPLEMENTATION. Ported main's fix/game-membership lifecycle onto actions.Request handlers, test-first on handlertest (real services, in-memory stores). sessionRepo LeaveGame on all four backends (contract: clears game/team/slot, keeps user and token, drops the session from Find(gameId), ignores another game, idempotent on a missing session); the SQL store's Update and LeaveGame share one locked read-modify-write (modify). handlerUtils HoldsSeat (RequireHost now reuses it), DropLostSeat, VacateSeat over narrow interfaces. leave frees a held seat and leaves the game; a caller whose seat is already gone just leaves — the review item "leave reports success on a failed removal" is met as: an error when a seat the caller still holds could not be freed. kick: the host check is repeated inside the removal via RemovePlayerIf (new on Storer and GameService; the guard runs inside the fenced Mutate on every retry), and the target's session leaves this game before the disconnect (LeaveGame acts only on this game, so a target playing elsewhere is untouched). join/create (SeatCaller) free the previous game's seat only after the new seat is held; create checks the team against the script before GameService.New. login and reconnect drop a lost seat; reconnect then resumes outside the game with no keyframe. refresh and Lua script actions refuse a session that names a game whose seat it lost; callers in no game, and unauthenticated ones, still reach the script. kick's tests moved off a hand-rolled session map onto a real in-memory session store. PROTOCOL.md and SCRIPTING.md updated.

### 2026-10-04T17:54:08.000Z - VERIFICATION. go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL up, 0 skips. Mutations, each killed: LeaveGame ignoring its game (sqlite, postgres, mongodb), leave not leaving the game, kick not taking the target out, SeatCaller not freeing the old seat (join and create), create skipping the team check, reconnect and login not dropping a lost seat, refresh and script actions not checking the seat. A Mongo contract test first failed on leftover sessions from earlier runs in a shared game id; fixed by giving it a unique game id.

### 2026-10-04T18:00:52.000Z - REVIEW. An independent review found no authorisation hole and no double or missing player:left. Acted on: (1) a move whose old-seat removal failed for a real reason (conflict, store error) left the player holding that seat, host flag included, with the error only logged — and the harmless case (seat already lost) logged as an error too. gameRepo now wraps a not-held refusal in ErrSlotNotHeld; SeatCaller ignores only that and returns any other failure after moving the session to the new game (test with fault-injected removal: red, then green; swallowing the error again fails it). (2) No handler test would notice the in-write host re-check removed: added a kick test whose store hands the host role to the target between the first check and the write (a guard that always passes fails it). (3) leave checked the seat on a copy and could report an error for a seat freed in between; it now attempts the removal and treats ErrSlotNotHeld as already gone. (4) reconnect handed back the pre-drop session; it now re-reads it. The reviewer ran without INDRI_TEST_POSTGRES_URI; every run here had MongoDB and PostgreSQL up, 0 skips.

### 2026-10-04T18:00:52.000Z - PROOF. completeness: every handler main changed is ported (leave, kick, join, create, login, reconnect, refresh, game actions) plus the three review items filed from 081. feature-availability: a player moves between games keeping exactly one seat. robustness/resilience: a refused join keeps the old seat; a failed old-seat release is reported, not hidden. security: a session that outlived its seat cannot act in, refresh, resume into or keep receiving the game; the kick host check holds inside the write. defense-in-depth: LeaveGame only acts on the named game, so a late kick cannot pull a player out of the next. input-validation: create refuses an unknown team before making the game. thread-safety: guard inside the fenced Mutate; leave race-free via the sentinel. configurability: n/a. Final: go build/vet/gofmt clean; go test -race -count=1 ./... green with MongoDB and PostgreSQL, 0 skips.


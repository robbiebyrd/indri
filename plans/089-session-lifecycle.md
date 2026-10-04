# Story 089 — game membership ends on leave, kick and moving games

Reference: `main`'s `fix/game-membership` merge (443f225), adapted to `actions.Request` handlers. A
session can outlive its seat; one still naming a game keeps receiving that game's broadcasts and passes
every "in this game" check. The slot's `Player.UserID` is the authority on who holds a seat.

## Steps (each test-first; handler tests on `handlertest`)

A. `sessionRepo.Storer.LeaveGame(id, gameId)` on all four backends — clears game, team and slot only
   while the session is still in `gameId`; a missing session is not an error. `SessionService.LeaveGame`.
B. `handlerUtils`: `HoldsSeat(g, session)`, `DropLostSeat` (leave the game if the seat is gone, report
   whether it is held), `VacateSeat` (free the slot the session names). Narrow interfaces, not the injector.
C. Handlers:
   - `leave`: free the seat, take the session out of the game, and **return** a failed removal.
   - `kick`: take the target's session out of this game before disconnecting; the host check and the
     removal happen in one `Mutate`, so a host who leaves in between cannot still kick.
   - `join`/`create`: moving to another game frees the seat in the previous one; `create` validates
     the team before `GameService.New`, so a refused seat leaves no holderless game.
   - `login`, `reconnect`: a session whose seat is gone is taken out of its game; reconnect then
     resumes outside it (no keyframe of a game the user is no longer in).
   - `refresh` and script actions: a caller whose session names a game must hold their seat in it.

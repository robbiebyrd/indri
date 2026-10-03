---
id: 081-7e0a
title: Port the player slot model and migrate the user-keyed membership call sites
status: complete
priority: P1
type: refactor
created: "2026-10-01T13:52:29.871Z"
updated: "2026-10-03T04:26:12.000Z"
dependencies: ["080-cf52"]
plan: plans/main-divergence-integration.md
plan_step: Step 4
depends_on: ["stories/080-cf52-pending-P1-add-sqlite-and-postgres-game-backends-against-loca.md"]
---

# Port the player slot model and migrate the user-keyed membership call sites

## Problem Statement

origin/main replaced user-keyed team membership with pre-declared slots: AssignSlot, SlotID on the session, and a slot-keyed Players map. It deleted the team helpers local still calls. Porting slots means migrating those call sites in one move, because a half-slot half-user membership model is the worst of both.

## Acceptance Criteria

- [x] models.Session carries SlotID and Game.Players is keyed by slot id
- [x] AssignSlot(id, teamId, userId, displayName) (slotId, seated, error) exists on every backend and is team-aware
- [x] slot_contract_test.go asserts one slot per player, and that only a slot's holder can leave or disconnect it
- [x] AddPlayerToTeam, ChangePlayerTeam, HasPlayerOnTeam and PlayerOnWhichTeam call sites are migrated, not left alongside slots
- [x] kick still resolves the caller from their own connection's sessionId and never from a client-supplied userId
- [x] The layout action still finds the host correctly under slots (origin 0f85d3b is the reference)
- [x] Lua checkInvariants still refuses membership and host changes from a script now that membership is slot-shaped
- [x] The host check requires the caller to still hold the host slot (main 92850e4)
- [x] kick frees a slot whose player's session is in another game (main 9f18199)
- [x] player:joined and player:left still fire exactly once per real join and departure
- [x] VERIFY: go test -race ./internal/repo/game/ ./internal/handlers/actions/...

## Files

- internal/models/session.go
- internal/repo/game/player.go
- internal/handlers/actions/kick/
- internal/handlers/actions/join/
- internal/handlers/actions/layout/

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-10-01T14:53:57.102Z - HANDOFF NOTE. Prerequisites 079, 088 and 080 are complete on POC-00003/main-integration. Local's game Storer is still user-keyed: Game.Players is keyed by userId and AddPlayerToTeam/ChangePlayerTeam/HasPlayerOnTeam/PlayerOnWhichTeam are live call sites. Four backends now implement the docs port, so AssignSlot must land on all four: mongoDocs (game.go), memoryDocs (memory.go) and sqlDocs (sql.go, which serves both SQL dialects). The slot change touches authorisation -- kick resolves the caller from their own connection's sessionId, and the layout handler finds the host via Game.Players; origin/main 0f85d3b is the reference for host-by-slot. Lua checkInvariants must keep refusing membership and host changes once membership is slot-shaped. example/tictactoe/game.lua authorises on session.teamId, which slots preserve, but that was reasoned rather than executed -- the scripttest fixtures are the signal.

### 2026-10-03T03:52:28.000Z - SCOPE. Local main is no longer the dispatch trunk: it is origin/main plus 69 newer commits, and the dispatch lineage lives only on this branch. The port reference is therefore main, not origin/main 0f85d3b. Boss chose: slot model plus the two narrow slot fixes (92850e4 host must hold host slot, 9f18199 kick frees a slot cross-game) in 081; the session-lifecycle half of main's fix/game-membership merge (LeaveGame, SeatedCaller, seat drops) is filed as 089 after 082. Local-only behaviour to keep: AddPlayerResult/RemovePlayerResult flags so player:joined/player:left fire once.

### 2026-10-03T04:14:15.000Z - IMPLEMENTATION. Slot rules live once on core over Mutate, so all four backends (mongo, memory, sqlite, postgres) share them; docs.setPlayerConnected was removed from the port and all three docs implementations, since connect/disconnect now need the holder check inside the fence. AssignSlot returns (slotId, seated, err) rather than the criterion's (slotId, err): seated replaces AddPlayerResult as what player:joined fires on. RemovePlayerResult is gone because RemovePlayer now fails for a non-holder, so a nil error is the single successful removal. newGameDoc declares slots via declareSlots, which copies the script's teams map rather than writing it. Beyond the listed criteria: inquire counts held slots (origin 6db999e) since len(PlayerIDs) is now capacity; join rejects a missing team up front (origin 5ca03c9); join/create share handlerUtils.SeatCaller and no longer write a game into the session when seating failed; the script loader refuses teams with maxPlayersPerTeam < 1, which would otherwise create unjoinable games; lifecycle subjects gain slotId; kick takes slotId on WS, GraphQL (regenerated with gqlgen@v0.17.95 run outside the module so go.mod/go.sum are untouched) and REST; client finds the caller's slot by userId (services/players.ts). Docs: PROTOCOL, SCRIPTING, ARCHITECTURE, CLAUDE.md.

### 2026-10-03T04:14:15.000Z - VERIFICATION. go build, go vet, gofmt clean; go test -race -count=1 ./... green against MongoDB and PostgreSQL (INDRI_TEST_POSTGRES_URI set), 0 skips. Client, CI-style (rm -rf node_modules, pnpm install --frozen-lockfile): typecheck clean, 416/416 tests, lint 0 errors / 4 warnings (baseline 5, none new). Mutation checks, each confirmed to fail a test: RemovePlayer without holder check; setConnected without holder check; move without clearing old slot; rejoin reported as seated; slots written into the script; slot search across all teams; host flag lost on move; player:joined emitted unconditionally; kick without RequireHost; kick refusing a target playing elsewhere; kick disconnecting a target playing elsewhere; layout without RequireHost. Two mutants first appeared to survive and were mine to fix: one did not compile (masked as no failures), one deferred a stale copy; both re-run correctly and killed.

### 2026-10-03T04:26:12.000Z - REVIEW. An independent review found no authorisation hole, lost update or double-firing event. Acted on: Lua req.session and indri-script fixture sessions now carry slotId (scripts could not find the caller's player at all); SetPlayerAsHost refuses a free slot (a host nobody holds would stop the next joiner being elected); stale markPlayerConnected references in CLAUDE.md and ARCHITECTURE.md fixed. Boss decided slot handover: a freed slot keeps score/data/privateData until another user is seated, who starts clean; a rejoin keeps its own. Filed on 089 (pre-existing, session-lifecycle adjacent): leave swallowing its removal error, create leaving an orphaned game when seating fails, kick's host check outside the removal's Mutate. Filed 090: TestAnswerOffer_ClosedMidGatherReturnsError fails deterministically without network egress (5/5 in the sandbox, 30/30 pass outside) -- pre-existing, not caused by this story.

### 2026-10-03T04:26:12.000Z - PROOF. completeness: every user-keyed call site migrated, grep for the removed API is empty outside history. feature-availability: kick/join/create/leave/logout/layout/inquire work on WS, GraphQL and REST; client finds its slot. robustness/resilience: a failed removal disconnects nobody; a failed seat leaves the session unchanged; stale holders change nothing. security/defense-in-depth: caller always from their own session, RequireHost also requires holding the slot, Lua cannot change a slot's holder, config with zero slots refused at boot. input-validation: empty game/slot ids, unknown team, missing slotId/teamId rejected. thread-safety: all membership writes inside Mutate's fence, concurrent-assign and racing-remove tests on all four backends under -race. configurability: slots come from the script's maxPlayersPerTeam. Final: go build/vet/gofmt clean, go test -race -count=1 ./... green with Mongo and Postgres, 0 skips; client CI-style install/typecheck/416 tests green.


---
id: 081-7e0a
title: Port the player slot model and migrate the user-keyed membership call sites
status: pending
priority: P1
type: refactor
created: "2026-10-01T13:52:29.871Z"
updated: "2026-10-01T13:52:39.697Z"
dependencies: ["080-cf52"]
plan: plans/main-divergence-integration.md
plan_step: Step 4
depends_on: ["stories/080-cf52-pending-P1-add-sqlite-and-postgres-game-backends-against-loca.md"]
---

# Port the player slot model and migrate the user-keyed membership call sites

## Problem Statement

origin/main replaced user-keyed team membership with pre-declared slots: AssignSlot, SlotID on the session, and a slot-keyed Players map. It deleted the team helpers local still calls. Porting slots means migrating those call sites in one move, because a half-slot half-user membership model is the worst of both.

## Acceptance Criteria

- [ ] models.Session carries SlotID and Game.Players is keyed by slot id
- [ ] AssignSlot(id, teamId, userId, displayName) (slotId, error) exists on every backend and is team-aware
- [ ] slot_contract_test.go asserts one slot per player, and that only a slot's holder can leave or disconnect it
- [ ] AddPlayerToTeam, ChangePlayerTeam, HasPlayerOnTeam and PlayerOnWhichTeam call sites are migrated, not left alongside slots
- [ ] kick still resolves the caller from their own connection's sessionId and never from a client-supplied userId
- [ ] The layout action still finds the host correctly under slots (origin 0f85d3b is the reference)
- [ ] Lua checkInvariants still refuses membership and host changes from a script now that membership is slot-shaped
- [ ] VERIFY: go test -race ./internal/repo/game/ ./internal/handlers/actions/...

## Files

- internal/models/session.go
- internal/repo/game/player.go
- internal/handlers/actions/kick/
- internal/handlers/actions/join/
- internal/handlers/actions/layout/

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

### 2026-10-01T14:53:57.102Z - HANDOFF NOTE. Prerequisites 079, 088 and 080 are complete on POC-00003/main-integration. Local's game Storer is still user-keyed: Game.Players is keyed by userId and AddPlayerToTeam/ChangePlayerTeam/HasPlayerOnTeam/PlayerOnWhichTeam are live call sites. Four backends now implement the docs port, so AssignSlot must land on all four: mongoDocs (game.go), memoryDocs (memory.go) and sqlDocs (sql.go, which serves both SQL dialects). The slot change touches authorisation -- kick resolves the caller from their own connection's sessionId, and the layout handler finds the host via Game.Players; origin/main 0f85d3b is the reference for host-by-slot. Lua checkInvariants must keep refusing membership and host changes once membership is slot-shaped. example/tictactoe/game.lua authorises on session.teamId, which slots preserve, but that was reasoned rather than executed -- the scripttest fixtures are the signal.


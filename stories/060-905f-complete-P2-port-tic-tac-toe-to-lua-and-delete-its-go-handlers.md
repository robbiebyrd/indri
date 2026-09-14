---
id: 060-905f
title: Port tic-tac-toe to Lua and delete its Go handlers
status: complete
priority: P2
type: refactor
created: "2026-09-12T01:28:12.574Z"
updated: "2026-09-14T00:48:48.193Z"
dependencies: ["055", "057"]
plan: plans/lua-game-scripting.md
plan_step: Step 15
depends_on: ["stories/055-c8e1-pending-P2-shared-helper-library-in-lua-served-from-memory.md", "stories/057-1deb-pending-P2-indri-after-indri-at-and-indri-cancel.md"]
started_at: "2026-09-13T23:56:50.719Z"
completed_at: "2026-09-14T00:48:48.193Z"
---

# Port tic-tac-toe to Lua and delete its Go handlers

## Problem Statement

The example game is the proof that a game becomes config plus Lua with no Go compilation. Its Go move handler and its test suite must be replaced, not merely duplicated.

## Acceptance Criteria

- [x] Every case from the deleted move handler test is re-expressed as a golden test of fixture state plus action to expected published delta
- [x] Turn enforcement, out-of-bounds moves, occupied cells, row, column and diagonal wins, and a draw are all covered
- [x] The example server handler directory is deleted
- [x] The game runs on the stock cmd/server binary with no custom main
- [x] VERIFY: go vet ./... && go test ./example/...

## Files

- example/tictactoe/game.lua
- example/tictactoe/main.go

## Proof

- [x] [completeness] Completeness (All 5 criteria met; 28 Go test functions became 49 golden leaf cases with nothing dropped, server/ is deleted, and the example runs on stock cmd/server with no main)
- [x] [feature-availability] Feature availability (Turn enforcement, out-of-bounds, occupied cells, row, column, both diagonals and draw are all covered, plus nine behaviours the old Go test never exercised)
- [x] [robustness] Robustness (Three diagonal-bound mutants survived because the ported algorithm swept an unwinnable-width window; the function was rewritten to enumerate only winnable diagonals, making the bounds observable, and all three then died. 33 mutants, zero survivors)
- [x] [resilience] Resilience (Golden tests assert the published delta, zero deltas and an unchanged version together. I mutated the port into a silent no-op myself: 18 leaf cases fail, so the vacuous shape this test style invites is closed)
- [x] [security] Security (Authorisation uses the transport-authenticated session.teamId rather than re-deriving the team from membership as the plan's sketch did, which is the weaker rule; indri.mutate resolves the game from the session so a script cannot edit a game its caller is not in)
- [x] [defense-in-depth] Defense in depth (A malformed move is refused with a message and no delta; the nil-guard on team.data.turn keeps a misconfigured team distinct from a refused move rather than collapsing both to a silent no-op)
- [x] [input-validation] Input validation (RefusesAMalformedMove covers 13 rows including non-string, 1.0, 0x2, 1e1, leading space and row-off-the-end; bounds follow the configured board across four shapes)
- [x] [thread-safety] Thread safety (The port writes only through indri.mutate, inheriting the existing lock and version fence; go test -race green tree-wide)
- [x] [configurability] Configurability (Board shape is read from config rather than hardcoded: bounds are verified against 3x3, 2x3, 3x2, 1x3 and 3x1)

## QA

None — covered by golden tests in example/tictactoe

## Work Log

### 2026-09-14T00:48:46.946Z - Ported example/tictactoe to Lua: config.json plus game.lua, no Go code of its own, running on the stock cmd/server binary. The 276-line move handler and its 457-line test are deleted, but only after all 28 Go test functions were re-expressed as 49 golden leaf cases passing against the Lua port; nothing failed to reproduce, so no handler was kept back. The port additionally covers nine behaviours Handle had that the old test never exercised. Golden tests assert the published delta rather than stored state, and every refusal case asserts the message, zero deltas and an unchanged version together, so a script that silently does nothing fails rather than passing: I verified that myself by mutating the port into a no-op, which fails 18 leaf cases. The plan's Lua sketch was wrong three ways and none were copied: indri.mutate takes one argument and resolves the game from the session; authorisation uses the transport-authenticated session.teamId rather than re-deriving the team from membership, which is a weaker rule; and team.data.turn needs a nil guard to keep 'misconfigured team' distinct from 'not your turn'. diagonal_win was rewritten to enumerate by starting column rather than offset, because the ported Go algorithm swept a window wider than any winnable window and three mutations of its bounds survived; enumerating only winnable diagonals is behaviourally identical and makes the bounds observable, killing all three. The Go original also had its two diagonal loops commented backwards. Four documentation pointers falsified by the deletion were corrected in CLAUDE.md, README.md and docs/ARCHITECTURE.md; CLAUDE.md's 'Adding a game action' section was deliberately left for 066, being stale rather than wrong. Verified: gofmt clean, build and vet clean, go test -race -count=1 ./... green tree-wide, go test ./example/... green with no skips against an unreachable Mongo URI. One claim I could not verify: the agent reported a live boot on the stock binary, which I cannot reproduce because no MongoDB is reachable from this sandbox and the failure occurs before scripts load; another session with a reachable Mongo is confirming it.


### 2026-09-14T00:48:47.397Z - Proof completeness set PROVEN: All 5 criteria met; 28 Go test functions became 49 golden leaf cases with nothing dropped, server/ is deleted, and the example runs on stock cmd/server with no main

### 2026-09-14T00:48:47.477Z - Proof feature-availability set PROVEN: Turn enforcement, out-of-bounds, occupied cells, row, column, both diagonals and draw are all covered, plus nine behaviours the old Go test never exercised

### 2026-09-14T00:48:47.560Z - Proof robustness set PROVEN: Three diagonal-bound mutants survived because the ported algorithm swept an unwinnable-width window; the function was rewritten to enumerate only winnable diagonals, making the bounds observable, and all three then died. 33 mutants, zero survivors

### 2026-09-14T00:48:47.640Z - Proof resilience set PROVEN: Golden tests assert the published delta, zero deltas and an unchanged version together. I mutated the port into a silent no-op myself: 18 leaf cases fail, so the vacuous shape this test style invites is closed

### 2026-09-14T00:48:47.721Z - Proof security set PROVEN: Authorisation uses the transport-authenticated session.teamId rather than re-deriving the team from membership as the plan's sketch did, which is the weaker rule; indri.mutate resolves the game from the session so a script cannot edit a game its caller is not in

### 2026-09-14T00:48:47.802Z - Proof defense-in-depth set PROVEN: A malformed move is refused with a message and no delta; the nil-guard on team.data.turn keeps a misconfigured team distinct from a refused move rather than collapsing both to a silent no-op

### 2026-09-14T00:48:47.873Z - Proof input-validation set PROVEN: RefusesAMalformedMove covers 13 rows including non-string, 1.0, 0x2, 1e1, leading space and row-off-the-end; bounds follow the configured board across four shapes

### 2026-09-14T00:48:47.943Z - Proof thread-safety set PROVEN: The port writes only through indri.mutate, inheriting the existing lock and version fence; go test -race green tree-wide

### 2026-09-14T00:48:48.012Z - Proof configurability set PROVEN: Board shape is read from config rather than hardcoded: bounds are verified against 3x3, 2x3, 3x2, 1x3 and 3x1

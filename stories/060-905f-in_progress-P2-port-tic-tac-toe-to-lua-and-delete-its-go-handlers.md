---
id: 060-905f
title: Port tic-tac-toe to Lua and delete its Go handlers
status: in_progress
priority: P2
type: refactor
created: "2026-09-12T01:28:12.574Z"
updated: "2026-09-13T23:56:50.720Z"
dependencies: ["055", "057"]
plan: plans/lua-game-scripting.md
plan_step: Step 15
depends_on: ["stories/055-c8e1-pending-P2-shared-helper-library-in-lua-served-from-memory.md", "stories/057-1deb-pending-P2-indri-after-indri-at-and-indri-cancel.md"]
started_at: "2026-09-13T23:56:50.719Z"
---

# Port tic-tac-toe to Lua and delete its Go handlers

## Problem Statement

The example game is the proof that a game becomes config plus Lua with no Go compilation. Its Go move handler and its test suite must be replaced, not merely duplicated.

## Acceptance Criteria

- [ ] Every case from the deleted move handler test is re-expressed as a golden test of fixture state plus action to expected published delta
- [ ] Turn enforcement, out-of-bounds moves, occupied cells, row, column and diagonal wins, and a draw are all covered
- [ ] The example server handler directory is deleted
- [ ] The game runs on the stock cmd/server binary with no custom main
- [ ] VERIFY: go vet ./... && go test ./example/...

## Files

- example/tictactoe/game.lua
- example/tictactoe/main.go

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


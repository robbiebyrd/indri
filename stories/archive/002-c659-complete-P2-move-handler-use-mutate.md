---
id: 002-c659
status: complete
priority: P2
type: fix
created: "2026-09-09T18:40:21Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: BUG-1
completed_at: "2026-09-09T18:45:58.154Z"
updated: "2026-09-09T18:45:58.155Z"
---
# Route the tictactoe move handler through Store.Mutate

## Description
`example/tictactoe/server/handlers/move/handler.go` does two independent, non-atomic
`GameRepo.UpdateField` calls ("stage" then "teams") built from a stale in-memory read, instead
of the `Store.Mutate` CAS primitive the repo now provides for exactly this. Two racing moves can
lose an update (last $set wins, no version check), and a crash between the two writes leaves the
board advanced but the turn flag unflipped — an inconsistent, unrecoverable mid-turn state. This
is the one place in the tree that bypasses `Mutate`, contradicting ARCHITECTURE.md's rule that
every multi-field game edit goes through it.

## Context Files
- `example/tictactoe/server/handlers/move/handler.go` — the handler to rewrite (~lines 130,146)
- `internal/repo/game/team.go` — reference pattern for Mutate-based edits
- `internal/repo/game/game.go` — Store.Mutate

## Acceptance Criteria
- The move handler performs its stage + teams update inside a single `GameRepo.Mutate(gameID, func(g *models.Game) error { ... })`.
- No behavioral regression: a valid move still updates the board, flips the turn, and broadcasts a delta.
- Build, vet, and the move-handler tests green.

## Work Log

### 2026-09-09T18:45:58.071Z - Move handler now performs validation + board update + win check + turn flip inside a single GameRepo.Mutate; removed the two non-atomic UpdateField writes. build/vet/test green.


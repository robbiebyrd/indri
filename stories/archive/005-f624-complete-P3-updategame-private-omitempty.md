---
id: 005-f624
status: complete
priority: P3
type: fix
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: BUG-3
completed_at: "2026-09-09T19:07:07.689Z"
updated: "2026-09-09T19:07:07.689Z"
---
# Fix UpdateGame.Private omitempty data-loss landmine

## Description
`UpdateGame.Private` (`internal/models/game.go:52`) is tagged `bson:"private,omitempty"`. Because
`false` is the zero value for `bool`, `omitempty` drops the field from the `$set` whenever a caller
sets `Private:false` (e.g. to un-privatize a game): the write succeeds but silently fails to change
`private`. `Game`/`CreateGame` correctly lack `omitempty` here. Latent (no current caller) but a
real landmine.

## Acceptance Criteria
- `UpdateGame.Private` no longer silently drops `false` (remove `omitempty`, or use `*bool`).
- build/vet/test green.

## Work Log

### 2026-09-09T19:07:07.601Z - UpdateGame.Private: removed omitempty so a partial Update can set private:false.


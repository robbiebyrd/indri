---
id: 008-c546
status: complete
priority: P3
type: fix
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: DATA-1
completed_at: "2026-09-09T19:07:08.305Z"
updated: "2026-09-09T19:07:08.306Z"
---
# Translate duplicate-key errors on session/game creation

## Description
`session.Store.New` and `game.Service.New` are check-then-act; unique indexes on `userId`/`code`
prevent duplicates but the race loser gets a raw Mongo `E11000` instead of the intended behavior
(session: return the existing one; game: friendly "already exists").

## Acceptance Criteria
- `session.New` re-fetches and returns the existing session on duplicate-key.
- game creation surfaces the friendly "already exists" error.
- build/vet/test green.

## Work Log

### 2026-09-09T19:07:08.230Z - session.New returns the existing session on duplicate-key; game repo New surfaces the friendly already-exists error.


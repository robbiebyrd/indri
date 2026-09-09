---
id: 003-916b
status: complete
priority: P2
type: fix
created: "2026-09-09T18:40:21Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: BUG-2
completed_at: "2026-09-09T18:45:58.355Z"
updated: "2026-09-09T18:45:58.355Z"
---
# Propagate the discarded Write error in register's already-logged-in branch

## Description
In `internal/handlers/actions/register/handler.go`, the "already logged in" branch does
`_ = ss.Write(authExistsErrorMessage)` and then returns `nil`. If that write fails there is no
log and the caller sees success, while the client never received the message — a debugging dead
end. The success path in the same file correctly checks its write error, so this branch is
inconsistent.

## Context Files
- `internal/handlers/actions/register/handler.go:33`

## Acceptance Criteria
- The already-logged-in branch checks the `Write` error and returns it wrapped (`%w`), matching the success path.
- Build, vet, and tests green.

## Work Log

### 2026-09-09T18:45:58.274Z - register already-logged-in branch now checks and wraps the Write error instead of discarding it.


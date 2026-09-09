---
id: 004-6ca2
status: complete
priority: P3
type: chore
created: "2026-09-09T18:40:21Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: DOC-1
completed_at: "2026-09-09T18:45:58.553Z"
updated: "2026-09-09T18:45:58.553Z"
---
# Fix stale "Storer out of date" claim in ARCHITECTURE.md

## Description
`docs/ARCHITECTURE.md:108` says the repo `Storer` interfaces are "unused and, for `game`, out
of date." Commit e058d20 restored full parity and added `var _ Storer = (*Store)(nil)`
assertions, so "out of date" is now false and contradicts the code. ("Unused as an abstraction"
is still true — no consumer takes a `Storer`.)

## Acceptance Criteria
- Drop the "and, for `game`, out of date" claim.
- State the assertion is a compile-time drift guard and note nothing consumes the interface as an abstraction yet.

## Work Log

### 2026-09-09T18:45:58.474Z - ARCHITECTURE.md updated: describe Storer as a compile-time drift guard, drop the stale 'out of date' claim.


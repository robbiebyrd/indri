---
id: 010-d108
status: complete
priority: P3
type: refactor
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: TEST-2
completed_at: "2026-09-09T19:07:08.665Z"
updated: "2026-09-09T19:07:08.665Z"
---
# Add a reset hook for the router handler registry

## Description
`router.registeredHandlerMap` is a mutable package global with no reset. Harmless today (single
caller) but a latent test-isolation trap for any future test that re-registers or asserts on count.

## Acceptance Criteria
- A test-scoped reset exists (export_test.go helper or Reset() via t.Cleanup), or the registry is documented as append-only with a note not to depend on exact counts.
- build/vet/test green.

## Work Log

### 2026-09-09T19:07:08.600Z - added router.Reset(); boot test resets the registry via t.Cleanup.


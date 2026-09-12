---
id: 009-42ce
status: complete
priority: P3
type: test
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: TEST-1
completed_at: "2026-09-09T19:07:08.497Z"
updated: "2026-09-09T19:07:08.498Z"
---
# Assert correct handler wiring, not just action presence

## Description
`internal/services/boot/handlers_test.go` verifies each action name is registered but not that the
correct handler is bound. Swapping two entries (kick↔login) would still pass while breaking runtime.

## Acceptance Criteria
- Test asserts each action maps to its own handler (Name contains action, or handler package path matches the action dir).
- build/vet/test green.

## Work Log

### 2026-09-09T19:07:08.427Z - added TestRegisterHandlers_BindsCorrectHandler asserting each action maps to its own handler package.


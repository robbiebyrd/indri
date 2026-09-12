---
id: 007-d035
status: complete
priority: P3
type: fix
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: SEC-2
completed_at: "2026-09-09T19:07:08.107Z"
updated: "2026-09-09T19:07:08.107Z"
---
# Handle re-authentication on an already-authenticated connection

## Description
`register` refuses to run when the connection already has a `sessionId`, but `login`/`reconnect` have
no equivalent guard. An authenticated connection (user A) can `login` as user B and rebind its
`sessionId` without cleaning up A, leaving A showing `connected:true` forever. Stale-presence /
state-integrity bug, not privilege escalation.

## Acceptance Criteria
- Re-auth on a connection that already holds a session is either rejected until logout, or cleans up the previous session's presence first.
- build/vet/test green.

## Work Log

### 2026-09-09T19:07:08.026Z - login and reconnect now reject an already-authenticated connection; runtime-verified second login is rejected while normal login/create still work.


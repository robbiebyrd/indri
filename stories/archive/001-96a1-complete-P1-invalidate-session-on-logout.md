---
id: 001-96a1
status: complete
priority: P1
type: fix
created: "2026-09-09T18:40:21Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: SEC-1
started_at: "2026-09-09T18:41:32.426Z"
updated: "2026-09-09T18:43:59.800Z"
completed_at: "2026-09-09T18:43:59.799Z"
---
# Invalidate the session token on logout

## Description
The `logout` action (now wired in `internal/services/boot/handlers.go`) only calls
`HandleDisconnect` — it closes the websocket and marks the player disconnected, but never
revokes the session's bearer token. There is no delete/invalidate/revoke/TTL anywhere in the
session layer (verified by grep). The 256-bit token on the `Session` document stays valid
indefinitely, and `sessionRepo.New` even reuses it across logins, so anyone holding the token
can `{"action":"reconnect","sessionId":"<token>"}` and resume full authenticated access after
the user believes they logged out (CWE-613, Insufficient Session Expiration).

## Context Files
- `internal/handlers/actions/logout/handler.go` — logout entry point
- `internal/entrypoints/websocket.go` — HandleDisconnect (what logout currently does)
- `internal/repo/session/session.go`, `internal/repo/session/interface.go` — session store (no Delete today)
- `internal/services/session/session.go` — session service
- `internal/handlers/actions/reconnect/handler.go` — the resume path that must be closed

## Acceptance Criteria
- Session store gains a `Delete`/`Invalidate` method (added to `Storer` interface + assertion).
- `logout.Handle` invalidates (deletes or rotates the token of) the caller's session before/while disconnecting.
- After logout, a `reconnect` with the old token fails to authenticate (add a test if feasible).
- Defense in depth: a TTL / idle-expiry on the `session` collection so tokens are not valid forever even without explicit logout.
- Build, vet, and tests green.

## Work Log

### 2026-09-09T18:43:59.704Z - Added session Delete/Invalidate + Storer method + service method; logout now deletes the session after disconnect cleanup; added 7-day TTL index on the session collection as a backstop. Runtime-verified: reconnect with a post-logout token is rejected. Added session repo regression test (skips without Mongo). build/vet/test green.


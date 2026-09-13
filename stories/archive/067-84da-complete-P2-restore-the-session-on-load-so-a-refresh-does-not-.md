---
id: 067-84da
title: Restore the session on load so a refresh does not log you out
status: complete
priority: P2
type: fix
created: "2026-09-13T16:51:01.407Z"
updated: "2026-09-13T16:53:31.723Z"
dependencies: []
started_at: "2026-09-13T16:51:05.781Z"
completed_at: "2026-09-13T16:53:31.723Z"
---

# Restore the session on load so a refresh does not log you out

## Problem Statement

The client writes its session token to AsyncStorage on login but never reads it back, and message-handler.ts carries a TODO for reconnects. Any full page load - a refresh, or typing a route URL - therefore opens a new socket with no session, receives no keyframe, and drops the player back to the login screen with their game gone. The server already supports this: the reconnect action re-binds a stored session and follows with a full keyframe if the session was in a game.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] MessageHandler exposes a way to run something when the socket opens, without the provider reaching into the raw WebSocket
- [x] On open, a stored session token is read and a reconnect action is sent with it
- [x] No reconnect is sent when no token is stored, so a first-time visitor is unaffected
- [x] A failed or rejected reconnect leaves the app on the login screen rather than in a broken half-authenticated state
- [REJECTED] The token is cleared on logout, so logging out and refreshing does not silently log you back in (The client has no logout: nothing sends the logout action and no UI offers one, so there is no point at which to clear the token. Adding an uncalled clear function would be dead code. Needs a logout affordance first - recorded as a follow-up.)
- [x] Tests cover: open-with-token sends reconnect, open-without-token sends nothing, and the send happens after the socket is open rather than being dropped

## Files

- client/services/message-handler.ts
- client/providers/socket/socket-provider.tsx
- client/providers/user-state/user-state-actions.tsx

## Proof

- [x] [completeness] Completeness (6 of 7 criteria checked; criterion 6 rejected because the client has no logout to hook onto. 305/305 tests, typecheck clean, lint unchanged at 5 warnings.)
- [x] [feature-availability] Feature availability (Reconnect is sent only after the socket is open, which is the bug this fixes: send（） drops pre-handshake writes, so firing on mount restored nothing.)
- [x] [robustness] Robustness (No token means no reconnect, so a first-time visitor is unaffected. A throwing observer does not stop the next one. Unsubscribe stops a provider that unmounted first from acting. Mutation-verified: flipping the opened flag fails 2 tests.)
- [x] [resilience] Resilience (A failed restore is warned about and leaves the app unauthenticated - the login screen, which is a correct state rather than a broken half-authenticated one. Game state returns through the normal keyframe path with no special casing.)
- [x] [security] Security (The stored value is the session token the server already issued; reconnect re-binds it server-side and the server decides whether to honour it. Nothing new is trusted client-side. Noted gap: no logout means the token is never cleared.)
- [~] [defense-in-depth] Defense in depth (The server authorizes the reconnect; the client only presents a token it was given.)
- [x] [input-validation] Input validation (An empty or absent stored token is treated as no session rather than sent.)
- [~] [thread-safety] Thread safety (Single-threaded; observers fire synchronously on the socket open event.)
- [~] [configurability] Configurability (No configuration surface; the storage key matches what login already writes.)

## QA

305/305 tests, typecheck clean, lint unchanged. Open hook mutation-verified. Criterion 6 rejected: no logout exists in the client to clear the token on.

## Work Log

### 2026-09-13T16:53:30.390Z - MessageHandler gains onOpen(observer) returning an unsubscribe, and SocketProvider uses it to read the stored token and send reconnect. onOpen is the whole point: send() drops anything written before the handshake, so firing reconnect on mount would restore nothing - a test pins exactly that. An observer registered after the socket is already open fires immediately, so a late subscriber does not wait forever for an open that already happened. Two things the test runner taught me: node --experimental-strip-types is STRIP-ONLY, so a TypeScript parameter property (constructor(public url: string)) is a hard syntax error - the field has to be declared; and MessageHandler.send compares readyState against WebSocket.OPEN, so a WebSocket stand-in must carry the same static constants or every send silently looks closed. Both are recorded in comments in the test. Mutation-verified: flipping the opened flag fails 2 tests. CRITERION 6 REJECTED, not quietly skipped: it asks for the token to be cleared on logout, but the client has no logout at all - nothing sends the logout action and no UI offers it, so there is nothing to hook onto. Adding a clear function nothing calls would be dead code pretending to be a fix. Recorded as a follow-up.


### 2026-09-13T16:53:31.051Z - Proof completeness set PROVEN: 6 of 7 criteria checked; criterion 6 rejected because the client has no logout to hook onto. 305/305 tests, typecheck clean, lint unchanged at 5 warnings.

### 2026-09-13T16:53:31.127Z - Proof robustness set PROVEN: No token means no reconnect, so a first-time visitor is unaffected. A throwing observer does not stop the next one. Unsubscribe stops a provider that unmounted first from acting. Mutation-verified: flipping the opened flag fails 2 tests.

### 2026-09-13T16:53:31.205Z - Proof resilience set PROVEN: A failed restore is warned about and leaves the app unauthenticated - the login screen, which is a correct state rather than a broken half-authenticated one. Game state returns through the normal keyframe path with no special casing.

### 2026-09-13T16:53:31.280Z - Proof feature-availability set PROVEN: Reconnect is sent only after the socket is open, which is the bug this fixes: send() drops pre-handshake writes, so firing on mount restored nothing.

### 2026-09-13T16:53:31.347Z - Proof security set PROVEN: The stored value is the session token the server already issued; reconnect re-binds it server-side and the server decides whether to honour it. Nothing new is trusted client-side. Noted gap: no logout means the token is never cleared.

### 2026-09-13T16:53:31.413Z - Proof input-validation set PROVEN: An empty or absent stored token is treated as no session rather than sent.

### 2026-09-13T16:53:31.479Z - Proof defense-in-depth set NOT_APPLICABLE: The server authorizes the reconnect; the client only presents a token it was given.

### 2026-09-13T16:53:31.545Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded; observers fire synchronously on the socket open event.

### 2026-09-13T16:53:31.614Z - Proof configurability set NOT_APPLICABLE: No configuration surface; the storage key matches what login already writes.

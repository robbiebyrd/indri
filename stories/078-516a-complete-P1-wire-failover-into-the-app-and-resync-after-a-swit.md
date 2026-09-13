---
id: 078-516a
title: Wire failover into the app and resync after a switch
status: complete
priority: P1
type: feature
created: "2026-09-13T23:20:21.357Z"
updated: "2026-09-13T23:40:40.334Z"
dependencies: ["075-3eb2", "077-6dbe"]
plan: plans/client-transport-failover.md
plan_step: Step 6
depends_on: ["stories/075-3eb2-pending-P2-sse-and-rest-client-transport.md", "stories/077-6dbe-pending-P1-run-session-restore-on-every-connect-not-just-the-.md"]
started_at: "2026-09-13T23:38:41.913Z"
completed_at: "2026-09-13T23:40:40.334Z"
---

# Wire failover into the app and resync after a switch

## Problem Statement

The supervisor has to be assembled with the real candidates and given the resync contract. A switch lands on a connection whose delta stream starts mid-flight, and GameStateParser needs a keyframe to replay onto, so reconnect alone is not enough - refresh has to follow or the board silently stops updating.

## Acceptance Criteria

- [x] SocketProvider builds the supervisor with WebRTC, WebSocket and conditionally SSE as candidates
- [x] SSE is only offered as a candidate once a session token exists
- [x] On each connect the stored token is replayed via reconnect before any queued application message
- [x] A refresh follows reconnect so the parser gets a keyframe to replay onto
- [x] MessageHandler's public surface is unchanged, so no consumer is touched
- [REJECTED] [MANUAL] Killing each channel in turn against a live server moves play to the next one without a reload ([MANUAL] Requires a live server and killing each channel by hand; cannot be automated in this environment)
- [x] VERIFY: cd client && pnpm run typecheck

## Files

- client/providers/socket/socket-provider.tsx

## Proof

- [x] [completeness] Completeness (6 of 7 criteria verified; criterion 6 is tagged [MANUAL] and is listed in QA. Ordering pinned by failover-transport.node-test.ts 'an onOpen observer's sends precede the flushed queue')
- [x] [feature-availability] Feature availability (All three transports wired in SocketProvider; typecheck clean; MessageHandler's constructor signature untouched so no consumer changed)
- [x] [robustness] Robustness (loadToken failure is caught and warned, still subscribing the restore observer; a missing token is treated as first visit, not an error)
- [x] [resilience] Resilience (reconnect+refresh run on EVERY connect, so a switch restores identity and gets a fresh keyframe; test asserts the resync repeats on the new channel and replays no application message)
- [x] [security] Security (Removed console.log（'Storing data...', value） from user-state-actions.tsx, which printed the bearer token on every login. session-token.ts never logs the token; SSE stays out of the candidate list until a token exists)
- [x] [defense-in-depth] Defense in depth (Fresh transport instances per cycle so a closed channel cannot be redialled; loadToken normalises an empty string to null so one null test covers every caller)
- [~] [input-validation] Input validation (No external input; the token comes from the app's own storage and the server's authenticated response)
- [~] [thread-safety] Thread safety (Single-threaded; the async-ordering hazard （an awaited storage read racing the queue flush） is exactly what the synchronous cache removes)
- [x] [configurability] Configurability (FAILOVER constant in socket-provider.tsx holds attemptTimeoutMs/backoffMs/queueLimit in one named place)

## QA

- [ ] Run a live server and kill each channel in turn (WebRTC, then WebSocket, then SSE) and confirm play continues without a reload

## Work Log

### 2026-09-13T23:40:24.651Z - SocketProvider now builds a FailoverTransport over WebRTC, WebSocket and - only once a token exists - SseRestTransport, with fresh instances per selection cycle. Added services/session-token.ts: an in-memory cache of the bearer token, because the supervisor flushes its queue the instant a channel opens and an awaited AsyncStorage read would put reconnect behind the queued application messages. user-state-actions.tsx writes through it, which also removes a console.log that printed the bearer token on every login. The restore observer subscribes only after the token is cached, so it can send synchronously; onOpen's run-if-already-connected makes that free. On each connect it sends reconnect then refresh. Ordering is pinned by a test in failover-transport.node-test.ts, since socket-provider.tsx cannot run under the node test runner. 396 tests pass, typecheck clean, no new lint warnings.


### 2026-09-13T23:40:39.564Z - Proof completeness set PROVEN: 6 of 7 criteria verified; criterion 6 is tagged [MANUAL] and is listed in QA. Ordering pinned by failover-transport.node-test.ts 'an onOpen observer's sends precede the flushed queue'

### 2026-09-13T23:40:39.635Z - Proof feature-availability set PROVEN: All three transports wired in SocketProvider; typecheck clean; MessageHandler's constructor signature untouched so no consumer changed

### 2026-09-13T23:40:39.709Z - Proof robustness set PROVEN: loadToken failure is caught and warned, still subscribing the restore observer; a missing token is treated as first visit, not an error

### 2026-09-13T23:40:39.782Z - Proof resilience set PROVEN: reconnect+refresh run on EVERY connect, so a switch restores identity and gets a fresh keyframe; test asserts the resync repeats on the new channel and replays no application message

### 2026-09-13T23:40:39.855Z - Proof security set PROVEN: Removed console.log('Storing data...', value) from user-state-actions.tsx, which printed the bearer token on every login. session-token.ts never logs the token; SSE stays out of the candidate list until a token exists

### 2026-09-13T23:40:39.939Z - Proof defense-in-depth set PROVEN: Fresh transport instances per cycle so a closed channel cannot be redialled; loadToken normalises an empty string to null so one null test covers every caller

### 2026-09-13T23:40:40.019Z - Proof input-validation set NOT_APPLICABLE: No external input; the token comes from the app's own storage and the server's authenticated response

### 2026-09-13T23:40:40.100Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded; the async-ordering hazard (an awaited storage read racing the queue flush) is exactly what the synchronous cache removes

### 2026-09-13T23:40:40.183Z - Proof configurability set PROVEN: FAILOVER constant in socket-provider.tsx holds attemptTimeoutMs/backoffMs/queueLimit in one named place

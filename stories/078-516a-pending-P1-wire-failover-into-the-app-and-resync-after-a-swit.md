---
id: 078-516a
title: Wire failover into the app and resync after a switch
status: pending
priority: P1
type: feature
created: "2026-09-13T23:20:21.357Z"
updated: "2026-09-13T23:20:27.307Z"
dependencies: ["075-3eb2", "077-6dbe"]
plan: plans/client-transport-failover.md
plan_step: Step 6
depends_on: ["stories/075-3eb2-pending-P2-sse-and-rest-client-transport.md", "stories/077-6dbe-pending-P1-run-session-restore-on-every-connect-not-just-the-.md"]
---

# Wire failover into the app and resync after a switch

## Problem Statement

The supervisor has to be assembled with the real candidates and given the resync contract. A switch lands on a connection whose delta stream starts mid-flight, and GameStateParser needs a keyframe to replay onto, so reconnect alone is not enough - refresh has to follow or the board silently stops updating.

## Acceptance Criteria

- [ ] SocketProvider builds the supervisor with WebRTC, WebSocket and conditionally SSE as candidates
- [ ] SSE is only offered as a candidate once a session token exists
- [ ] On each connect the stored token is replayed via reconnect before any queued application message
- [ ] A refresh follows reconnect so the parser gets a keyframe to replay onto
- [ ] MessageHandler's public surface is unchanged, so no consumer is touched
- [ ] [MANUAL] Killing each channel in turn against a live server moves play to the next one without a reload
- [ ] VERIFY: cd client && pnpm run typecheck

## Files

- client/providers/socket/socket-provider.tsx

## Proof

- [ ] [completeness] Completeness
- [ ] [feature-availability] Feature availability
- [ ] [robustness] Robustness
- [ ] [resilience] Resilience
- [ ] [security] Security
- [ ] [defense-in-depth] Defense in depth
- [ ] [input-validation] Input validation
- [ ] [thread-safety] Thread safety
- [ ] [configurability] Configurability

## Work Log


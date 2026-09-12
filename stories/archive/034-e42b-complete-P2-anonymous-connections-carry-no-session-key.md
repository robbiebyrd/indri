---
id: 034-e42b
title: Anonymous connections carry no session key
status: complete
priority: P2
type: refactor
created: "2026-09-12T01:27:30.640Z"
updated: "2026-09-12T01:37:40.159Z"
dependencies: []
plan: plans/webrtc-transport.md
plan_step: Step 1
completed_at: "2026-09-12T01:37:40.159Z"
---

# Anonymous connections carry no session key

## Problem Statement

transport.NewKeys always sets SessionIDKey, so a WebRTC peer that has not logged in yet would register with an empty session id and could be matched by any broadcast filter comparing against the empty string. An anonymous connection must carry no session key at all until a login binds one.

## Acceptance Criteria

- [x] NewKeys with an empty id leaves SessionIDKey absent from the connection keys
- [x] NewKeys with a non-empty id still sets SessionIDKey, so SSE and GraphQL behaviour is unchanged
- [x] VERIFY: go test -race ./internal/transport/

## Files

- internal/transport/keys.go
- internal/transport/keys_test.go

## Proof

- [x] [completeness] Completeness (Both branches tested: TestKeys_EmptyIDLeavesSessionIDKeyAbsent and TestKeys_SessionIDIsSetOnConstruction.)
- [~] [feature-availability] Feature availability (Internal helper; no client-facing feature is added or removed.)
- [x] [robustness] Robustness (The empty string is the edge case this story exists for, and it is covered by a test.)
- [~] [resilience] Resilience (A map constructor has no failure or recovery path.)
- [x] [security] Security (This is itself the security fix: an unauthenticated connection can no longer be matched by a broadcast filter comparing against the empty string.)
- [x] [defense-in-depth] Defense in depth (Absence of the key removes the match surface entirely, while BroadcastService still resolves recipients through the session store first.)
- [~] [input-validation] Input validation (sessionID originates from the servers own session store, not from client input.)
- [x] [thread-safety] Thread safety (The existing Keys mutex is untouched, the map is built before publication, and go test -race is clean.)
- [~] [configurability] Configurability (No configuration is involved.)

## QA

go test -race ./internal/transport/ green; verified independently by the parent session.

## Work Log

### 2026-09-12T01:36:38.909Z - NewKeys with an empty id now leaves SessionIDKey unset, so an anonymous WebRTC peer cannot be matched by a broadcast filter comparing against the empty string. TDD: RED confirmed before the fix. SSE and GraphQL pass unchanged.


### 2026-09-12T01:37:17.708Z - Proof completeness set PROVEN: Both branches tested: TestKeys_EmptyIDLeavesSessionIDKeyAbsent and TestKeys_SessionIDIsSetOnConstruction.

### 2026-09-12T01:37:17.786Z - Proof feature-availability set NOT_APPLICABLE: Internal helper; no client-facing feature is added or removed.

### 2026-09-12T01:37:35.005Z - Proof robustness set PROVEN: The empty string is the edge case this story exists for, and it is covered by a test.

### 2026-09-12T01:37:35.086Z - Proof resilience set NOT_APPLICABLE: A map constructor has no failure or recovery path.

### 2026-09-12T01:37:35.163Z - Proof security set PROVEN: This is itself the security fix: an unauthenticated connection can no longer be matched by a broadcast filter comparing against the empty string.

### 2026-09-12T01:37:35.241Z - Proof defense-in-depth set PROVEN: Absence of the key removes the match surface entirely, while BroadcastService still resolves recipients through the session store first.

### 2026-09-12T01:37:35.320Z - Proof input-validation set NOT_APPLICABLE: sessionID originates from the servers own session store, not from client input.

### 2026-09-12T01:37:35.396Z - Proof thread-safety set PROVEN: The existing Keys mutex is untouched, the map is built before publication, and go test -race is clean.

### 2026-09-12T01:37:35.474Z - Proof configurability set NOT_APPLICABLE: No configuration is involved.

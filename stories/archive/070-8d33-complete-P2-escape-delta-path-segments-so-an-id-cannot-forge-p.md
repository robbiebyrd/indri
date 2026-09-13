---
id: 070-8d33
title: Escape delta path segments so an id cannot forge privateData
status: complete
priority: P2
type: fix
created: "2026-09-13T18:49:29.773Z"
updated: "2026-09-13T18:57:24.559Z"
dependencies: []
completed_at: "2026-09-13T18:57:24.559Z"
---

# Escape delta path segments so an id cannot forge privateData

## Problem Statement

events.joinPath builds delta paths by raw concatenation (prefix + dot + key) and pathHasSegment splits them back on a dot to find privateData. A map key containing a dot therefore forges a path segment: a widget id of foo.privateData yields a trailing segment that is literally privateData, so SanitizeDelta silently drops every update to that widget from the broadcast. findReservedKey uses exact key equality so it does not catch it. The same root cause was found independently against the Lua plan for arbitrary script table keys, so fixing it at the events layer closes both rather than adding a charset allowlist at each new producer.

## Acceptance Criteria

- [x] A map key containing a dot cannot produce a delta path that collides with a nested path
- [x] A key literally named or ending in privateData is not silently swallowed by SanitizeDelta
- [x] The client side of the protocol still parses the paths it is sent, or the change is confined to comparison and not the wire format
- [x] Existing delta and sanitize tests still pass unchanged where the behaviour is unchanged
- [x] A regression test covers the foo.privateData forgery specifically
- [x] VERIFY: go test -race ./internal/services/events/

## Files

- internal/services/events/delta.go

## Proof

- [x] [completeness] Completeness (6 of 6 criteria; escaping chosen over a segment array or a boundary charset rule, with the reasoning recorded)
- [x] [feature-availability] Feature availability (Server encoder and client decoder changed in the same edit, so the wire format and the reducer never disagree)
- [x] [robustness] Robustness (Escaping is the identity function on every key the server generates today - ObjectID hex, json tag names, hand-built paths - so UpdateField and DeleteField, which publish raw paths not routed through joinPath, still agree with the differ)
- [~] [resilience] Resilience (Pure path encoding with no failure path of its own)
- [x] [security] Security (A widget id of foo.privateData can no longer forge a path segment and have its updates silently dropped from the broadcast; the reverse is also covered, so the encoding cannot smuggle a real private field past the filter)
- [x] [defense-in-depth] Defense in depth (Sanitize and SanitizeDelta still agree, which CLAUDE.md requires, and an exact privateData key is still dropped on both paths)
- [x] [input-validation] Input validation (splitPath is escape-aware rather than scanning for a raw dot, so interpretation matches generation instead of approximating it)
- [~] [thread-safety] Thread safety (Stateless string encoding)
- [~] [configurability] Configurability (Wire format detail, not a tunable)

## QA

Verified independently: go build, vet and full suite green under -race; client typecheck clean and 350/350 tests pass. Agent proved RED on both sides - neutering escapeSegment fails 6 Go subtests including the silent-drop case, and reverting the client to split on dot fails both dotted-id tests. 6/6.

## Work Log

### 2026-09-13T18:56:07.587Z - Fixed the forged-segment defect by escaping document keys in the delta path encoder, rather than restricting keys at each producer. events.joinPath now escapes '.' and '\' inside a key with a backslash (escapeSegment); pathHasSegment decides on segments decoded by the new splitPath instead of a raw split, so a widget id of 'foo.privateData' is one segment, is not a privateData field, and is broadcast normally. Escaping is identity for every key the server generates today (object ids, json tag names), and splitPath is identity-equivalent to a plain split for the hand-built paths UpdateField/DeleteField/playerConnectedKey publish, so both producers agree. Client reducer updated in the same change: exported splitDeltaPath in client/services/game-state-parser.ts decodes the same escaping and replaces path.split('.') in both updateJSONKeyByDotPath and deleteJSONKeyByDotPath. docs/PROTOCOL.md now documents the escaping as part of the wire contract. GameService.Sanitize and SanitizeDelta still agree: an exact 'privateData' key is dropped on both paths, and an escaped spelling that decodes to privateData is dropped too, so escaping cannot smuggle a private field past the filter. Tests: table-driven escape/split round trip (delta_internal_test.go), Diff+SanitizeDelta forgery regression for 'foo.privateData' including the removal case, and a decoded-segment table over the wire format (delta_test.go); three client node tests covering the decoder and both dotted-id apply/remove paths. Both sides proved RED by temporarily reverting the encoder/split, then GREEN. Verified: go build ./... && go vet ./... && go test -race ./... all pass; client pnpm run typecheck clean and 350/350 node tests pass. Existing delta and sanitize tests are unchanged.


### 2026-09-13T18:57:21.333Z - Proof completeness set PROVEN: 6 of 6 criteria; escaping chosen over a segment array or a boundary charset rule, with the reasoning recorded

### 2026-09-13T18:57:21.401Z - Proof feature-availability set PROVEN: Server encoder and client decoder changed in the same edit, so the wire format and the reducer never disagree

### 2026-09-13T18:57:21.466Z - Proof robustness set PROVEN: Escaping is the identity function on every key the server generates today - ObjectID hex, json tag names, hand-built paths - so UpdateField and DeleteField, which publish raw paths not routed through joinPath, still agree with the differ

### 2026-09-13T18:57:21.531Z - Proof resilience set NOT_APPLICABLE: Pure path encoding with no failure path of its own

### 2026-09-13T18:57:21.599Z - Proof security set PROVEN: A widget id of foo.privateData can no longer forge a path segment and have its updates silently dropped from the broadcast; the reverse is also covered, so the encoding cannot smuggle a real private field past the filter

### 2026-09-13T18:57:21.666Z - Proof defense-in-depth set PROVEN: Sanitize and SanitizeDelta still agree, which CLAUDE.md requires, and an exact privateData key is still dropped on both paths

### 2026-09-13T18:57:21.744Z - Proof input-validation set PROVEN: splitPath is escape-aware rather than scanning for a raw dot, so interpretation matches generation instead of approximating it

### 2026-09-13T18:57:21.815Z - Proof thread-safety set NOT_APPLICABLE: Stateless string encoding

### 2026-09-13T18:57:21.891Z - Proof configurability set NOT_APPLICABLE: Wire format detail, not a tunable

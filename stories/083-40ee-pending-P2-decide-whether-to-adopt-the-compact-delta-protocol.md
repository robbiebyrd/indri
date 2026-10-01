---
id: 083-40ee
title: Decide whether to adopt the compact delta protocol
status: pending
priority: P2
type: task
created: "2026-10-01T13:52:29.872Z"
updated: "2026-10-01T13:52:39.842Z"
dependencies: ["080-cf52"]
plan: plans/main-divergence-integration.md
plan_step: Step 6
depends_on: ["stories/080-cf52-pending-P1-add-sqlite-and-postgres-game-backends-against-loca.md"]
---

# Decide whether to adopt the compact delta protocol

## Problem Statement

origin/main replaced the dotted-path delta with positional integer-array encoding over MessagePack, plus layout frames and keyframe-on-shape-change. It buys payload size but costs the client parser, every scripttest fixture, and local's escaped-key path support, since remote splits paths on a plain dot. This is a genuine decision, not a carry, and the plan's default is to decline it.

## Acceptance Criteria

- [ ] A written comparison of payload size against migration cost is produced before any code is written
- [ ] The decision is recorded in the story worklog naming explicitly what is given up
- [ ] If declined: local's dotted-path delta is retained and the compact protocol is filed as its own plan
- [ ] If adopted: SanitizeDelta's pair-array form strips exactly what the map form stripped, so it still agrees with GameService.Sanitize
- [ ] If adopted: escaped-key paths still round-trip, or the loss is recorded as accepted
- [ ] If adopted: every scripttest fixture and the client parser are updated and green
- [ ] game-state-parser.bench.node-test.ts is re-baselined either way

## Files

- internal/services/events/
- client/services/game-state-parser.ts
- internal/services/scripttest/

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

### 2026-10-01T14:14:33.690Z - MEASURED on the real tictactoe config (scripts in .integration/scratch/). Decisive finding: no compression is enabled anywhere -- melody.New() leaves gorilla EnableCompression false, so every byte goes raw today. For a 9-move game, total delta bytes: A (today, JSON+dotted) 2007 raw / 292 with permessage-deflate; D (remote's full compact format) 873 raw / 205 deflated. Enabling deflate alone gets 85% off; the entire compact-delta migration then buys a further 30% (292->205, i.e. 87 bytes per game). Positional encoding and deflate target the SAME redundancy (repeated string paths), so they do not stack. Separately the keyframe IS worth fixing and is orthogonal: 4936 bytes today vs 754 once layout is split onto its own content-hash-versioned frame sent once per connection (layout is 3159 of the 4936, resent on every join/refresh/reconnect).

### 2026-10-01T14:15:50.904Z - RN compression check (partial). Client constructs the global WebSocket (client/services/transport.ts:156), so compression support is per-platform. Verified in the installed RN 0.79.6 tree: no deflate/permessage/compress handling anywhere in RN's WebSocket JS layer; Android's WebSocketModule.kt:82-89 builds OkHttpClient.Builder() with only timeouts configured. NOT settled from this tree: whether OkHttp negotiates permessage-deflate by default on Android (OkHttp is an external dep, not vendored here), and what iOS actually uses in 0.79. Expo web gets browser-native compression. DECISIVE TEST NEEDED: enable gorilla EnableCompression server-side, connect from web/iOS/Android, and observe whether the handshake response carries Sec-WebSocket-Extensions: permessage-deflate. FALLBACK if iOS lacks it: application-layer deflate on the payload before Write with the client inflating -- keeps dotted paths and escaped-key support, and is far less work than the positional migration.


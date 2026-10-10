---
id: 083-40ee
title: Decide whether to adopt the compact delta protocol
status: complete
priority: P2
type: task
created: "2026-10-01T13:52:29.872Z"
updated: "2026-10-04T18:33:52.000Z"
dependencies: ["080-cf52"]
plan: plans/main-divergence-integration.md
plan_step: Step 6
depends_on: ["stories/080-cf52-pending-P1-add-sqlite-and-postgres-game-backends-against-loca.md"]
---

# Decide whether to adopt the compact delta protocol

## Problem Statement

origin/main replaced the dotted-path delta with positional integer-array encoding over MessagePack, plus layout frames and keyframe-on-shape-change. It buys payload size but costs the client parser, every scripttest fixture, and local's escaped-key path support, since remote splits paths on a plain dot. This is a genuine decision, not a carry, and the plan's default is to decline it.

## Acceptance Criteria

- [x] A written comparison of payload size against migration cost is produced before any code is written
- [x] The decision is recorded in the story worklog naming explicitly what is given up
- [x] If declined: local's dotted-path delta is retained and the compact protocol is filed as its own plan
- [x] Not applicable (declined) — SanitizeDelta's pair-array form strips exactly what the map form stripped, so it still agrees with GameService.Sanitize
- [x] Not applicable (declined) — escaped-key paths still round-trip, or the loss is recorded as accepted
- [x] Not applicable (declined) — every scripttest fixture and the client parser are updated and green
- [x] game-state-parser.bench.node-test.ts is re-baselined either way

## Files

- internal/services/events/
- client/services/game-state-parser.ts
- internal/services/scripttest/

## Proof

- [x] [completeness] Completeness
- [x] [feature-availability] Feature availability
- [x] [robustness] Robustness
- [x] [resilience] Resilience
- [x] [security] Security
- [x] [defense-in-depth] Defense in depth
- [x] [input-validation] Input validation
- [x] [thread-safety] Thread safety
- [x] [configurability] Configurability

## Work Log

### 2026-10-01T14:14:33.690Z - MEASURED on the real tictactoe config (scripts in .integration/scratch/). Decisive finding: no compression is enabled anywhere -- melody.New() leaves gorilla EnableCompression false, so every byte goes raw today. For a 9-move game, total delta bytes: A (today, JSON+dotted) 2007 raw / 292 with permessage-deflate; D (remote's full compact format) 873 raw / 205 deflated. Enabling deflate alone gets 85% off; the entire compact-delta migration then buys a further 30% (292->205, i.e. 87 bytes per game). Positional encoding and deflate target the SAME redundancy (repeated string paths), so they do not stack. Separately the keyframe IS worth fixing and is orthogonal: 4936 bytes today vs 754 once layout is split onto its own content-hash-versioned frame sent once per connection (layout is 3159 of the 4936, resent on every join/refresh/reconnect).

### 2026-10-01T14:15:50.904Z - RN compression check (partial). Client constructs the global WebSocket (client/services/transport.ts:156), so compression support is per-platform. Verified in the installed RN 0.79.6 tree: no deflate/permessage/compress handling anywhere in RN's WebSocket JS layer; Android's WebSocketModule.kt:82-89 builds OkHttpClient.Builder() with only timeouts configured. NOT settled from this tree: whether OkHttp negotiates permessage-deflate by default on Android (OkHttp is an external dep, not vendored here), and what iOS actually uses in 0.79. Expo web gets browser-native compression. DECISIVE TEST NEEDED: enable gorilla EnableCompression server-side, connect from web/iOS/Android, and observe whether the handshake response carries Sec-WebSocket-Extensions: permessage-deflate. FALLBACK if iOS lacks it: application-layer deflate on the payload before Write with the client inflating -- keeps dotted paths and escaped-key support, and is far less work than the positional migration.


### 2026-10-03T04:30:15.000Z - DECISION (Boss). Decline the compact delta protocol; keep local's dotted-path JSON deltas. Instead: (1) enable permessage-deflate server-side and verify the handshake (Sec-WebSocket-Extensions: permessage-deflate) from web, iOS and Android; if a native platform does not negotiate it, fall back to application-layer deflate of the payload; (2) split layout out of the keyframe onto its own content-hash-versioned frame sent once per connection (measured 4936 -> 754 bytes). Given up, explicitly: the further ~30% on deflated deltas (292 -> 205 bytes per 9-move game), MessagePack framing, and alignment with origin/main's wire format -- the later rebase onto main will conflict on the delta path. Kept: the client parser as is, every scripttest fixture, and escaped-key path support. The compact protocol is to be filed as its own plan, not implemented.

### 2026-10-04T18:33:52.000Z - CLOSED. Declined, per Boss's decision above. Local's dotted-path JSON delta is retained unchanged. The compact protocol is filed as its own plan, plans/compact-delta-protocol.md, with the measurements, the cost of adopting it, and the condition that would reopen it. The two pieces Boss chose instead are filed as stories: 093 (enable WebSocket compression and verify it negotiates per platform) and 094 (send the layout once per connection instead of in every keyframe). Re-baselined game-state-parser.bench.node-test.ts on the unchanged parser (node v26.10.0, this machine): 200 widgets, 4 grid levels, 179 leaves, 49,933 bytes serialised; deep clone alone median 0.240 ms; reapply median 0.253 / 0.261 / 0.399 / 1.016 ms at 1 / 10 / 100 / 500 retained deltas. The If-adopted criteria are marked not applicable rather than met.


---
id: 093-2c8a
title: Enable WebSocket compression and verify it negotiates on every platform
status: in_progress
priority: P2
type: feature
created: "2026-10-04T18:33:52.000Z"
updated: "2026-10-04T19:17:36.000Z"
dependencies: ["083-40ee"]
---

# Enable WebSocket compression and verify it negotiates on every platform

## Problem Statement

Story 083 found no compression anywhere: melody.New() leaves gorilla's EnableCompression false, so every delta goes raw. permessage-deflate alone cuts a nine-move game's deltas from 2007 to 292 bytes. Boss chose this over the compact delta protocol. It is only worth anything where the client negotiates it: browsers do; React Native's WebSocket on iOS and Android was not settled (RN 0.79.6's JS layer has no deflate handling; Android builds OkHttp with only timeouts; iOS unknown).

## Acceptance Criteria

- [x] The WebSocket transport offers permessage-deflate, and a Go client that asks for it gets Sec-WebSocket-Extensions: permessage-deflate back (test)
- [x] A client that does not ask still connects, uncompressed (test)
- [ ] The handshake is checked from web, iOS and Android builds of the client, and the result recorded per platform
- [ ] If a native platform does not negotiate it: application-layer deflate of the payload is offered as the fallback, or the gap is recorded and Boss decides
- [x] VERIFY: go test -race ./internal/transport/ws/

## Work Log

### 2026-10-04T19:17:36.000Z - IMPLEMENTED (server). ws.New sets m.Upgrader.EnableCompression = true. compression_test.go drives a raw TCP handshake and parses the server's frames byte for byte: TestCompression_NegotiatedWhenTheClientOffersIt (extension answered, every frame RSV1-compressed, fewer wire bytes) failed first ("answered \"\"") and passes now; TestCompression_AClientThatDoesNotAskConnectsUncompressed (no extension answered, plain frames) passes. go test -race ./internal/transport/... green.

### 2026-10-04T19:17:36.000Z - FINDING, needs Boss. gorilla/websocket (melody's dependency) implements permessage-deflate WITHOUT context takeover only: it answers "permessage-deflate; server_no_context_takeover; client_no_context_takeover", so each message is deflated alone. Story 083's 2007 -> 292 figure (.integration/scratch/stream.py) modelled one deflate stream per connection, i.e. context takeover. Measured on the wire here, the same nine-move tictactoe game: 1926 bytes uncompressed -> 1390 compressed (-28%), not -85%. Modelling per-message deflate on 083's exact data gives 2007 -> 1436, which agrees. The 083 decision to decline the compact protocol rested on the 85% figure. Options for Boss: (a) accept -28%; (b) replace gorilla/melody with a library that supports context takeover (coder/websocket does, and is already a test dependency) -- a transport rewrite inside internal/transport/ws only; (c) application-layer deflate with one stream per connection (context takeover by hand, client inflates); (d) revisit the compact protocol plan.

### 2026-10-04T19:17:36.000Z - PLATFORMS. Web: Chrome 154 (Playwright, page on the server's own origin, INDRI_ALLOWED_ORIGINS overridden to it for the check) reports ws.extensions = "permessage-deflate; server_no_context_takeover; client_no_context_takeover" -- negotiated. iOS and Android: NOT checked -- needs a device or simulator build of the client, which this session cannot run; left for Boss. The fallback criterion waits on those two results and on the finding above.


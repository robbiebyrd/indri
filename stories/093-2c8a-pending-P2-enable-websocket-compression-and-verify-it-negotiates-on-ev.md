---
id: 093-2c8a
title: Enable WebSocket compression and verify it negotiates on every platform
status: pending
priority: P2
type: feature
created: "2026-10-04T18:33:52.000Z"
updated: "2026-10-04T18:33:52.000Z"
dependencies: ["083-40ee"]
---

# Enable WebSocket compression and verify it negotiates on every platform

## Problem Statement

Story 083 found no compression anywhere: melody.New() leaves gorilla's EnableCompression false, so every delta goes raw. permessage-deflate alone cuts a nine-move game's deltas from 2007 to 292 bytes. Boss chose this over the compact delta protocol. It is only worth anything where the client negotiates it: browsers do; React Native's WebSocket on iOS and Android was not settled (RN 0.79.6's JS layer has no deflate handling; Android builds OkHttp with only timeouts; iOS unknown).

## Acceptance Criteria

- [ ] The WebSocket transport offers permessage-deflate, and a Go client that asks for it gets Sec-WebSocket-Extensions: permessage-deflate back (test)
- [ ] A client that does not ask still connects, uncompressed (test)
- [ ] The handshake is checked from web, iOS and Android builds of the client, and the result recorded per platform
- [ ] If a native platform does not negotiate it: application-layer deflate of the payload is offered as the fallback, or the gap is recorded and Boss decides
- [ ] VERIFY: go test -race ./internal/transport/ws/

## Work Log

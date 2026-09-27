---
status: complete
approved_at: "2026-09-27"
---
# Pluggable client transports

**Created:** 2026-09-27 | **Effort:** L | **Branch:** `wip/pluggable-transports`

## Summary

Let deployments serve clients over WebSocket, REST+SSE, GraphQL subscriptions, or WebRTC data channels —
or several at once — and give the Expo client a package that sets up whichever transport is active.

The transport-agnostic interface already existed (`internal/transport/transport.go`, with melody
confined to `transport/ws`), and everything above it already programmed against it. So the work was:
run several transports concurrently, add three implementations, build the JS client package, and fix
docs that still described the pre-refactor melody-everywhere layout.

## Decisions

- **GraphQL is framing, not a schema.** Actions are registered dynamically from a game's script, so a
  static per-action schema can't exist. `graphqlws` speaks `graphql-transport-ws` over one socket: an
  `IndriEvents` subscription carries server messages, and each client message is a `Send` operation.
  The server executes no GraphQL and ignores query text, so no Apollo/urql compatibility is claimed.
- **Binary-safe.** `feat/compact-delta-protocol` (unmerged) moves the wire format to MessagePack, so
  `Conn` gained `WriteBinary`. SSE and graphqlws base64 binary frames; WebRTC and WS send them natively.
  Merge note: that branch's `WriteEncoded` should call `WriteBinary`. Today it calls `Write`, which
  melody sends as a text frame, and browsers reject non-UTF-8 text frames (close 1007).
- **WebRTC signaling is one POST with complete ICE** — self-contained, no dependency on another
  transport. No TURN; ICE servers default to none.
- **Shared guarantees live in one place.** `QueuedConn`/`Hub` give the non-melody transports:
  - a single writer per connection;
  - serial inbound delivery (`login` does check-then-set);
  - exactly-once `Disconnect`;
  - queued writes flushed on a server-side close.

  `transporttest` is the conformance suite every transport, `ws` included, must pass.
- **Multi-instance:** SSE and WebRTC need IP-hash sticky sessions behind a load balancer. Relaying
  over Redis was judged over-engineering. An SSE send to the wrong instance gets `404` and reconnects.
- **No client auto-reconnect** until the session-resume handshake exists. Without it, a reconnect
  lands on the login scene while the old player is marked disconnected.

## Server (done)

| Commit | What |
|---|---|
| shared plumbing | `WriteBinary`, `origin.go` (allowlist + CORS `Route`), `QueuedConn`/`Hub`, `transporttest` run against `ws` |
| `Composite` | best-effort fan-out; `Close` skips closed children because shutdown closes twice |
| `sse` | clears its own read/write deadlines (the server's 10s timeouts would kill streams); connection ID before `Connect` |
| `graphqlws` | subprotocol close codes 4400/4401/4406/4408/4409/4429 |
| `webrtc` | peer cap, data-channel open timeout, `OnConnectionStateChange` backstop |
| config | `INDRI_TRANSPORTS` (default `ws`) and WebRTC ICE/NAT/port/peer-cap settings; unknown or duplicate names fail boot |

Findings worth keeping:

- melody's `Session.Close()` only queues a close frame. The contract callers rely on is "`IsClosed()`
  is true by the time `Disconnect` fires" (`melody.go`: `session.close()` runs before
  `disconnectHandler`), not "immediately after `Close()`".
- WebRTC setup took about 2s on a host that offers many interfaces (VM bridges, IPv6), and over 15s
  under `-race`. The tests keep the client peer on loopback IPv4, which brings setup to milliseconds.
  The production `OpenTimeout` is 15s to leave headroom.

## Client (`client/packages/protocol-client`, done)

- `TransportClient` interface (`connect/send/close/onOpen/onMessage/onClose/onError`), carrying
  `string | Uint8Array`. A shared base class owns the lifecycle: messages that arrive while
  `connect()` is still resolving are delivered, not dropped, and a `close()` from our side never
  fires `onClose`.
- Adapters for `ws`, `sse`, `graphqlws`, and `webrtc`. `fetch`, `WebSocket`, and `RTCPeerConnection`
  are injected so the package has no Expo/RN dependency: RN's global `fetch` can't stream, and native
  WebRTC needs `react-native-webrtc` in a dev client. Base64 and UTF-8 stream decoding are
  implemented in the package because Hermes doesn't reliably provide `atob`/`TextDecoder`.
- `MessageHandler` takes a `TransportClient` instead of a URL, and its public API is unchanged.
  `app/index.tsx` picks the transport from `EXPO_PUBLIC_TRANSPORT` (default `ws`), and
  `EXPO_PUBLIC_API_URL` is that transport's endpoint (`ws://…/ws`, `ws://…/graphql`, or an `http://`
  base for `sse`/`webrtc`). One variable, not the separate HTTP URL first planned. On native the
  app passes `expo/fetch`.
- The lockfile change is only the workspace link, applied by hand. pnpm 10 strips `libc:` fields
  and pnpm 11.28 adds `supports-color` peer suffixes throughout, so neither reproduces the committed
  lockfile. `pnpm@11 install --frozen-lockfile`, which is what CI runs, accepts it.

## Verification

- Go: `go test -race ./...`; every transport passes `transporttest`.
- Client: typecheck, 236 node tests, and lint (0 errors). The CI steps also passed in a clean copy
  with pnpm 11 and `--frozen-lockfile`; `expo export --platform web` builds.
- Live, against one server running all four transports on a scratch MongoDB database: a Node script
  using the package ran register → login → inquire over `ws`, `sse`, and `graphqlws` concurrently.
  In the browser (Expo web, Chromium), login → game list → create game → keyframe passed on each
  of the four transports. The root `config.json` has no board layout, so moves were not exercised.
- The browser caught two bugs the unit tests couldn't:
  - Adapters called `fetch` as a method, which browsers reject ("Illegal invocation"). Fixed with
    `resolveFetch`, and the fakes now enforce the same check.
  - A pre-existing crash: the Join screen with zero games (`"games": null`).
- Native React Native was not verified for any transport.

## Out of scope

- Session resume on reconnect: the persisted `sessionId` is written but never read.
- TURN hosting.
- Verifying native (non-web) RN WebRTC.

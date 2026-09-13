---
status: in_progress
approved_at: "2026-09-12T01:27:30.641Z"
updated: "2026-09-12T01:27:30.641Z"
---
# Plan: WebRTC DataChannel transport

**Created:** 2026-09-11 | **Status:** Draft | **Effort:** XL | **Branch:** `POC-00002/webrtc-transport`

## Summary

Add WebRTC as a fourth `transport.Transport` alongside WebSocket, GraphQL and REST+SSE, using a
DataChannel for bidirectional JSON messaging. The server becomes a pion peer holding one
`PeerConnection` per client. Signalling is a transport-owned HTTP route, so WebRTC stands alone —
a client can register and log in over the DataChannel without any other transport being open.

Audio/video is **not** built here. The plan buys the cheap insurance that keeps it open (an explicit
`MediaEngine`, session-keyed identity, serialised renegotiation) and spends one step settling the one
question that would otherwise be discovered too late: whether a DataChannel-only `PeerConnection` can
carry a track added after connect.

## Architecture Context

- `transport.Transport` is the seam. A WebRTC DataChannel is **bidirectional and message-oriented**, so
  unlike `sse` it implements `Handle` in full and is shaped like `ws`, not like the push-only transports.
- **Inbound needs no new plumbing.** `boot.registerHandlers` calls `Handle` on the `Multi`, and
  `boot.handleClientMessage` (`internal/services/boot/handlers.go:101`) is transport-agnostic: resolve
  session from the conn's `sessionId` key → `router.DispatchMessage` → write `Responses` → `SetKey` on
  auth. Pipe `DataChannel.OnMessage` into `Handlers.Message` and login/kick/logout all work unchanged.
  `entrypoints.HandleConnect`/`HandleDisconnect` take `transport.Conn` and work the same way.
- **Outbound reuses the shared push machinery** added in `ec85e94`: `transport.Registry` (conn set +
  fan-out), `transport.Keys` (per-conn state, `WhileOpen`/`MarkClosed`), `transport.BufferedConn[[]byte]`
  (bounded queue, drop-oldest). These already back `sse` and the GraphQL subscription.
- **The only genuinely new thing is signalling**, plus per-peer lifecycle.
- `transport.OriginPolicy.Middleware` wraps the whole mux in `internal/entrypoints/http/server.go`, so
  the signalling route inherits the allowlist and CORS for free. It does **not** cover the UDP path —
  that is protected by DTLS plus the SDP fingerprint.
- Key files: `internal/transport/{transport,registry,keys,bufferedconn,origin}.go`,
  `internal/transport/sse/sse.go` (closest prior art for a handshake-authenticated transport),
  `internal/injector/services.go:25-45` (transport aggregation + `SetPeer`),
  `client/services/message-handler.ts:34,78,133` (WebSocket hardcoded — the client seam to extract).

ADR: **signalling lives in the transport, not in the action registry** — action handlers are
connection-independent and only receive `req.Session`, which would force every peer to be keyed by
session and require logging in over another transport first. Rejected signalling-as-an-action, accepting
a fourth inbound surface as the price of WebRTC standing alone.

ADR: **non-trickle ICE on both sides** — one POST returns one complete SDP answer, no push channel, no
second endpoint. pion's docs discourage this generally, but the warning targets peers that must STUN or
TURN themselves; a server with `SetNAT1To1IPs` and a single-port mux gathers host candidates with no
network round trip. Rejected trickle, which would make WebRTC depend on WebSocket or SSE already
being connected.

## Research Findings

- **pion/webrtc/v4** is current; module path `github.com/pion/webrtc/v4`. *Unverified:* the precise v3→v4
  breaking-change list was not confirmed — read the v4.0.0 release notes before writing code.
- **`DataChannel.Send` has no documented goroutine-safety guarantee.** Serialise writes per channel with
  one owning goroutine. This is exactly what `BufferedConn` + a drain goroutine gives us.
- Backpressure is `BufferedAmount()` + `SetBufferedAmountLowThreshold` + `OnBufferedAmountLow`. pion's
  `data-channels-flow-control` example gives each channel its own queue; **never share one send loop
  across peers**, or one slow client stalls everyone.
- **Only `MediaEngine` codec registration forecloses media later.** Transceivers need no pre-creation —
  `AddTrack` after connect creates one and triggers renegotiation. `RegisterDefaultCodecs` is *not*
  safe for concurrent use; call it once at construction.
- **Same-PeerConnection data→media renegotiation is contested.** pion ships
  `play-from-disk-renegotiation` and renegotiation tests, but issues #1073/#2774 report `OnTrack` not
  firing when upgrading a data-only connection. Step 8 settles it for our configuration.
- Serialise renegotiations: overlapping `AddTrack` calls produce `"Failed to process the bundled
  m= section"` (issue #1169).
- **`GatheringCompletePromise` leaks** a blocked goroutine if the PeerConnection closes mid-gather
  (issue #2507). Select against a cancellation channel, never `<-gatherComplete` bare.
- **Selecting against a context is necessary but NOT sufficient** (found while building Step 4, v4.2.20).
  pion deliberately fires the gather-complete handler when the PeerConnection closes, so the channel
  firing does *not* mean gathering succeeded — a peer closed mid-gather otherwise returns a truncated
  SDP with a nil error. Check `pc.SignalingState() == SignalingStateClosed` after the select and turn
  that into an error.
- `SetNAT1To1IPs` is deprecated in v4.2.20 in favour of `SetICEAddressRewriteRules`. Still used, since
  the change was out of scope for Step 4; revisit before this ships.
- `NewMultiUDPMuxFromPort` **excludes loopback by default** — in-process tests need
  `UDPMuxFromPortWithLoopback()` or they gather no reachable candidates.
- The mux binds one socket **per interface**, so port 0 yields a different ephemeral port on each. Any
  test asserting that peers share *one* port must reserve a concrete free port first, or it passes or
  fails depending on how many interfaces are up.
- Single-port UDP mux (`ice.NewMultiUDPMuxFromPort` + `SetICEUDPMux`) is the container-friendly choice.
  `SetEphemeralUDPPortRange` does **not** constrain server-reflexive candidate ports, so it cannot
  actually lock down exposure. The mux must exist before any PeerConnection uses it.
- `OnConnectionStateChange` spawns a goroutine per invocation; `OnICEConnectionStateChange` runs
  synchronously. No ordering guarantee between them. Guard teardown with `sync.Once` (issue #744).
- `Disconnected` is transient (~30s default) and can recover. Tear down on `Failed`/`Closed` only.
- DataChannel default (reliable, ordered) matches Indri's delta contract exactly — no downgrade.
- Practical cross-browser message ceiling is **16 KiB**, not 256 KiB. Keyframes can exceed this and must
  be chunked. Read `RTCSctpTransport.maxMessageSize` at runtime.
- **Client:** `react-native-webrtc` + `@config-plugins/react-native-webrtc` + `expo prebuild` + a custom
  dev client. **Expo Go can never work.** `react-native-webrtc-web-shim` covers web.
  *Unverified:* New Architecture compatibility on Expo SDK 53 / RN 0.79 / React 19 (New Arch is on by
  default). Step 9 is a spike precisely because of this.
- **Media later needs an SFU, not mesh** — the mesh cutoff is 3–4 participants and the target is 4–8.
  LiveKit (Go, pion-based, Apache-2.0) is the credible option; ion-sfu is stagnant. Out of scope here.
- TURN is needed **even for DataChannel-only**, since NAT traversal applies regardless of media.

## Security Considerations

- **Anonymous signalling is a resource-exhaustion vector.** Each POST allocates a PeerConnection, an ICE
  agent and a mux registration. Unauthenticated callers can do this in a loop. Mitigate with a pending-peer
  TTL (peer never reaching `connected` is closed and dropped), a global peer cap returning 503, and
  teardown on `Failed`/`Closed`. Non-negotiable — see Step 6.
- SDP is attacker-controlled input parsed by pion. Cap the request body, and keep pion current.
- A peer id is a capability: renegotiation for a peer must be accepted only over that peer's own
  DataChannel, never by peer id from an unauthenticated HTTP caller.
- TURN credentials are secrets; they belong in env config, never in a client bundle. Use ephemeral
  time-limited TURN credentials when TURN is introduced.
- Authenticated signalling (bearer token present) binds `SessionIDKey` at handshake, matching SSE.
  Anonymous peers carry no session key and are unreachable by targeted broadcast until `login` binds one
  through the existing dispatch path.

## Performance Considerations

- A PeerConnection is far heavier than a WebSocket: ICE agent, DTLS, SCTP, plus goroutines per peer.
  Budget an order of magnitude more memory per client than `ws`.
- Non-trickle gathering blocks the signalling response. Server-side this is host-candidate enumeration
  (microseconds) **only if `SetNAT1To1IPs` is correct**; misconfigured, it stalls on STUN timeouts. Bound
  the wait and fail loudly rather than hanging the request.
- Two-layer backpressure: `BufferedConn` drops oldest at 16 queued (client recovers via `refresh`, same
  contract as SSE), and the drain goroutine parks on `BufferedAmount` so we never pile into SCTP.
- Single UDP port for all peers — no ephemeral port exhaustion, one `docker-compose` mapping.
- Multi-instance: peers are process-pinned, exactly like melody sessions today. Redis fan-out reaches the
  owning instance, whose `Registry` holds the conn. No new problem, but a UDP-aware load balancer needs
  session affinity on the mux port.

## Open Questions

### Critical (P1 - Blockers)
1. **RESOLVED — yes**, for this configuration. `internal/transport/webrtc/renegotiate.go` implements
   `AddTrack` → `CreateOffer` → `SetLocalDescription` → offer over the `signal` DataChannel → await the
   answer → `SetRemoteDescription`, serialised per peer with a mutex (pion issue #1169). An in-process
   pion client (`internal/transport/webrtc/renegotiation_test.go`,
   `TestRenegotiate_TrackAddedAfterDataChannelOnly`) added an Opus `TrackLocalStaticSample` to an
   already-connected, DataChannel-only server `PeerConnection` and observed `OnTrack` fire on the client
   on every one of 10 repeated runs under `-race` (pion/webrtc v4.2.20, the API built in `peer.go`'s
   `newAPI` — a custom `MediaEngine`/`SettingEngine` with default interceptors auto-registered because
   none were supplied explicitly). Issues #1073/#2774 did not reproduce under this configuration.
   **Consequence:** phase-two video may reuse this same PeerConnection; this question alone does not
   force a second PeerConnection or an SFU. (LiveKit may still be the right call once mesh sizing at
   4–8 participants is considered — see Research Findings — but that is a separate, scaling-driven
   decision, not a renegotiation-capability one.)
2. Does `react-native-webrtc` work under Expo SDK 53 New Architecture with React 19? — **Step 9 answers
   this.** If not: disable New Arch in `app.json`, or defer the client half and keep Steps 1–8.

### Important (P2 - Affects implementation)
3. Chunking for keyframes over 16 KiB — needed before a real game ships, deferred to Step 7's assertion
   that oversized payloads are detected and logged rather than silently truncated.

### Unresolved / waiting on signal
- ~~Whether phase-two media reuses this PeerConnection or adds a second~~ — **resolved by Step 8: yes,
  it can reuse this connection** (see Q1 above). Steps 1–7 needed no change either way. Revisit only if
  mesh sizing at 4–8 participants pushes phase two toward an SFU (LiveKit) for reasons unrelated to
  renegotiation capability.

## Steps

### Step 1: Anonymous connections carry no session key
- **Test:** `internal/transport/keys_test.go` — `NewKeys("")` leaves `SessionIDKey` absent; `NewKeys("x")`
  sets it (existing behaviour unchanged, so SSE and GraphQL are unaffected).
- **Implement:** `internal/transport/keys.go`
- **Code:**
  ```go
  func NewKeys(sessionID string) *Keys {
      keys := make(map[string]any, 1)
      // An anonymous connection (WebRTC before login) carries no session key and
      // is invisible to targeted broadcast until a login binds one.
      if sessionID != "" {
          keys[SessionIDKey] = sessionID
      }
      return &Keys{keys: keys}
  }
  ```
- **Validation:** `go test -race ./internal/transport/`

### Step 2: Signal envelope decoding
- **Test:** `internal/transport/webrtc/signal_test.go` — a valid offer decodes; missing `sdp`, wrong
  `type`, oversized body and malformed JSON are each rejected with a distinct error and never reach pion.
- **Implement:** `internal/transport/webrtc/signal.go`
- **Code:**
  ```go
  // Signal is one step of an SDP exchange. Type is "offer" or "answer"; the
  // renegotiation path (Step 8) reuses the same envelope over the signal channel.
  type Signal struct {
      Type   string `json:"type"`
      SDP    string `json:"sdp"`
      PeerID string `json:"peerId,omitempty"` // set on renegotiation only
  }

  func DecodeSignal(r io.Reader) (*Signal, error)
  ```
- **Constraint:** cap the body at 256 KiB before decoding — SDP is attacker-controlled.
- **Validation:** `go test -race ./internal/transport/webrtc/`

### Step 3: DataChannel-backed connection
- **Test:** `internal/transport/webrtc/conn_test.go` — against a fake sink: writes arrive in order; the
  17th queued message drops rather than blocking; `Close` stops the drain goroutine (assert with
  `runtime.NumGoroutine` or a done channel); double `Close` is safe; a sink reporting high
  `BufferedAmount` parks the drain rather than piling in.
- **Implement:** `internal/transport/webrtc/conn.go`
- **Code:**
  ```go
  // dataSink is the half of webrtc.DataChannel this conn uses, so tests need no
  // real PeerConnection.
  type dataSink interface {
      Send([]byte) error
      BufferedAmount() uint64
      OnBufferedAmountLow(func())
      SetBufferedAmountLowThreshold(uint64)
  }

  type rtcConn struct {
      *transport.BufferedConn[[]byte]
      sink dataSink
      done chan struct{}
  }

  // drain owns every write to the channel: pion documents no goroutine-safety
  // guarantee for Send, and one owning goroutine also gives us a place to honour
  // BufferedAmount so a slow peer backs up in its own queue, not in SCTP.
  func (c *rtcConn) drain() { /* select on Events(), done; park on low-water signal */ }
  ```
- **Constraint:** drop-oldest at 16 queued matches SSE; the client recovers with `refresh`.
- **Validation:** `go test -race ./internal/transport/webrtc/`

### Step 4: Peer factory and the non-trickle answer
- **Test:** `internal/transport/webrtc/peer_test.go` — pion drives both sides in-process (`signalPair`
  style, SDP handed across as Go values). A client offer produces an answer whose SDP already contains
  candidates; the DataChannel opens; messages flow both ways. A PeerConnection closed mid-gather returns
  an error instead of hanging.
- **Implement:** `internal/transport/webrtc/peer.go`
- **Code:**
  ```go
  // newAPI is built once per transport. RegisterDefaultCodecs is not safe for
  // concurrent use, and codec registration is the ONE thing that cannot be added
  // after a PeerConnection exists -- registering now is what keeps audio/video
  // open later at zero cost today.
  func newAPI(cfg Config) (*webrtc.API, error) {
      m := &webrtc.MediaEngine{}
      if err := m.RegisterDefaultCodecs(); err != nil { return nil, err }

      se := webrtc.SettingEngine{}
      mux, err := ice.NewMultiUDPMuxFromPort(cfg.UDPPort)
      if err != nil { return nil, err }
      se.SetICEUDPMux(mux)
      if len(cfg.NAT1To1IPs) > 0 { se.SetNAT1To1IPs(cfg.NAT1To1IPs, webrtc.ICECandidateTypeHost) }

      return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se)), nil
  }

  // answer waits for gathering, but never bare: issue #2507 leaks a goroutine if
  // the PeerConnection closes first.
  select {
  case <-webrtc.GatheringCompletePromise(pc):
  case <-ctx.Done():
      return nil, fmt.Errorf("ice gathering did not complete: %w", ctx.Err())
  }
  ```
- **Constraint:** bound the gather wait with a context — misconfigured `SetNAT1To1IPs` otherwise hangs
  the signalling request on STUN timeouts.
- **Depends on:** Step 2
- **Validation:** `go test -race ./internal/transport/webrtc/`

### Step 5: Peer lifecycle and teardown
- **Test:** `internal/transport/webrtc/peer_test.go` — `Failed` removes the peer from the registry and
  closes the conn exactly once; `Disconnected` alone does **not** tear down; concurrent state callbacks
  cannot double-close.
- **Implement:** `internal/transport/webrtc/peer.go`
- **Code:**
  ```go
  // OnConnectionStateChange fires on a fresh goroutine per invocation and has no
  // ordering guarantee against the ICE callback, so teardown runs under a Once.
  pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
      switch s {
      case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
          p.teardown.Do(p.close)
      case webrtc.PeerConnectionStateDisconnected:
          // Transient, recovers within pion's ~30s window. Not a teardown.
      }
  })
  ```
- **Constraint:** a peer left in the `Registry` after failure is a leak — the outbound mirror of the
  repo's "a write that never publishes is invisible" rule.
- **Depends on:** Step 4
- **Validation:** `go test -race ./internal/transport/webrtc/`

### Step 6: The transport and its signalling route
- **Test:** `internal/transport/webrtc/webrtc_test.go` — `POST /rtc/offer` with a pion-generated offer
  returns an answer and registers a conn; a bearer token binds `SessionIDKey` so `BroadcastFilter`
  reaches it; no token yields an anonymous conn that a `login` over the DataChannel then binds; a peer
  that never connects is dropped after the TTL; the global cap returns 503; `Disconnect` closes the peer.
- **Implement:** `internal/transport/webrtc/webrtc.go`
- **Code:**
  ```go
  type Transport struct {
      *transport.Registry           // conn set + fan-out, shared with sse/graphql
      sessions SessionLookup
      api      *webrtc.API
      mu       sync.Mutex
      peers    map[string]*peer     // keyed by server-generated peer id, not session
  }

  func (t *Transport) Register(mux *http.ServeMux) {
      mux.HandleFunc("POST "+signalPath, t.offer)
  }

  // Two channels by label: game traffic is dispatched like any other transport's
  // messages; "signal" is renegotiation and never reaches the action router.
  pc.OnDataChannel(func(dc *webrtc.DataChannel) {
      switch dc.Label() {
      case gameChannel:   t.attach(p, dc)
      case signalChannel: t.attachSignalling(p, dc)
      }
  })
  ```
- **Constraint:** anonymous signalling is a DoS vector — pending-peer TTL **and** a global cap are
  required, not optional.
- **Depends on:** Steps 3, 5
- **Validation:** `go test -race ./internal/transport/webrtc/`

### Step 7: Wiring, config and the message-size guard
- **Test:** `internal/services/boot/handlers_test.go` — the aggregate reaches a WebRTC peer;
  `internal/transport/webrtc/conn_test.go` — a payload over 16 KiB is logged and reported, not silently
  truncated.
- **Implement:** `internal/injector/services.go`, `internal/repo/env/env.go`, `docker-compose.yml`,
  `.env.example`
- **Code:**
  ```go
  rtc, err := webrtcTransport.New(repos.SessionRepo, webrtcTransport.ConfigFromEnv())
  if err != nil { return nil, err }
  multi := transport.NewMulti(clients.Transport, gql, events, api, rtc)
  for _, t := range []interface{ SetPeer(transport.Transport) }{gql, events, api, rtc} { t.SetPeer(multi) }
  ```
  New env: `INDRI_RTC_UDP_PORT` (default 8443), `INDRI_RTC_NAT_1TO1_IPS`, `INDRI_RTC_ICE_SERVERS`,
  `INDRI_RTC_MAX_PEERS`, `INDRI_RTC_PENDING_TTL`.
- **Constraint:** `docker-compose.yml` must publish the UDP port; without it ICE fails with no error at
  the HTTP layer.
- **Depends on:** Step 6
- **Validation:** `go vet ./... && go test -race ./...`

### Step 8: Spike — can this PeerConnection carry a track added later?
- **Test:** `internal/transport/webrtc/renegotiation_test.go` — after the DataChannel is open, the server
  calls `AddTrack`, drives the offer/answer over the `signal` channel, and the in-process pion client
  asserts `OnTrack` fires. A second `AddTrack` issued while the first negotiation is in flight must be
  serialised, not interleaved (issue #1169).
- **Implement:** `internal/transport/webrtc/renegotiate.go` — serialised renegotiation over the signal
  channel. **No media features**: no capture, no routing, no SFU.
- **Code:**
  ```go
  // One renegotiation at a time per peer: overlapping AddTrack calls produce
  // "Failed to process the bundled m= section" (pion #1169).
  func (p *peer) renegotiate(ctx context.Context) error {
      p.negotiating.Lock()
      defer p.negotiating.Unlock()
      // CreateOffer -> SetLocalDescription -> send over signalChannel -> await answer
  }
  ```
- **Constraint:** this step exists to answer P1 Q1. If `OnTrack` does not fire, **record the result and
  stop** — do not attempt a fix. The finding redirects phase two to a second PeerConnection or LiveKit,
  and Steps 1–7 remain correct either way.
- **Depends on:** Step 6
- **Validation:** `go test -race -run Renegotiation ./internal/transport/webrtc/`

### Step 9: Spike — react-native-webrtc under Expo SDK 53 New Architecture
- **Test:** manual. A custom dev client builds and a DataChannel opens against the Step 7 server.
- **Implement:** `client/app.json` (add `@config-plugins/react-native-webrtc`), `npx expo prebuild`,
  `eas build --profile development`.
- **Visual — requires human verification:** app launches on a real iOS device, a real Android device and
  web; DataChannel reaches `open` on each.
- **Constraint:** New Architecture is **on by default** in SDK 53 and compatibility is unverified. If it
  fails, try `"newArchEnabled": false`; if it still fails, **stop and report** — Steps 10–11 are blocked
  and Steps 1–8 ship without a client.
- **Depends on:** Step 7
- **Validation:** `pnpm run typecheck`, plus the manual checks above.

### Step 10: Extract the client transport seam
- **Test:** `client/services/transport.node-test.ts` — `MessageHandler` drives a fake transport: `send`
  before `open` is dropped with a warning, inbound messages reach the existing parser chain unchanged.
- **Implement:** `client/services/transport.ts`, `client/services/message-handler.ts`
- **Code:**
  ```ts
  // Mirrors the server's transport.Transport split so MessageHandler stops
  // hardcoding WebSocket (message-handler.ts:34,78,133).
  export interface ClientTransport {
      connect(url: string): void
      send(message: object): void
      onMessage(handler: (data: string) => void): void
      close(): void
  }
  ```
- **Constraint:** behaviour-preserving. `game-state-parser.ts` and the `parsers` array must not change.
- **Depends on:** Step 9
- **Validation:** `cd client && pnpm run typecheck && pnpm test`

### Step 11: WebRTC client transport
- **Test:** `client/services/webrtc-transport.node-test.ts` — pure parts only (offer/answer envelope
  building, message framing). The PeerConnection itself is exercised by the Step 9 manual check, since
  the client has no WebRTC-capable test runner.
- **Implement:** `client/services/webrtc-transport.ts`; add `react-native-webrtc`,
  `@config-plugins/react-native-webrtc`, `react-native-webrtc-web-shim`
- **Code:**
  ```ts
  // Non-trickle: gather fully, then POST one offer and apply the one answer.
  const pc = new RTCPeerConnection({iceServers})
  const game = pc.createDataChannel('game')       // reliable, ordered — matches the delta contract
  const signal = pc.createDataChannel('signal')
  await pc.setLocalDescription(await pc.createOffer())
  await iceGatheringComplete(pc)                   // wait for gathering, then one round trip
  const answer = await post('/rtc/offer', {type: 'offer', sdp: pc.localDescription!.sdp})
  await pc.setRemoteDescription(answer)
  ```
- **Constraint:** after touching client imports, verify the way CI does —
  `rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test`.
- **Depends on:** Step 10
- **Validation:** `cd client && pnpm run typecheck && pnpm test`

## Acceptance Criteria

- [ ] A pion client connects over `POST /rtc/offer` and exchanges game messages on the DataChannel
- [ ] `register`, `login`, `create`, `join` and `kick` work over WebRTC with no new action code
- [ ] A delta broadcast reaches WebRTC, WebSocket, GraphQL and SSE clients in the same game identically
- [ ] An authenticated signalling request binds `SessionIDKey` at handshake; an anonymous one binds on login
- [ ] A peer that never connects is dropped after the TTL; the global cap returns 503
- [ ] `Failed` tears the peer down exactly once and removes it from the `Registry`; `Disconnected` does not
- [ ] Step 8 produces a recorded yes/no on post-connect `AddTrack`, written into this plan
- [ ] Step 9 produces a recorded yes/no on Expo New Architecture compatibility
- [ ] `go vet ./...` and `go test -race ./...` clean
- [ ] No audio/video feature code ships

## Checklist (non-TDD cleanup)

- [ ] `docs/PROTOCOL.md` gains a WebRTC section; the transport table at the top gains a fourth row
- [ ] `docs/ARCHITECTURE.md` adapter table and request-lifecycle diagram updated
- [ ] `README.md` transport table updated
- [ ] `CLAUDE.md` transports section notes that WebRTC is bidirectional and needs no new inbound plumbing
- [ ] `.env.example` documents every new `INDRI_RTC_*` variable
- [ ] `docker-compose.yml` publishes the UDP port
- [ ] TURN is documented as required in production even without media

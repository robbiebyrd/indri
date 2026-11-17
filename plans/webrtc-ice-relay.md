# WebRTC connectivity and future media: STUN / TURN / SFU options

Status: **proposal — options for Boss to choose from.** Nothing here is built.
Scope: the existing WebRTC *data channel* transport, plus future **webcam/mic** support.

## Where we are

- The Indri server is itself the WebRTC peer for the data-channel transport (pion,
  `internal/transport/webrtc`): client ↔ server, never client ↔ client.
- Server ICE config is a flat URL list (`INDRI_WEBRTC_ICE_SERVERS` → `pion.ICEServer{URLs: …}`,
  `webrtc.go:182`). It has no `Username`/`Credential`, so it can't authenticate to a TURN server.
- The client adapter accepts `iceServers: {urls}[]` (`protocol-client/src/adapters/webrtc.ts:38`),
  also with no credentials. `app/index.tsx` passes none.
- Direct-path knobs already exist: `WEBRTC_NAT_1TO1_IPS` and `WEBRTC_UDP_PORT_MIN/MAX`.

## Why media changes the answer

| | Data channel (today) | Webcam/mic (future) |
|---|---|---|
| Peers | client ↔ Indri server (public address) | client ↔ client (mesh) or client ↔ media server (SFU) |
| Relay needed when | the client's network blocks UDP or allows only port 443 | also whenever two players' NATs can't reach each other (mesh) |
| Relay traffic | small game messages | audio/video: orders of magnitude more bandwidth |
| Fallback if WebRTC fails | `ws` / `sse` / `graphqlws` | none: media has no WebSocket fallback |

The media column drives four consequences:

1. **TURN goes from nice-to-have to required.** Media can't fall back to another transport.
2. **Relay bandwidth becomes media-sized.** That rules out embedding TURN in the game server: relay load
   would compete with game traffic, and scaling one would mean scaling the other.
3. **"Only relay to our own server" no longer works as the TURN permission rule once peers are other
   clients.** That rule was the clean abuse guard for the data-channel-only case.
4. **Topology matters more than TURN.**
   - Mesh (every player sends to every other player) works only for small groups. Commonly cited limits
     are about 4–6 video participants, because each client uploads N−1 streams.
   - An SFU (every player uploads once, and the server forwards) is the usual answer past that.
   - Party games can easily exceed mesh limits, and teams suggest per-team rooms.

   Sources: [Ant Media, topology guide](https://antmedia.io/webrtc-network-topology/);
   [Forasoft, P2P vs MCU vs SFU](https://www.forasoft.com/blog/article/p2p-vs-mcu-vs-sfu-for-video-conference-app-805).

## Options

### A. Data channel only: make the direct path robust (do now; independent of media)

Use pion's own muxes on the Indri server:
- `SetICEUDPMux` (all ICE over one UDP port);
- `SetICETCPMux` plus TCP network types (passive ICE-TCP on one TCP port), in `pion/webrtc/v4`
  `settingengine.go:490-499`.

This covers UDP-blocked clients whose network allows that TCP port, and makes firewall and Docker setup
one UDP port plus one TCP port. It needs no TURN, no credentials and no client change. Clients whose
network allows only 443 fall back to `ws`/`sse`. ICE-TCP support in `react-native-webrtc` is unverified.

### B. Media via mesh plus a standalone TURN (coturn)

- Indri relays SDP offers and answers between players over the existing transports, as new actions.
  Browsers do the media.
- A separate coturn instance authenticates with short-lived HMAC credentials that Indri mints
  (the "TURN REST" scheme; coturn `use-auth-secret`, pion/turn `LongTermTURNRESTAuthHandler`).
- **Pros:** no media server; cheapest when groups are small.
- **Cons:**
  - Breaks down beyond a handful of video participants.
  - Every relayed pair costs TURN bandwidth.
  - We own the signaling code, including glare and renegotiation.
  - Coturn is another service to run and secure.

### C. Media via a self-hosted SFU: LiveKit (recommended when media lands)

- LiveKit server is Apache-2.0 and written in Go on pion. It includes an embedded TURN server with
  TURN/TLS and TURN/UDP on 443, and authenticates clients with JWT access tokens signed with an API key
  and secret. Sources: [LiveKit GitHub](https://github.com/livekit/livekit);
  [LiveKit self-hosting docs](https://docs.livekit.io/home/self-hosting/deployment/).
- Indri's only job is authorization. A new `media` action mints a LiveKit token for the caller.
  - The caller's identity comes from *their own connection's* `sessionId`, per the repo's identity
    rule.
  - The room is the game or the team (for example `game:<id>` or `team:<id>:<team>`).
  - Grants decide who can publish and who can subscribe.
- **Pros:**
  - Scales past mesh limits, with simulcast.
  - Its TURN covers media, so we run no coturn of our own.
  - Mature client SDKs (web, React Native).
  - The same token code works against LiveKit Cloud (option D).
- **Cons:** another service with UDP port requirements, and new client SDK dependencies.

### D. Media via a managed SFU (LiveKit Cloud, Cloudflare Realtime, Daily, …)

This is C without the operations work. Indri still mints the tokens.
- **Cons:** per-minute or per-GB cost (pricing not researched here, so verify before choosing) and a
  vendor dependency. With LiveKit Cloud, moving between C and D is configuration only.

### E. Build our own SFU inside Indri on pion (not recommended)

pion can forward RTP tracks, but a production SFU has a lot of work that LiveKit already has:
- simulcast and layer selection;
- bandwidth estimation and congestion control;
- NACK/PLI and keyframe requests;
- recording and scaling.

That is a large, specialized scope, and it would put media load on the game server.

## Recommendation

1. **Now:** option A for the data channel. Build no TURN infrastructure yet (YAGNI). Blocked clients
   still have `ws`/`sse`.
2. **When webcam/mic is scheduled:** option C, LiveKit as a separate media plane, with Indri minting
   game- or team-scoped tokens. Pick self-hosted or managed (D) then; the Indri code is the same.
   - Its embedded TURN covers media. If blocked-network data-channel users matter by then, the same
     LiveKit deployment or a coturn beside it can serve them, through a credentialed `iceServers`
     provider built at that point.
3. **Rejected:**
   - Embedded TURN inside Indri: it couples relay bandwidth to the game servers.
   - A home-grown SFU (E).
   - Mesh (B), unless the product commits to at most about 4 video participants per room.

## Tests when built (TDD)

- **A:** a pion client restricted to TCP candidates connects; UDP-mux and TCP-mux settings via env,
  `server.json` and flags (`internal/cli` drift guard).
- **C:**
  - Token minting is pure: grants are derived from the session, and a caller can't request another
    player's identity or a room outside their game.
  - An integration test against the `livekit-server --dev` container verifies that a minted token joins
    the right room and is refused for others.

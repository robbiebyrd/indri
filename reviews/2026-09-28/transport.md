# Transport slice review: internal/transport/**, internal/entrypoints/**, injector/transport.go, broadcast's use of the transport

Reviewed at worktree `wip/codebase-review` (based on main 3eab208). Read-only.

Tool results (run with `GOCACHE=$TMPDIR/gocache`, because the default Go build cache is outside the sandbox):
- `go vet ./internal/transport/... ./internal/entrypoints/... ./internal/services/broadcast/...`: clean (exit 0).
- `go test -race -count=1 ./internal/transport/... ./internal/entrypoints/... ./internal/services/broadcast/...`: all pass.
  `transporttest` and `entrypoints` have **no test files**.

Dependency code was checked in the module cache where it matters: pion/webrtc v4.2.22, pion/sctp v1.11.3,
olahol/melody v1.3.0, gorilla/websocket v1.5.3, and Go's net/http.

## Prior findings re-check

| Prior | Finding | Status | Evidence |
|---|---|---|---|
| Review A #7 | WebRTC OnMessage can run before OnOpen's Hub.Add/Connected | **STILL-OPEN** (see TR-1) | `webrtc.go:289-304` still registers them independently. In pion, `DataChannel.onOpen` runs the handler with `go` (`pion/webrtc/v4@v4.2.22/datachannel.go:243`), and `handleOpen` then starts `go d.readLoop()` separately (`:377`). Nothing orders the two. |
| Review A #10 | Origin check allows any Origin whose host equals `Host` (DNS rebinding, scheme ignored) | **STILL-OPEN, needs a decision** (see TR-6) | `origin.go:36-38` still does `strings.EqualFold(u.Host, r.Host)`. `docs/PROTOCOL.md:10-14` documents it as intended. |
| Review B #10 / "7-item" #7 | A slow client's full buffer drops frames and the connection stays open | **STILL-OPEN / DEFERRED** (see TR-2) | `hub.go:98-108` returns `ErrBufferFull`. ws: melody `session.go` `writeMessage` drops on a full buffer and `Session.Write` still returns nil. The fix is scheduled in `plans/session-resume.md` §"Slow clients". |
| Review B cleanup | One MessagePack encode per recipient in `broadcastToSessions` | **STILL-OPEN, moved** (see TR-12) | Now in `broadcast.go:257` (`deliverLocal` → `transport.WriteEncoded` per conn). |
| Review 0 (6760eda) | melody MaxMessageSize read the wrong env var; `:=` shadow | **FIXED** | `ws.go:49` uses `WSMaxMessageSizeBytes`, and `m.Config` is assigned, not shadowed. |
| Review 0 (56d1a6f) | No Origin allowlist (CSWSH) | **FIXED, then CHANGED** | Every transport is guarded now: `ws.go:53` and `graphqlws.go:73` through CheckOrigin, and sse/webrtc through `Route`. The same-host rule later added the A #10 concern. |
| session-resume plan bugs 1 and 3 (not a review, in my slice) | reconnect never calls ConnectPlayer; a stale connection's disconnect marks a resumed player offline | **STILL-OPEN, tracked** (see TR-13) | `ConnectPlayer` still has no non-test caller. `entrypoints/websocket.go:57` still runs `DisconnectPlayer` unconditionally. |

---

### [P2] WebRTC can deliver client messages before the conn is in the hub and before Connect fires
- ID: TR-1
- Category: concurrency
- Location: internal/transport/webrtc/webrtc.go:289-304; also webrtc.go:345-365 (close vs a late OnOpen)
- Confidence: CONFIRMED (ordering). The hub-leak variant is PLAUSIBLE.
- Prior: Review A #7
- What: pion runs the OnOpen handler on its own goroutine (`go d.openHandlerOnce.Do(...)`, datachannel.go:243) and starts the read loop on another (`go d.readLoop()`, :377). `OnMessage` → `conn.Deliver` can therefore run before `Hub.Add` and `conn.Connected`. `QueuedConn.Deliver` (hub.go:171) does not wait for Connect.
  A second race: if `p.close()` runs (from dc.OnClose or a Failed/Closed state change) before the OnOpen goroutine is scheduled, OnOpen still calls `Hub.Add(p.id, conn)` on the already-closed conn (webrtc.go:292). `closeOnce` has already fired, so nothing ever removes it.
- Failure scenario: (1) The client sends `login` in `dc.onopen`. The server handles it, sets `sessionId` and writes the auth response. Then Connect runs and writes the greeting `{"stage":{"currentScene":"login"}}` *after* the auth response, so the client returns to the login scene. Any broadcast to that session between login and `Hub.Add` is also missed, because `deliverLocal` iterates `Transport.Conns()`. (2) The client closes right after open, and OnClose wins the race. A closed `*QueuedConn` then stays in `Hub.conns` forever, a slow leak that every `Conns()` scan pays for.
- Suggested fix: in OnOpen, take `p.mu`, bail if the peer is closed (add a `closed` flag set in `close()` under `p.mu`), then Hub.Add, start the writer, and fire Connected. Gate Deliver on a `ready` channel that OnOpen closes. Blocking pion's read loop until then is fine, because it is one goroutine per channel. Add a conformance case "first message sent immediately on open is handled after Connect".

### [P2] Slow clients silently lose frames and stay connected, so their positional deltas drift out of sync
- ID: TR-2
- Category: bug
- Location: internal/transport/hub.go:95-109; internal/transport/ws/ws.go:32-33 (melody `Session.Write` returns nil and drops through `errorHandler(ErrMessageBufferFull)`); internal/services/broadcast/broadcast.go:257-259; internal/services/boot/handlers.go:34-36
- Confidence: CONFIRMED
- Prior: Review B #10 (deferred by design)
- What: When the queue is full, a frame is dropped. For QueuedConn transports the broadcaster only logs the drop. For ws the caller sees success, because melody reports the drop only to `HandleError`, whose handler just logs "client transport error". Positional deltas cannot be replayed.
- Failure scenario: A phone on a slow link lets 1024 frames back up. Later deltas are dropped, and every later delta then applies to stale state or the wrong positions until the client happens to get a keyframe.
- Suggested fix: as planned. Close on `ErrBufferFull` in QueuedConn, and close the melody session in `HandleError` on `ErrMessageBufferFull`. Ship it together with resume.

### [P2] SSE and graphqlws writers have no write deadline, so one non-reading client can pin a connection forever and block kick and shutdown
- ID: TR-3
- Category: security / concurrency
- Location: internal/transport/sse/sse.go:72 (write deadline cleared, never set again), sse.go:105-111; internal/transport/graphqlws/graphqlws.go:338, 385 (`WriteJSON` without `SetWriteDeadline`), graphqlws.go:310-315 (`reply` blocks on a full `ctrl`)
- Confidence: CONFIRMED (by code reading; not reproduced)
- Prior: new
- What: Only ws (melody `writeRaw` sets `WriteWait`) bounds writes. The SSE stream clears its write deadline to stay open, and graphqlws never sets one, so a write to a peer that stops reading (TCP zero window) blocks with no limit. While it is blocked, the loop cannot see `Done()`, so kick and `Transport.Close` never finish, `Disconnect` never fires, and the conn stays in the hub. In graphqlws it is worse. Once the writer is stuck, `reply` blocks as soon as the 16-slot `ctrl` channel fills (it only unblocks on `writerDone`), and then the read loop is stuck too. The read deadline can no longer fire, and nothing ever closes the socket.
- Failure scenario: A graphqlws client acks, subscribes, stops reading, and sends 17+ `{"type":"ping"}` messages. Both goroutines block forever, and so do the socket, the 1024-frame queue and the hub entry. Repeating this exhausts FDs and memory. For SSE, a stalled reader holds its stream goroutine until the kernel gives up (never, if the peer keeps ACKing zero-window probes). `server.Shutdown` then waits out its full 10s and logs an error.
- Suggested fix: set a per-write deadline (`rc.SetWriteDeadline(time.Now().Add(writeWait))` before each SSE emit, and `ws.SetWriteDeadline` before each graphqlws write), reusing `INDRI_WS_WRITE_TIMEOUT`. On any write failure, close the socket/stream.

### [P2] Disconnect marks a player offline even when that user still has a live or newer connection (tracked in the session-resume plan)
- ID: TR-13
- Category: bug
- Location: internal/entrypoints/websocket.go:27-61; `ConnectPlayer` has no non-test caller (only repo/game/*_player.go)
- Confidence: CONFIRMED
- Prior: plans/session-resume.md bugs 1 and 3 (known, not from a review)
- What: `HandleDisconnect` unconditionally calls `DisconnectPlayer` for the conn's session. Sessions are one per user, and nothing calls `ConnectPlayer` again after `reconnect`.
- Failure scenario: A user has two tabs, or reconnects on a new transport before the old half-open one times out (60s pong wait). When the old connection closes, the slot is set `Connected=false` while the new connection is live, and it stays false for the rest of the game.
- Suggested fix: the epoch design already in `plans/session-resume.md`. Listed here only because entrypoints is in this slice.

### [P3] Kick does not disconnect a slow WebSocket client
- ID: TR-4
- Category: bug
- Location: internal/services/connection (Close → `c.Write` then `c.Close`); internal/transport/ws/ws.go:34; melody v1.3.0 `session.go` `Close` → `writeMessage` (`select … default: errorHandler(ErrMessageBufferFull)`)
- Confidence: CONFIRMED
- Prior: new (related to B10)
- What: melody's `Session.Close()` only *enqueues* a close envelope on the same bounded output channel. On a full buffer both the `{"disconnected":true}` notice and the close are dropped, `Close()` still returns nil, and `IsClosed()` stays false. QueuedConn's `Close` is synchronous and is not affected.
- Failure scenario: A host kicks a player whose ws buffer is full. The slot is removed (kick.go:61), but the socket stays open with `sessionId` set, and the client never learns it was kicked.
- Suggested fix: in `ws.conn.Close`, on a full buffer close the underlying connection directly (`s.WebsocketConnection().Close()`). The TR-2 fix also covers this, if the Error handler closes the session on `ErrMessageBufferFull`.

### [P3] Same-host origin rule accepts DNS-rebinding pages and ignores the scheme
- ID: TR-6
- Category: security
- Location: internal/transport/origin.go:36-38 (used by ws.go:53, graphqlws.go:73, and `Route` for sse/webrtc)
- Confidence: CONFIRMED
- Prior: Review A #10
- What: Any Origin whose host:port equals the request's `Host` header is allowed. A rebinding page at `http://evil.test:5002` sends `Origin: http://evil.test:5002` and `Host: evil.test:5002`, which is allowed. Because auth is a bearer token in messages, not cookies, the practical gain is reach: `ListenAddress` defaults to `localhost` (env.go:16), so a web page can drive a developer's local or LAN-only server through the victim's browser.
- Failure scenario: A developer runs the server on localhost and visits a malicious page, which rebinds to 127.0.0.1 and opens `/ws`. It can register users and create or join games on that server.
- Suggested fix: needs Boss's decision. Options: keep the rule only when `Host` is in an allowlist of expected hostnames, or drop the rule and have the iOS client send an allowlisted Origin. At minimum, document the rebinding trade-off next to the rule.

### [P3] WebRTC ignores INDRI_WS_MAX_MESSAGE_SIZE for inbound messages
- ID: TR-7
- Category: security
- Location: internal/transport/webrtc/webrtc.go:41-67, 80-98 (no size cap, no `SetSCTPMaxMessageSize`); internal/injector/transport.go:101-111 (not passed)
- Confidence: CONFIRMED that the configured cap is not applied. The effective ceiling is PLAUSIBLE.
- Prior: new
- What: ws, sse and graphqlws enforce `WSMaxMessageSizeBytes` (32 KiB by default). WebRTC passes every data-channel message straight to `Deliver`. pion's read loop grows its buffer up to `SettingEngine.getSCTPMaxMessageSize()`, which defaults to 1 GiB (`constants.go:28`). In practice the SCTP receive window (1 MiB, sctp `initialRecvBufSize`) probably bounds a single message. To confirm: send a 900 KiB message and check that the router receives it.
- Failure scenario: A client sends a roughly 1 MiB JSON message over the data channel. It is decoded into `map[string]interface{}` by the router, about 30× the limit the operator configured, from an unauthenticated connection.
- Suggested fix: add `MaxMessageSize` to `webrtc.Config`, apply it with `se.SetSCTPMaxMessageSize`, and also drop (or close on) `len(m.Data) > max` in OnMessage.

### [P3] WebRTC silently drops a connection when an outbound frame exceeds the peer's SCTP max-message-size
- ID: TR-8
- Category: bug
- Location: internal/transport/webrtc/webrtc.go:313-317, 335-341
- Confidence: PLAUSIBLE
- Prior: new
- What: `dc.Send` fails with `ErrOutboundPacketTooLarge` when a message is larger than the remote's advertised max-message-size (sctp `stream.go:312-314`). The limit defaults to 65535 when the SDP has no `a=max-message-size` (webrtc `sctptransport.go:144-147`), and Chrome/libwebrtc advertise 256 KiB. The writer then calls `p.close()` with no log. Layout frames may be up to 256 KiB serialized (PROTOCOL.md:287), plus the envelope.
- Failure scenario: A game with a large layout (over about 64 KiB against a peer with no max-message-size, or near the 256 KiB cap against Chrome). Every join or keyframe write tears the peer down. The client reconnects and hits the same wall, so it can never join. To confirm: a test that sends a 100 KiB `WriteBinary` to a pion client that omits max-message-size.
- Suggested fix: log send errors. Check `pc.SCTP().GetCapabilities().MaxMessageSize` (or chunk at the app level) before sending, and fail loudly in the handler instead of disconnecting.

### [P3] No connection caps or pre-auth idle timeout. WebRTC's global MaxPeers can be exhausted by one client
- ID: TR-9
- Category: security
- Location: internal/transport/ws/ws.go:81-87; sse/sse.go:64-141; graphqlws/graphqlws.go:115-152; webrtc/webrtc.go:188-194
- Confidence: CONFIRMED
- Prior: new
- What: ws, sse and graphqlws accept unbounded connections, each with a 1024-slot channel and 1-2 goroutines. Nothing closes a connection that never logs in (graphqlws only times out the missing `connection_init`, and after `connection_ack` it can idle forever). WebRTC has a global `MaxPeers=256` but no per-client limit, so one unauthenticated client that opens 256 data channels locks everyone out of WebRTC (503 "too many peers").
- Failure scenario: A script opens 256 WebRTC peers and holds their data channels open, or opens thousands of `/sse/stream` requests. New players get 503s, or the process runs out of FDs and memory.
- Suggested fix: a per-IP connection limit in `Route` and the upgrade handlers, and a login deadline (close a conn with no `sessionId` after N seconds). Document that a reverse proxy is expected to rate-limit.

### [P3] WebRTC: data race on `peer.pc` and a leaked PeerConnection when Close races an in-flight offer
- ID: TR-10
- Category: concurrency
- Location: internal/transport/webrtc/webrtc.go:186-202 (peer published in `t.peers` before `p.pc` is assigned without a lock); webrtc.go:114-128, 361-363 (`close` reads `p.pc`)
- Confidence: CONFIRMED (data race by the memory model). The leak is shutdown-only.
- Prior: new
- What: `newPeer` inserts `p` into `t.peers` under `peersMu`, releases the lock, then writes `p.pc = pc`. `Transport.Close` can snapshot `t.peers` and call `p.close()` concurrently, which reads `p.pc` (a race). If it reads nil, `closeOnce` is spent without closing the pc. The offer then proceeds (answer, ICE, DTLS), and every later `p.close()` is a no-op, so that PeerConnection is never closed.
- Failure scenario: SIGTERM arrives while an offer is being answered. `-race` flags it, and the pion PeerConnection with its UDP sockets outlives the transport. This matters for tests and embedders more than for process exit.
- Suggested fix: create the PeerConnection first and publish the peer (with `pc` set) under `peersMu`, re-checking `IsClosed` there. Or guard `pc` with `p.mu`.

### [P3] Transport timing and size settings are unvalidated, and PingPeriod=0 crashes the process
- ID: TR-11
- Category: bug
- Location: internal/injector/transport.go:55-117; internal/transport/ws/ws.go:44-51; graphqlws/graphqlws.go:326; sse/sse.go:121
- Confidence: CONFIRMED
- Prior: new
- What: The injector validates the GraphQL init timeout and the WebRTC settings, but not the shared `WS*` values:
  - `INDRI_WS_PING_PERIOD=0` → `time.NewTicker(0)` panics in melody's `writePump` goroutine and in the graphqlws `writer` goroutine. Neither is recovered, so the **process crashes on the first connection**. For SSE the panic is inside the HTTP handler and is recovered per request.
  - `PING_PERIOD >= PONG_TIMEOUT` → every idle ws/graphqlws connection hits its read deadline and drops.
  - `WS_MESSAGE_BUFFER_SIZE=0` → an unbuffered queue, and nearly every QueuedConn write returns `ErrBufferFull`.
  - `WS_MAX_MESSAGE_SIZE=0` → no limit at all on ws/graphqlws (gorilla treats 0 as unlimited), but SSE rejects every POST.
  - `WEBRTC_GATHER_TIMEOUT >= 10` exceeds the hard-coded `WriteTimeout: 10s` (entrypoints/http.go:23), which webrtc.go:58-59 warns against.
- Failure scenario: An operator sets `INDRI_WS_PING_PERIOD=0` to "disable pings", and the server dies on the first client.
- Suggested fix: validate these fields in `newTransport` (positive; ping < pong; gather < server write timeout), returning an error at boot the way the webrtc checks do.

### [P3] deliverLocal scans every connection and re-encodes the payload for each recipient
- ID: TR-12
- Category: performance
- Location: internal/services/broadcast/broadcast.go:231-261
- Confidence: CONFIRMED
- Prior: Review B cleanup ("one MessagePack encode per recipient")
- What: Every delta for every game calls `Transport.Conns()` (it allocates a slice of *all* conns on the instance, and melody's `Sessions()` copies under its lock), does an O(recipients) `slices.Contains` per conn, and re-runs `msgpack`/`json` encoding per recipient.
- Failure scenario: 200 games × 8 players on one instance, with a delta per move. Each move allocates and scans 1600 conns and encodes 8 times. Cost grows with total instance load, not game size.
- Suggested fix: build a `map[string]struct{}` of the session IDs, encode once per format (msgpack/json) and reuse the bytes. Longer term, a sessionId→conns index maintained by the transport layer (set when login/reconnect call `SetKey`).

### [P3] Test gaps for the documented transport invariants
- ID: TR-20
- Category: tests
- Location: internal/transport/transporttest/transporttest.go; internal/entrypoints (no tests)
- Confidence: CONFIRMED
- Prior: new
- What: The conformance suite never covers:
  - a message sent immediately on open being handled after Connect (it would catch TR-1);
  - buffer-full behavior (TR-2, TR-4; the plan already asks for this);
  - a writer blocked by a non-reading client (TR-3);
  - per-transport inbound size limits (TR-7);
  - messages arriving after a server-side Close (TR-5).

  `entrypoints` has no tests at all, although HandleDisconnect is on the kick, logout and transport-close paths.
- Failure scenario: The races above pass CI today. The webrtc harness sends only after it has observed Connect, which hides TR-1.
- Suggested fix: add those cases to `transporttest.Run`, and add handler-level tests for `HandleDisconnect` (no session, logout double-invoke, stale session).

### [P4] QueuedConn.Deliver still dispatches messages after the conn is closed
- ID: TR-5
- Category: bug
- Location: internal/transport/hub.go:171-180; webrtc.go:302-304, 318-330 (flush window up to `flushTimeout`=1s); graphqlws.go:251
- Confidence: CONFIRMED
- Prior: new
- What: After a server-side Close (kick), webrtc keeps the peer up to 1s to flush, and graphqlws keeps reading until the writer closes the socket. In both cases inbound messages still reach the router with the conn's `sessionId` still set. SSE is the only transport that refuses them (`sse.go:145`). Impact is low, because kick removes the slot first (kick.go:61), so game actions fail their holder check.
- Failure scenario: A kicked WebRTC client fires `logout` or `layout` actions in the 1s flush window, and they run as the kicked session.
- Suggested fix: `if c.IsClosed() { return }` at the top of `Deliver`, re-checked after taking `deliverMu`.

### [P4] Transport.Broadcast and BroadcastFilter have no production caller, and deliverLocal discards Composite's partial results
- ID: TR-14
- Category: maintainability
- Location: internal/transport/transport.go:47-52; hub.go:266-291; composite.go:32-65; ws.go:89-97; broadcast.go:236-240
- Confidence: CONFIRMED
- Prior: new
- What: The only production fan-out is `deliverLocal` over `Conns()` (grep finds no other caller of `Broadcast`, `BroadcastFilter` or `Conns`). Every transport and the conformance suite still carry both broadcast methods (YAGNI). Separately, `Composite.Conns` documents returning partial results plus the errors, but `deliverLocal` returns on any error and delivers to nobody.
- Failure scenario: With `ws,sse`, if the ws child is closed (melody's `Sessions()` returns ErrClosed) while sse is open, no SSE client gets any delivery. This does not happen today, because both close together at shutdown.
- Suggested fix: drop Broadcast/BroadcastFilter from the interface, or have `deliverLocal` use BroadcastFilter. Log the error and still iterate the partial `conns`.

### [P4] ws Register writes a second HTTP response after a failed upgrade
- ID: TR-15
- Category: bug
- Location: internal/transport/ws/ws.go:81-87
- Confidence: CONFIRMED
- Prior: new
- What: melody's `HandleRequest` returns gorilla's Upgrade error after gorilla has already written the 400/403 response. `http.Error(w, …, 500)` then writes again, which net/http logs as "superfluous response.WriteHeader call". A closed transport (`melody.ErrClosed`) gets a 500 instead of 503.
- Failure scenario: Any cross-origin or non-upgrade `GET /ws` logs a superfluous-WriteHeader line, so a client can spam the logs at will.
- Suggested fix: map `melody.ErrClosed` to 503, and otherwise don't write (the upgrader already responded).

### [P4] ws.New reads the global env instead of the vars newTransport was given
- ID: TR-16
- Category: maintainability
- Location: internal/transport/ws/ws.go:39-56, 12; internal/injector/transport.go:57-58
- Confidence: CONFIRMED
- Prior: new
- What: sse, graphqlws and webrtc take a `Config` built from `vars`. `ws.New()` calls `envVars.GetEnv()` itself, so `newTransport(vars)` silently ignores `vars` for ws, and `internal/transport/ws` depends on `internal/repo/env`, against the hexagonal layering.
- Failure scenario: A test or embedder calls `newTransport(customVars)` with a different `AllowedOrigins`, and ws enforces the global value instead.
- Suggested fix: `ws.New(ws.Config{…})`, like its siblings.

### [P4] graphqlws writer exits on a write error without closing the socket, and never-subscribed sockets survive Transport.Close
- ID: TR-17
- Category: bug
- Location: internal/transport/graphqlws/graphqlws.go:337-346, 363-366, 115-152
- Confidence: CONFIRMED
- Prior: new
- What: On a failed write the writer `return`s (closing `writerDone`) but not the socket. The conn stays in the hub, its queue fills and drops, and the read loop keeps handling Sends until the read deadline, which a client can keep extending by sending messages. A socket that acked but never subscribed to `IndriEvents` is not in the hub, so `Transport.Close` never closes it.
- Failure scenario: A transient write error leaves a zombie connection that still accepts and handles actions but can never receive anything.
- Suggested fix: `defer s.closeWith(...)` in the writer. Track sockets (not just conns) for `Close`.

### [P4] HandleConnect's failure path is dead, and logout runs HandleDisconnect twice
- ID: TR-18
- Category: maintainability
- Location: internal/entrypoints/websocket.go:20-24, 27-61; internal/handlers/actions/logout/handler.go:31
- Confidence: CONFIRMED
- Prior: new
- What: (a) On a greeting write failure, `HandleConnect` calls `HandleDisconnect`, which returns at once because a new conn has no `sessionId`. The conn is neither closed nor cleaned up. (b) logout calls `HandleDisconnect`, which closes the conn, and the transport's Disconnect then calls it again. By then logout has usually deleted the session, so every logout logs "error getting session". Otherwise `DisconnectPlayer` runs twice. The file is also named `websocket.go` although it is transport-agnostic (ARCHITECTURE.md:80-81 admits this).
- Failure scenario: Every logout produces a spurious error log line.
- Suggested fix: in HandleConnect, just `connection.Close(s)` on failure. In logout, `UnsetKey("sessionId")` after the cleanup so the transport-driven call no-ops. Rename the file `lifecycle.go`.

### [P4] SSE `event()` splits data only on "\n"
- ID: TR-19
- Category: bug
- Location: internal/transport/sse/sse.go:180-194
- Confidence: CONFIRMED (the trigger is rare)
- Prior: new
- What: The SSE spec ends a line at `\r`, `\n` or `\r\n`. A raw `\r` in a text frame splits the data field on the client and silently drops the text after it. JSON from `json.Marshal` escapes `\r`, so only non-JSON `Conn.Write` text is affected.
- Failure scenario: A game handler writes a plain-text message containing `\r\n`. The client receives it truncated or split.
- Suggested fix: normalize or split on `\r\n|\r|\n`, or send any text frame containing `\r` as a binary (base64) event.

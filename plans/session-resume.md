# Session resume

Status: **decided (2026-09-28), not built.** Boss's decisions are recorded at the end. Two follow-up
values for the abandoned-slot timeout are still open.

Goal: when a client's transport drops (network blip, app backgrounded, SSE wrong-instance `404`,
server restart), it reconnects and resumes as the same player in the same slot, instead of landing on
the login screen while its old slot shows as disconnected.

## Where we are (line refs are for `wip/transports-compact-delta`)

**The server half mostly exists:**
- `reconnect` (`internal/handlers/actions/reconnect/handler.go`) takes `{sessionId: <token>}`, looks it up
  with `GetByToken`, sets the connection key, and replies `{authenticated, sessionId, user}`. It follows
  with a layout frame and a keyframe if the session has a game.
- Disconnect (`internal/entrypoints/websocket.go:27-69`) only sets `slot.Connected = false`. The slot
  stays assigned, so resume needs **no grace timer**: the slot waits.
- Sessions are one per user and the token never rotates; `login` returns the existing one.

**Server bugs this plan must fix:**
1. **`reconnect` never marks the player connected again.** `ConnectPlayer` exists in every store but
   has no caller. After a resume, everyone else still sees the player as disconnected.
2. **`kick` and `leave` leave `GameID`/`SlotID`/`TeamID` in the session.** A kicked player who
   reconnects is sent the game's keyframe again, and `reconnect` never checks that the slot still
   belongs to them. (Partly fixed, `wip/review-fixes`: slot operations now take the caller's user and
   act only if that user still holds the slot, so a stale session can no longer disconnect or remove
   the slot's new holder. Clearing the session's game fields is still to do.)
3. **A stale connection's disconnect can overwrite a fresh resume.** The old transport may only notice
   the drop after the new one has reconnected (a half-open TCP connection or SSE stream). Its
   `HandleDisconnect` then runs `DisconnectPlayer` and marks the just-resumed player offline. With
   several instances, the old and new connections can be on different servers.
4. **Fixed** (`wip/review-fixes`): `assignSlot` now gives a user at most one slot. Joining their team
   again keeps and reconnects it; joining another team moves them there, host flag included.
5. **Session expiry is inconsistent.** Only Mongo expires sessions: a TTL on `createdAt`, 7 days, never
   refreshed, so it can expire an *active* player mid-game. Memory, SQLite and Postgres never expire
   sessions.

**The client half doesn't exist:**
- The token is saved (`AsyncStorage.setItem('sessionId')`, `user-state-actions.tsx:17`) but never read
  back.
- `reconnect` is never sent.
- `onClose` is a `//TODO: Handle reconnects` log (`message-handler.ts:86-89`).
- A `TransportClient` can `connect()` only once (`transport.ts:126-129`), so reconnecting means a new
  client from `createTransportClient`.

**Protocol:**
- Deltas carry only wall-clock `t`, and `Game.Version` is hidden from clients, so there's no way to ask
  for "deltas since X".
- A keyframe on resume is the right, simple answer: it's authoritative, and the client already discards
  deltas older than a keyframe's cutoff. **No delta replay (YAGNI).**

## Design

### Server

- **Connection epoch (fixes bug 3, works across instances).** Add `Epoch int64` to `models.Session`.
  - `login` and `reconnect` increment it atomically in the session store and store the new value on
    the connection (`Set("sessionEpoch", n)`).
  - `HandleDisconnect` calls `DisconnectPlayer` only if the connection's epoch still equals the
    session's current epoch. A stale connection's disconnect is then a no-op.
  - The check goes through the shared session store, so it holds when the old and new connections are
    on different instances.
  - It needs a new session-store method with a contract test for every backend:
    `BumpEpoch(id) (int64, error)`, a conditional increment.
- **`reconnect` becomes a real resume (fixes bugs 1 and 2).**
  - After binding, if the session has `GameID`/`SlotID`, check that the slot's `UserID` is still the
    session's user.
  - If it is: `ConnectPlayer`, then the layout frame and keyframe (as today).
  - If it isn't (kicked, left, or the game is gone): clear the session's game fields and reply without
    a keyframe. The client lands on Join/Create.
- **`kick`/`leave` clear the target's session game fields** (fixes bug 2 at its source).
- **`join`/`create` are idempotent per user (fixes bug 4).**
  - If the caller already holds a slot in that game, return it: `ConnectPlayer` plus a keyframe, not a
    second slot.
  - `assignSlot` gains the check, and the store contract suite gains a test.
- **Invalid-token reply.** `reconnect` with an unknown or expired token returns a distinct
  `models.WSError` code (`session_invalid`), so the client knows to drop the token and show login,
  rather than retrying.
- **Expiry, uniform across stores (fixes bug 5):**
  - Sessions carry `LastSeenAt`, refreshed on `login`/`reconnect` (and on disconnect, so idle time
    counts from when the player left).
  - `GetByToken` rejects sessions idle longer than `INDRI_SESSION_MAX_IDLE` (**default 24h**, decided).
    This is enforced in application code on every store, with one contract test, not by a TTL sweep.
    That also closes today's Mongo gap, where an expired session stays readable until the asynchronous
    TTL sweep runs.
  - This replaces the stores' current rule: 7 days from `createdAt`, never refreshed, so it expires
    active players.
  - Cleanup: every store deletes expired rows when a session is created, as the Postgres store does
    today. Mongo's TTL index moves to `lastSeenAt` and stays as a backstop.
- **Duplicate live connections: last connection wins (decided).** A resume or login while an older
  connection for the same session is still open closes the older ones with
  `{"disconnected": true, "reason": "replaced"}`, on whichever instance they're on.
  - It uses `BroadcastService.CloseSessions`, which already crosses instances. `events.Delivery` gains
    a `Reason` so the notice can say why.
  - It must close only the *older* connections, not the new one, which shares the session id. The
    close delivery therefore carries the new connection's epoch (see "Connection epoch" above), and
    instances skip connections with that epoch.
- **Token rotation: none (decided).** A session keeps one token for its life.
- **Abandoned slots: freed after a timeout (decided).**
  - Disconnecting records `disconnectedAt` on the slot; connecting clears it.
  - A slot disconnected longer than `INDRI_SLOT_ABANDON_AFTER` is freed:
    - lazily, by `assignSlot`, which treats such a slot as empty;
    - and by a periodic sweep on every instance, so other players see the slot open up even if nobody
      joins.
  - The sweep frees slots through `Mutate`, checking inside `apply` that the slot is still
    disconnected and past the timeout. Several instances can sweep at once safely: the version fence
    and the re-check make it idempotent. So no cross-instance timer or leader election is needed.
  - Freeing a slot clears the player's session game fields (the same operation `leave`/`kick` need,
    bug 2).
  - If the freed player was the host, the host flag passes to another connected player (rule below).
  - The sweep walks games with a disconnected player. Each store needs a query for that, or a scan of
    open games at first. Measure before optimizing.

### Client

- **`@indri/protocol-client` gets a transport-level wrapper**, `createReconnectingClient(kind, config,
  options)`.
  - It implements `TransportClient` by building a fresh adapter per attempt.
  - It reconnects on remote close, with exponential backoff and jitter, and gives up after a limit.
  - It exposes `onReconnect`, `onReconnecting` and `onGiveUp`.
  - It knows nothing about sessions (hexagonal: the transport package stays protocol-agnostic).
  - A local `close()` never reconnects. SSE `404` is already a remote close, so wrong-instance recovery
    comes for free.
  - Tested with the existing fake-injection style (`*.node-test.ts`).
- **`MessageHandler` owns the resume handshake:**
  - **Startup:** `AsyncStorage.getItem('sessionId')`. If a token exists, send `reconnect` on open,
    otherwise show login.
  - **`onReconnect`:** send `reconnect` with the stored token.
  - **`session_invalid`:** remove the token, clear user and game state, show login.
  - **`{disconnected: true, reason: "replaced"}`:** stop and don't reconnect; show "signed in
    elsewhere". **Kicked:** reconnect is allowed and lands on Join/Create, because the session no longer
    has the game.
  - **`logout`:** remove the stored token (verify that happens today; the research found only
    `setItem`).
- **UI:**
  - A "reconnecting…" state disables game input while down.
  - Sends while down are dropped with a warning, as today. They are **not queued**: an action queued
    against pre-disconnect state could be wrong after resume, and the keyframe re-syncs the player
    anyway.

### Slow clients: close instead of dropping (review finding #7)

Today a connection whose outbound buffer is full drops the frame and stays open. That happens in
`QueuedConn.enqueue` (`ErrBufferFull`) for sse, graphqlws and webrtc, and in melody's
`ErrMessageBufferFull` for ws. Positional deltas can't be resent, so that client drifts out of sync for
good.

With resume in place, the fix is to close the connection on a full buffer. The client then reconnects,
resumes, and gets a fresh keyframe.
- **Where:** `QueuedConn` closes itself on `ErrBufferFull`, and the ws transport's `HandleError` closes
  the melody session on `ErrMessageBufferFull`.
- **Test:** a conformance-suite case that fills one connection's buffer and expects `Disconnect`.

This is deliberately scheduled with resume: without resume, closing would send the player back to the
login screen.

### Docs

- `docs/PROTOCOL.md`: document resume, `session_invalid`, the `replaced` reason, idle expiry, and that
  `logout` deletes the session.
- Also fix two existing mismatches:
  - `kick` is documented with `userId`, but the code reads `slotId`.
  - PROTOCOL says the SSE `404` reconnects; it will once this plan ships.

## Test plan (TDD, in order)

1. Session store contract, on every backend:
   - `BumpEpoch` is monotonic under concurrency;
   - idle expiry in `GetByToken`;
   - `LastSeenAt` refresh.
2. Game store contract: `assignSlot` returns the existing slot for a user who already holds one.
3. Handler tests:
   - `reconnect` → `ConnectPlayer` and a keyframe;
   - `reconnect` after a kick → no keyframe, session cleared;
   - a stale-epoch disconnect → no `DisconnectPlayer`;
   - an invalid token → `session_invalid`.
4. Client:
   - the reconnecting wrapper (backoff, give-up, local close vs remote close, a fresh adapter per
     attempt);
   - `MessageHandler` resume flows with a fake transport.
5. E2E (the Node harness from the transport work): drop a player's transport mid-game on each of `ws`,
   `sse`, `graphqlws` and `webrtc`, then check:
   - the player resumes in the same slot;
   - the other player sees `connected` go false and then true;
   - the rebuilt state equals a fresh keyframe.

## Decisions (Boss, 2026-09-28)

1. **Expiry:** 24 hours idle (`INDRI_SESSION_MAX_IDLE`, default `24h`), refreshed by login, reconnect
   and disconnect. Every store rejects expired sessions on read and deletes them when a session is
   created.
2. **Duplicate connections:** last connection wins; older ones are closed with reason `replaced`.
3. **Token rotation:** none.
4. **Abandoned slots:** freed after a configurable timeout, lazily by `assignSlot` and by a periodic
   sweep.

### Still open (abandoned slots)

- **Default for `INDRI_SLOT_ABANDON_AFTER`.** Not chosen yet. It must be longer than a typical
  reconnect (the client's backoff give-up limit), or a resume could find its slot gone.
- **Host handoff when the host's slot is freed:** the first connected player in slot order, or no host
  until someone claims it?

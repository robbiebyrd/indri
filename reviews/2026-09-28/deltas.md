# Review slice: keyframe / delta / layout-frame pipeline (server + client)

Reviewed at 6330d92 (wip/codebase-review, = main 3eab208 + review docs). Read-only; all proofs were
throwaway tests in the scratchpad (`scratchpad/proof/`), run against the repo via `go test -overlay`
and `node --test` with absolute imports. Nothing in the repo was modified.

Test runs:
- `go test -race ./internal/services/events/... ./internal/services/boot/... ./internal/repo/game/... ./internal/services/broadcast/...` — all `ok` (needed `GOCACHE=$TMPDIR/gocache` because of the sandbox).
- `client/node_modules` does **not** exist in this worktree, so `pnpm test` / `pnpm run typecheck` were not run and nothing was installed. The slice's node tests only need Node built-ins, so I ran them directly: `node --experimental-strip-types --test services/*.node-test.ts packages/protocol-client/src/*.node-test.ts packages/protocol-client/src/adapters/*.node-test.ts` → 66/66 pass. **Typecheck was not verified.**

## Prior findings re-check

| Prior | Status | Evidence |
|---|---|---|
| Review B #3 (setSchema aliases the keyframe object) | **FIXED** | `client/services/game-state-parser.ts:27-29` deep-clones the schema. Test "the schema is a snapshot…" passes. |
| Review B #8 (layout edits never reach clients) | **FIXED** | `internal/repo/game/changes.go:41-43` publishes `OpLayout` when `data.layout` differs. `internal/services/boot/monitor.go:62-63` loads `GameService.LayoutFrame` (per game, `internal/services/game/game.go:228`). The client routes `o:4` to `handleLayout` (`message-handler.ts:149,177`). See DP-12 for a remaining gap: a missed layout frame is never recovered. |
| Review B #9 (array shrink leaves holes) | **FIXED** | `game-state-parser.ts:166-171` truncates the array. `delta.go:76-78` still sends one removal per trailing index, and truncating to `min` makes that idempotent. Test passes. |
| Review B cleanup: dead pass-through in `SanitizeDelta` | **STILL-OPEN** (and larger than first reported) | `delta.go:109-111` and `123-125`: the only caller (`changes.go:64`) passes only string paths, so the `[]interface{}` branches are unreachable. A test pins them (`positional_test.go:90`). See DP-16. |
| Sep 9 (83c2be0/7dc395f): privateData leaked in deltas | **FIXED** for the delta and keyframe paths | `changes.go:45` diffs `ClientView` output. `stripKey` removes `privateData` at every depth, including inside arrays and `playerData`. `TestChangePublisher_PrivateOnlyChangePublishesNothing`. **However**, the Lua `refresh` path still uses the old shallow `GameService.Sanitize` and leaks nested privateData. See DP-4. |
| Sep 9 (f39cc50): inverted `deleteBefore` (data loss) | **FIXED** | `game-state-parser.ts:122-127` keeps `t > cutoff`. Test "a delta newer than an incoming keyframe survives" passes. A minor `<` vs `<=` inconsistency with `update()` is in DP-18. |
| Sep 9: pre-keyframe delta throw / keyframe mutation / prototype pollution / arrays turned into maps | **FIXED** | `reapply` returns early (`:87`). A fresh `deepClone` is made on each reapply (`:94`). `UNSAFE_KEYS` guard (`:11,135,146`). Numeric next segment gives `[]` (`:140`). All have passing tests. A new prototype-related decode issue is in DP-6. |

## Findings (severity-sorted)

### [P2] Delta ordering and keyframe cutoff use wall-clock timestamps from whichever instance wrote (multi-instance)
- ID: DP-1
- Category: concurrency
- Location: internal/repo/game/changes.go:42,49,81 (`time.Now()`); internal/repo/game/memory.go:374, mongo.go:176, sqlite.go:197, postgres.go:217 (`UpdatedAt = time.Now()`); client/services/message-handler.ts:188; client/services/game-state-parser.ts:49-61,122-127
- Confidence: CONFIRMED (client-side proof, `scratchpad/proof/client-proof.node-test.ts`)
- Prior: new
- What: The client orders deltas by `t`, and it drops deltas older than the keyframe's `updatedAt`. Both values are wall-clock times taken on the instance that did the write. With `INDRI_LOCK_BACKEND=redis`, consecutive writes to one game can happen on different instances, whose clocks are skewed. The server does have a per-game monotonic counter (`Game.Version`), but it is `json:"-"` and never reaches the client.
- Failure scenario (proved): the keyframe has `updatedAt` 00:10.000, written by instance A. Instance B's clock is 1 s behind, and it then writes `score=5` with `t`=00:09.000. The client drops it (`update()` returns early), so the score stays 0 until the next shape-changing keyframe. Second case: A (clock ahead) writes x=1 at t=20.0, then B writes x=2 at t=19.5. The client sorts B's delta first and ends at x=1 while the server has 2. Third case: a delta encoded against the pre-shape-change schema has a skewed `t` greater than the new keyframe's `updatedAt`. It survives `deleteBefore` and is resolved against the new schema, so it writes a value into the wrong key.
- Suggested fix: expose `Version` as a sequence number on both keyframes and change events. Order by it and cut off by it on the client, and drop timestamps from the ordering logic.

### [P2] A Redis subscription error permanently stops all delta and relay delivery on that instance
- ID: DP-2
- Category: bug
- Location: internal/services/events/redis.go:64-71; internal/services/boot/monitor.go:26-29; internal/services/broadcast/broadcast.go:112-115; internal/services/boot/serve.go:17-18
- Confidence: CONFIRMED (code reading, plus go-redis v9.22.0 `pubsub.go:535-540`: `ReceiveMessage` returns the error to the caller and expects a retry. go-redis's own `Channel()` retries with backoff at `pubsub.go:757-772`)
- Prior: new
- What: On the first `ReceiveMessage` error (a Redis restart, a failover, a network blip, or Redis disconnecting a slow subscriber at `client-output-buffer-limit pubsub`), the goroutine logs, closes `out`, and returns. `monitorGameChanges` and `RelayDeliveries` then treat the closed channel as a clean shutdown and `return nil`. An errgroup goroutine that returns nil cancels nothing, so the HTTP server keeps accepting players while that instance never again broadcasts a delta, a keyframe, a layout frame, a relayed send, or a kick.
- Failure scenario: restart Redis for 1 s while the server is running. Every instance's monitor exits silently. Games still accept moves (writes commit and publish), but no client sees any update until the process restarts.
- Suggested fix: resubscribe with backoff inside `Subscribe` (or use `sub.Channel()`), and return an error from the loops when the channel closes before ctx is done, so the process fails loudly. Pub/Sub is at-most-once, so after a resubscribe also send a keyframe to every local game.

### [P2] A player who joins misses every delta and layout frame committed between the keyframe load and the session update
- ID: DP-3
- Category: concurrency
- Location: internal/handlers/actions/join/handler.go:68-84 (same order in internal/handlers/actions/create/handler.go:70-80); internal/services/broadcast/broadcast.go:72 (`gameSessions` resolves recipients from the session store at broadcast time)
- Confidence: CONFIRMED (code reading)
- Prior: new
- What: `join` does `Get(fresh)`, then `WriteKeyframe`, and only then `SessionService.Update(GameID…)`. Broadcasts go only to sessions whose `GameID` matches at the time the monitor processes the event. A write committed after `fresh` was loaded, and broadcast before the session update, is neither in the joiner's keyframe nor delivered to them. The client has no gap detection, so later deltas touch only other paths.
- Failure scenario: in a busy game, player X makes a move just as Y joins. Y's keyframe predates the move, and Y's session isn't in the game yet when the move's delta fans out. Y's board shows the cell empty until the next shape-changing keyframe or a manual refresh. An `OpLayout` in the same window leaves Y with the stale layout, and refresh doesn't fix that either (see DP-12).
- Suggested fix: set the session's `GameID` before loading the keyframe. A delta that races ahead of the keyframe is then held by the parser and filtered by the cutoff, or better, by a version number (DP-1).

### [P2] Lua `indri.refresh()`/`refreshSelf()` send a shallow-sanitized game that leaks nested privateData, and the client ignores it anyway
- ID: DP-4
- Category: security
- Location: internal/handlers/luahandler/handler.go:53-72; internal/services/game/game.go:168-190 (`Sanitize`), 73-85 (`GetJSONBytes`, unused)
- Confidence: CONFIRMED (Go overlay proof `scratchpad/proof/sanitize_leak_test.go`: `data.round.privateData.answer` and `players.p1.data.hand.privateData.secret` both appear in the output)
- Prior: related to Sep 9 privateData leak (FIXED for deltas and keyframes only)
- What: `GameService.Sanitize` nils only the top-level `PrivateData` fields of the game, stage, scenes, teams, and players. Any `privateData` nested inside a `data` map survives. CLAUDE.md and ARCHITECTURE promise that `ClientView` removes it "at every depth". The payload also still contains `data.layout`, and it is a bare `models.Game` with no `sv`, so `MessageHandler.messageType` returns `undefined` and the reference client discards it. The comment at game.go:169 ("It matches events.SanitizeDelta") is false.
- Failure scenario: a game stores `data.round.privateData.answer = "PARIS"` and its Lua `join` hook calls `indri.refresh()`. Every player's socket receives the answer. The refresh has no effect on the reference client's UI.
- Suggested fix: remove `Sanitize` and `GetJSONBytes`, and make refresh send `GameService.Keyframe(id)` (a `KeyframeWrapper` built from `ClientView`), broadcast the same way the monitor does for `OpKeyframe`.

### [P3] The SSE client drops its connection whenever the app's message handler throws
- ID: DP-5
- Category: bug
- Location: client/packages/protocol-client/src/adapters/sse.ts:56-74 (the `try` wraps the read loop and calls `ended()` on any exception); transport.ts:209-213 (`emitMessage` doesn't isolate the callback); client/services/message-handler.ts:100-103 (`decode` is not in a try/catch)
- Confidence: CONFIRMED (proof: a handler that throws on the first message ends with `received ['first'] closed: handler bug`, and later events are lost)
- Prior: new
- What: For WS, GraphQL, and WebRTC, an exception in `onMessage` is an uncaught event-handler error and the connection survives. For SSE it unwinds through `parser.push` into `stream()`'s catch and tears the transport down. This breaks the promise that "every adapter behaves the same". Anything that throws in the app's pipeline (a MessagePack `DecodeError`, a reducer bug, a malformed base64 chunk) disconnects SSE clients, and the client has no reconnect (`message-handler.ts:86`, "TODO: Handle reconnects").
- Failure scenario: see DP-6. A layout that contains a `__proto__` key makes every SSE client's connection end on join.
- Suggested fix: wrap the `messageCb` call in `emitMessage` in try/catch and route the error to `emitError`. Also catch decode errors in `routeIncomingMessage`.

### [P3] A `__proto__` key anywhere in a server message makes the default (MessagePack) client throw on decode
- ID: DP-6
- Category: security
- Location: client/services/message-handler.ts:102 (`@msgpack/msgpack` 3.1.3 `Decoder` throws `DecodeError("The key __proto__ is not allowed")`); internal/handlers/actions/layout/handler.go:151,189 (host-chosen `widgetId` and config keys stored verbatim); layout/validate.go (no key-name checks besides `privateData`)
- Confidence: CONFIRMED for the decode throw (proof `msgpack-proof`). That a host can store such a key is from code reading, and I did not run it end to end.
- Prior: new (distinct from the fixed path-segment pollution)
- What: The Go side encodes a map key `__proto__` without complaint. The JS decoder rejects the whole frame. The layout frame is re-sent on every join and reconnect, so one bad key breaks the frame for every binary client of that game, permanently. Debug/JSON clients are unaffected, so the two encodings behave differently.
- Failure scenario: a host sends `{"action":"layout","op":"setWidgetConfig",…,"config":{"__proto__":1}}`. Every player's layout frame fails to decode, so no layout renders. On SSE this also kills the connection (DP-5).
- Suggested fix: reject `__proto__`, `constructor`, and `prototype` as keys in `ValidateLayout` and in the TS `parseLayout` (a paired change). Consider rejecting them in `ToMap`/`ClientView` output generally.

### [P3] Key changes inside array elements are sent as deltas with raw string segments, which the client resolves wrongly
- ID: DP-7
- Category: bug
- Location: internal/services/events/positional.go:29 (the positional map does not descend into arrays), 47-53; internal/repo/game/changes.go:48; client/services/positional-map.ts:36-43
- Confidence: CONFIRMED (Go proof: adding keys `length` and `5` to `data.log[0]` publishes `OpUpdate` with paths `[2,0,0,"5"]` and `[2,0,0,"length"]`. TS proof: `resolvePath` turns those into `data.log.0.f` and `data.log.0.6`)
- Prior: new
- What: The shape check compares positional maps, and those contain only object keys along map-only chains. Adding or removing a key in an object inside an array is therefore not a shape change. Go then emits the key as a string. On the client, `keys[seg]` with a string seg does an array property lookup: a numeric string picks the n-th sorted key, and names like `length` or `map` return an Array property instead of `undefined`. Other strings happen to fall through correctly. PROTOCOL.md says key deletions "always arrive as a keyframe", which is false inside arrays.
- Failure scenario: a game keeps `data.rounds: [{…}]` and adds `"5"` or `"length"` to a round. The client writes the value under the wrong key.
- Suggested fix: extend `BuildPositionalMap` (in Go and TS) into array elements, so key-set changes there also trigger a keyframe. Or have the client treat non-number segments as literal keys (`typeof seg === "string"`).

### [P3] Object keys containing "." corrupt client state and can be wrongly filtered
- ID: DP-8
- Category: bug
- Location: internal/services/events/delta.go:175-181 (`joinPath`), positional.go:39-44 (`strings.Split(path, ".")`), delta.go:132-146 (`pathHasSegment`); client/services/game-state-parser.ts:129-151
- Confidence: CONFIRMED (proof: a change to `data["a.b"]` encodes as `[0,"a","b"]`, and the client state becomes `{"a.b":1,"z":0,"a":{"b":2}}`)
- Prior: new
- What: Paths are dotted strings on both sides, so a key that contains `.` is split into segments that don't exist. Also, a key such as `x.privateData` is treated as a privateData segment and silently never sent. Nothing validates or documents that keys can't contain dots.
- Suggested fix: carry paths as `[]string` segment slices from `Diff` through `EncodePath` (they are already arrays on the wire). On the client, apply the resolved segment arrays directly instead of re-joining and splitting.

### [P3] Go sorts keys by UTF-8 bytes and JS by UTF-16 code units, so sibling positions disagree for some non-ASCII keys
- ID: DP-9
- Category: bug
- Location: internal/services/events/positional.go:23 (`sort.Strings`); client/services/positional-map.ts:36 (`Object.keys(...).sort()`)
- Confidence: CONFIRMED (proof: Go order `Z, a, U+FF01, U+1F600`; JS order `Z, a, U+1F600, U+FF01`)
- Prior: new
- What: The orders differ whenever siblings mix a supplementary-plane character (emoji and similar) with BMP characters in U+E000–U+FFFF (fullwidth forms, CJK compatibility, and so on). ASCII and most BMP text sort the same. When they differ, every delta under those siblings resolves to the wrong key.
- Failure scenario: `data.reactions` keyed by `"😀"` and `"！"`. A delta to one updates the other on the client.
- Suggested fix: sort by UTF-16 code units in Go (compare `utf16.Encode([]rune(k))`), or send the sorted key list for the affected levels. Add a shared Go/TS fixture test (DP-15).

### [P3] Client delta buffer grows without bound between keyframes, and every message re-sorts and replays all of it
- ID: DP-10
- Category: performance
- Location: client/services/game-state-parser.ts:53-61,84-116
- Confidence: CONFIRMED (code). The bench file's own numbers are ~0.5 ms/message at 500 deltas, and my run of the bench took 187 ms for its 500-delta case.
- Prior: new (the bench comment at game-state-parser.bench.node-test.ts:1-11 knowingly defers it)
- What: Deltas are dropped only when a keyframe arrives, and keyframes now come only on shape changes, which ordinary play is designed to avoid. Each `update()` pushes, sorts the whole array, deep-clones the base, and replays every delta. Memory and per-message CPU grow linearly for the life of a game.
- Failure scenario: a game with a server-driven countdown writing once per second accumulates about 3,600 deltas per hour. Every tick then re-sorts and replays all of them on the phone. This is an extrapolation from the bench and I have not measured it.
- Suggested fix: once ordering uses a server sequence (DP-1), apply in-order deltas directly to the base and buffer only out-of-order ones. Or fold deltas older than N into the base.

### [P3] Client never discards deltas or checks the game id when the game changes, and has no reconnect path
- ID: DP-11
- Category: bug
- Location: client/services/message-handler.ts:22,86-89,139-142,184-190; client/services/game-state-parser.ts:39-47
- Confidence: CONFIRMED in code. It is latent, because the reference UI has no leave or switch-game path.
- Prior: new
- What: One `GameStateParser` lives for the whole connection. Every `ChangeEvent` carries the game `id`, but the client ignores it. On a new keyframe, `deleteBefore(updatedAt)` keeps any buffered delta newer than the new game's `updatedAt`, even if it belongs to the previous game. A late delta from the old game (its broadcast resolved sessions before the leave) is also applied. `onClose` only logs, so after any disconnect the UI is frozen on stale state.
- Failure scenario: a client leaves game A (its last delta at 12:00:05) and joins a quiet game B (`updatedAt` 11:50). A's buffered deltas replay onto B's state with B's schema.
- Suggested fix: reset the parser on each keyframe whose `game.id` differs, and drop deltas whose `id` doesn't match the held game. Implement reconnect using `reconnect` + keyframe.

### [P3] An older keyframe can overwrite a newer one, and a missed layout frame is never recovered
- ID: DP-12
- Category: concurrency
- Location: internal/handlers/actions/refresh/handler.go:44-50 (loads the game, then writes it directly); internal/services/boot/monitor.go:60-61 (broadcast keyframe built from a separate load); client/services/game-state-parser.ts:39-47 (`set` accepts any keyframe); client/services/message-handler.ts:148,184-190 (`sv` used only as a discriminator)
- Confidence: PLAUSIBLE (timing-dependent; not reproduced)
- Prior: new (follows from the B #8 fix)
- What: Two goroutines can write keyframes to the same connection: the handler (refresh/reconnect) and the monitor (`OpKeyframe`). If the handler's load is older but its write lands second, `set()` moves the base, the cutoff, and the schema backwards. Deltas encoded against the newer schema then misresolve. Separately, the keyframe's `sv` (layout version) is never compared with the layout the client holds, and `refresh` sends no layout frame. A client that missed an `OpLayout` (for example during the DP-3 window) can't get the current layout back without reconnecting.
- Suggested fix: once keyframes carry a version (DP-1), ignore keyframes older than the one held. On an `sv` mismatch, request or attach the layout frame (for example, have `refresh` send it when `sv` differs).

### [P3] One serial monitor loop fans out all games, and a slow fan-out blocks writers while they hold the game lock
- ID: DP-13
- Category: performance
- Location: internal/services/boot/monitor.go:22-45; internal/services/events/inprocess.go:14-24; internal/repo/game/memory.go:346-353 (and the other stores: `diff` runs inside `mutation.Run`, under the game lock)
- Confidence: PLAUSIBLE (not load-tested)
- Prior: new
- What: Each event costs a session-store `Find("gameId")` (a full scan on SQL backends, which is a known open item). An `OpKeyframe` or `OpLayout` event also costs a game load, and then there is one encode per recipient connection. This all happens on one goroutine for every game. When the 256-slot in-process channel fills, `Publish` blocks inside `Mutate` while the per-game lock is held, so write latency becomes fan-out latency. In Redis mode, the reader stalls instead and Redis may disconnect the subscriber, which triggers DP-2.
- Suggested fix: resolve sessions per game less often (or cache them), fan out per game concurrently, and publish after releasing the mutation lock while keeping per-game order (for example with a per-game sequence, DP-1).

### [P3] `playerData` goes to every player, but the docs say only "keyed per player"
- ID: DP-14
- Category: docs
- Location: docs/ARCHITECTURE.md:219; internal/services/events/clientview.go:8-16
- Confidence: PLAUSIBLE (the intended semantics are unstated)
- Prior: new
- What: `ClientView` strips only `privateData`, so every player's `playerData` at every level is broadcast to everyone. A game author who reads "keyed per player" as "visible per player" (hidden hands, secret roles) would leak them.
- Suggested fix: document it explicitly as public, or implement per-recipient filtering. Per-recipient filtering would need per-session keyframes and deltas.

### [P3] Test gaps around the pipeline's hardest invariants
- ID: DP-15
- Category: tests
- Location: internal/services/events/*_test.go; client/services/*.node-test.ts; internal/services/boot/monitor_test.go
- Confidence: CONFIRMED
- Prior: new
- What: No shared fixture checks that Go `EncodePath` output decodes to the same key with TS `resolvePath` (DP-7, DP-8, and DP-9 would all have been caught). Nothing tests `monitorGameChanges` or `RelayDeliveries` when the subscription channel closes, or Redis resubscription (DP-2). No test covers the join and broadcast ordering (DP-3), or the Lua refresh payload's privacy (DP-4).
- Suggested fix: add a JSON fixture (schema, string path, encoded path) generated by a Go test and asserted by a node test. Add monitor tests with a closable fake bus.

### [P4] Dead and redundant code in the delta path
- ID: DP-16
- Category: maintainability
- Location: internal/services/events/delta.go:84-130 (SanitizeDelta), 199-215 (`DiffDocuments`, unused outside its definition); events.go:18-19 (`OpInsert`/`OpDelete` never emitted); internal/services/game/game.go:73-85 (`GetJSONBytes`, no callers), 168-190 (`Sanitize`, see DP-4); client/services/positional-map.ts:3-20 (`buildPositionalMap` is used only by tests)
- Confidence: CONFIRMED (grep)
- Prior: Review B cleanup "Dead pass-through branch in SanitizeDelta", STILL-OPEN
- What: `SanitizeDelta`'s `[]interface{}` branches are unreachable. Its `privateData` filtering is also redundant, because `Diff` already runs on `ClientView` output, which contains no `privateData`. Its `"version"` metadata key never appears, because `Version` is `json:"-"`. The only effective part is dropping top-level `createdAt`/`updatedAt`.
- Suggested fix: reduce it to a metadata filter (or strip those keys in `ClientView` for deltas), and delete the dead helpers and their pinning tests.

### [P4] In multi-instance mode, deltas are about twice as large on the wire
- ID: DP-17
- Category: performance
- Location: internal/services/events/redis.go:36,75 (JSON round trip of `ChangeEvent`); internal/transport/write.go:33-42
- Confidence: CONFIRMED (proof: the same delta encodes to 31 bytes locally and 63 bytes after the Redis round trip, because integer path segments come back as `float64` and are encoded as 9-byte floats)
- Prior: new
- What: The client decodes both forms the same way, so this is a size and performance difference, not a correctness one.
- Suggested fix: use MessagePack on the Redis bus too, or normalize whole-number floats in paths back to ints after decoding.

### [P4] Docs and small inconsistencies
- ID: DP-18
- Category: docs
- Location: docs/PROTOCOL.md:394-398 (the invariant "keys must never be deleted… breaks decoding" contradicts "Keyframes mid-game" at :363-368; the claim that the client "rebuilds the positional map… using buildPositionalMap" is false); docs/PROTOCOL.md:379 (opcodes 2 and 3 are documented but never sent); client/services/game-state-parser.ts:55 vs 126 (`update()` keeps a delta with `t == cutoff`, but `deleteBefore` drops it)
- Confidence: CONFIRMED
- Prior: new
- Suggested fix: update PROTOCOL.md to the keyframe-on-shape-change model, and make the cutoff comparison consistent (moot if DP-1 switches to sequence numbers).

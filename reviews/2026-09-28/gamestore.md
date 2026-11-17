# Game store / mutation / lock review (slice GS)

Reviewed: `wip/codebase-review` @ 6330d92 (main 3eab208 + review docs). Read-only; no repo files modified.

Verification run:
- `go vet ./internal/repo/... ./internal/services/mutation/ ./internal/services/lock/` — clean.
- `go test -race -count=1 ./internal/repo/game/ ./internal/services/mutation/ ./internal/services/lock/ ./internal/repo/ids/ ./internal/repo/utils/` — all pass, no races.
  - memory, SQLite: ran. **MongoDB: ran** (local Mongo was reachable). **Postgres: skipped** (`INDRI_TEST_POSTGRES_URI` unset).
  - `internal/services/lock` and `internal/repo/utils` have **no test files**.
- The behaviour claims marked "(probed)" were checked with throwaway tests run in a **copy** of the repo under `$TMPDIR/indri-probe` (source: `scratchpad/probe_test.go`), against memory, SQLite and Mongo.

## Prior findings re-check

| Prior | Topic | Status | Evidence |
|---|---|---|---|
| A#1 / B#6 | Shallow `copyGame`, Mutate edits stored state in place | **FIXED** | `memory.go:60-177` deep-copies every map/slice/pointer-to-map. `commit` stores a copy (`memory.go:375`). `TestStore_FailedMutateLeavesNestedStateUnchanged` and `TestStore_GamesShareNoStateWithTheirScript` pass. Residual: values that aren't JSON-shaped are still aliased (GS-6). |
| A#2 | `Exists` looked up by id on memory/SQLite/Postgres | **FIXED** | memory `memory.go:227-232` (codes map), SQLite `sqlite.go:153-162`, Postgres `postgres.go:172-181`, Mongo `mongo.go:105-112` all match on `code`. Pinned by the contract test `TestStore_ExistsChecksTheGameCode` (`store_contract_test.go:220`). |
| A#4 | Mongo `_id` changed from ObjectID to string with no migration | **STILL-OPEN (accepted), impact is wider than recorded** | See GS-1. A legacy document breaks `FindOpen` for **every** game, not just old games (probed). |
| A#6 / B#7 | Memory store publishes while holding `s.mu` | **FIXED** | `memory.go:345-355`: `commit` takes and releases `s.mu`, and `diff` runs afterwards. `TestMemoryStore_PublishesOutsideTheStoreLock` (`memory_test.go:250`). Publishing still happens under the *mutation* lock, on purpose, to keep commit order (`memory.go:348-351`). |
| A#8 | SQLite `Storer` assertion commented out | **FIXED** | `sqlite.go:29` `var _ Storer = (*SQLiteStore)(nil)`. Same for memory `:33` and Postgres `:40`. Mongo's is in `interface.go:7`. |
| B#4 | `assignSlot` never checks whether the user already holds a slot | **FIXED** | `operations.go:137-156` (`slotHeldBy`, keep-or-move, host carried over). Pinned by the `slot_contract_test.go` tests. |
| B#5 | remove/setConnected don't check the slot holder | **FIXED in this slice** | `operations.go:186` (`removePlayer`) and `:207` (`setConnected` → `ErrAbort`). `TestStore_AStaleHolderCannotDisconnectOrRemoveTheNewHolder`. Clearing session GameID/SlotID on leave/kick is outside this slice and still open, per the prior doc. |
| A leftover | `repo.ErrConflict` unused, duplicates `mutation.ErrConflict` | **STILL-OPEN** | `internal/repo/errors.go:11`. `rg ErrConflict` finds no reference outside its own definition. See GS-15. |
| A leftover | `isPgDuplicateKey` / SQLite unique check copied and matched on error text | **CHANGED: SQLite half fixed, Postgres half still open** | SQLite is consolidated into `internal/clients/sqlite/client.go:121 IsUniqueViolation`. `isPgDuplicateKey` still has three copies: `repo/game/postgres.go:81`, `repo/user/postgres.go:51`, `repo/session/postgres.go:56`. All three substring-match `"23505"`. See GS-14. |
| B cleanup | `*_host.go` / `*_player.go` wrappers duplicated across four stores | **STILL-OPEN** | All 8 files are byte-identical once the receiver type is normalized (`diff` returned no difference for every pair). See GS-13. |
| B cleanup | Snapshot-and-diff logic duplicated across four stores | **CHANGED: partly fixed** | The diff and publish logic now lives once in `changes.go`. The Mutate glue (snapshot `before` in load → save → `changes.diff` on commit) is still written four times: `mongo.go:135-169`, `memory.go:322-357`, `sqlite.go:265-296`, `postgres.go:252-283`. See GS-13. |
| Leftover 09-28 | Postgres DDL has indexes that duplicate UNIQUE | **CONFIRMED STILL-OPEN (games table)** | `postgres.go:27` `idx_games_code ON games (code)` duplicates the index that `code TEXT UNIQUE` (`:22`) already creates. See GS-16. |
| Leftover 09-28 | Postgres rejects `\u0000` with an unclear error | **PLAUSIBLE, not run** (no Postgres) | See GS-17. |

---

## Findings (severity-sorted)

### [P2] A legacy Mongo document with an ObjectID `_id` breaks FindOpen (game listing) for every game
- ID: GS-1
- Category: bug
- Location: internal/repo/game/mongo.go:91-93; internal/models/game.go:8
- Confidence: CONFIRMED (probed against the local Mongo `indri_test` DB, which still holds pre-upgrade docs)
- Prior: Review A #4 (accepted/deferred; impact understated)
- What: `FindOpen` decodes the whole cursor into `[]*models.Game`, and `Game.ID` is a `string`. A single document whose `_id` is a BSON ObjectID fails the decode with `error decoding key _id: decoding an object ID into a string is not supported by default`, so the call returns an error instead of the other games.
- Failure scenario: a deployment upgraded from before the string-ID change has one old public game → the `inquire` handler (`inquire/handler.go:74`, `FindOpen(100)`) errors on every call → no client can list open games, including brand-new ones. Probe output: `FindOpen(0) n=0 err=error decoding key _id: ...`.
- Suggested fix: ship the migration or release note A#4 already calls for. At minimum, filter `FindOpen` to string `_id`s (`{"_id": {"$type": "string"}}`) or skip documents that fail to decode.

### [P3] Mongo `saveWithVersion` `$set` cannot clear top-level `omitempty` fields, so the committed delta disagrees with what is stored
- ID: GS-2
- Category: bug
- Location: internal/repo/game/mongo.go:175-194; internal/models/game.go:8,16,18
- Confidence: CONFIRMED (probed)
- Prior: new
- What: the save marshals the game to BSON and `$set`s each top-level key. `teams` (`bson:"teams,omitempty"`) and `deletedAt` (`omitempty`) disappear from the document when they are empty, so `$set` leaves the stored value unchanged. Memory, SQLite and Postgres replace the whole document. Meanwhile `changes.diff` is computed from the in-memory `g` (teams empty), so clients are told the teams are gone while Mongo still holds them.
- Failure scenario: `Update(id, &UpdateGame{Teams: &map[string]Team{}})`, or `DeleteField(id, "teams")`, or a Lua/handler apply that sets `g.Teams = map{}`. Memory/SQLite: `len(teams)=0`. Mongo: `len(teams)=1` (probe). Clients get a keyframe request, rebuild without teams, and the next Get or keyframe brings them back.
- Suggested fix: use `ReplaceOne` with the `{_id, version}` filter, or add `$unset` for keys absent from the marshaled doc.

### [P3] UpdateField / DeleteField on an array index replaces the whole array with an object
- ID: GS-3
- Category: bug
- Location: internal/repo/game/memory.go:254-273 (`applyDottedPath`); internal/repo/game/operations.go:269-303; the doc comment at operations.go:269 and CLAUDE.md both give `"data.board.1"` as the example
- Confidence: CONFIRMED (probed on memory, SQLite and Mongo)
- Prior: new
- What: `applyDottedPath` treats any non-map intermediate value as missing and overwrites it with a fresh map. Arrays are never indexed.
- Failure scenario: `data.arr = ["a","b","c"]`, then `UpdateField(id, "data.arr.1", "Z")` → stored `data.arr = {"1":"Z"}`. `a` and `c` are lost, and the type changes (probe: memory/SQLite `map[string]interface{}{"1":"Z"}`, Mongo `bson.D{{"1","Z"}}`). The shape change triggers a keyframe, so every client sees the corruption. Today UpdateField/DeleteField have no production caller (only the unimported `internal/services/stage`), but CLAUDE.md recommends UpdateField to game authors, and uses exactly this path shape as its example.
- Suggested fix: when the current value is a `[]interface{}` and the segment is an integer, index into it (and bounds-check); otherwise return an error instead of silently replacing a non-map value.

### [P3] Delta paths are ambiguous when a client-visible map key contains `.` — the client updates the wrong field
- ID: GS-4
- Category: bug
- Location: internal/repo/game/changes.go:46,53-76 (caller); the root cause is in internal/services/events/positional.go:38-68 and BuildPositionalMap (events slice)
- Confidence: CONFIRMED (probed on memory)
- Prior: new
- What: `Diff` produces dotted string paths, and `BuildPositionalMap` / `EncodePath` join and split on `.`. A key `"a.b"` collides with nested `a → b` in the positional map, and the last writer wins.
- Failure scenario: `data = {"a.b": 1, "a": {"b": 2, "c": 5}}`. A write changes `data["a.b"]` to 3, and the published delta is `[[2,0,1], 3]`, which a client decodes as `data.a.c = 3`. The client shows `a.c = 3` while the server holds `a.c = 5`. The shape is unchanged, so no keyframe corrects it. Keys come from game data (Lua scripts, handlers, and potentially player-supplied names or answers used as map keys).
- Suggested fix: carry paths as `[]string` segments end to end (Diff → Sanitize → Encode), or reject/escape `.` in data keys at write time. Coordinate with the events reviewer.

### [P3] Mongo game store never maps not-found to `repo.ErrNotFound`, and its errors are unwrapped
- ID: GS-5
- Category: bug / maintainability
- Location: internal/repo/game/mongo.go:81-93 (Get, FindByCode, FindOpen), :106-111, :143-146; internal/repo/errors.go:1-3 ("Callers compare with errors.Is; they never inspect driver types")
- Confidence: CONFIRMED (probed)
- Prior: new
- What: memory, SQLite and Postgres return `fmt.Errorf("id %q: %w", id, repo.ErrNotFound)`. Mongo returns the raw `mongo: no documents in result`. Mongo errors also get no lowercase `%w` context, against the project convention.
- Failure scenario: `errors.Is(store.Get("missing"), repo.ErrNotFound)` is `true` on memory/SQLite and `false` on Mongo (probe). No production caller checks it today, so the risk is latent: the first handler that branches on ErrNotFound (for example "game gone → clear session") works in tests on the memory store and misbehaves on the default backend. The per-store `*_Get_UnknownID_ReturnsErrNotFound` tests exist for memory, SQLite and Postgres but not Mongo, and the contract suite doesn't cover it.
- Suggested fix: map `mongo.ErrNoDocuments` to `repo.ErrNotFound` in Get/FindByCode, wrap the other errors, and move the not-found test into `eachStore`.

### [P3] Memory store keeps non-JSON value types: stored state is aliased and differs in type from other stores
- ID: GS-6
- Category: bug / concurrency
- Location: internal/repo/game/memory.go:161-177 (`deepCopyValue`), :375
- Confidence: CONFIRMED by reading (tictactoe writes a `[][]string` board: example/tictactoe/server/handlers/move/handler.go:148, restart/handler.go:21-26)
- Prior: related to A#1/B#6 (residual)
- What: `deepCopyValue` copies only `map[string]interface{}` and `[]interface{}`. Anything else an apply stores (`[][]string`, `[]string`, `map[string]string`, structs, pointers) is copied by reference into stored state and into every copy that `Get` returns. SQL stores normalize through JSON (`[]interface{}`), and Mongo yields `bson.A`/`bson.D`.
- Failure scenario: (a) aliasing: after a tictactoe move the memory store holds the handler's `[][]string`. Any code that type-asserts it from a `Get` result and writes a cell outside `Mutate` edits the stored game with no version bump and no delta, racing concurrent `copyGame` readers. (b) divergence: `g...["board"].([][]string)` succeeds on memory and fails on SQLite/Postgres/Mongo, so handler tests on the memory store (handlertest uses memory, `handlertest.go:115`) can pass while production fails.
- Suggested fix: normalize in `commit` through the JSON form (`events.ToMap` → `fromMap`, keeping `Version`/`DeletedAt` as `editPath` does), so memory behaves like the SQL stores.

### [P3] Mongo data is decoded as `bson.D`/`bson.A`, so every store hands apply functions different Go types
- ID: GS-7
- Category: maintainability / bug-prone
- Location: internal/repo/game/mongo.go:81-83 (no decode normalization); workarounds at example/tictactoe/server/board/list.go:6-9, internal/handlers/actions/layout/handler.go:56-58, internal/services/events/clientview.go:19-20
- Confidence: CONFIRMED (probed: Mongo `nested type=bson.D arr type=bson.A`; memory/SQLite `map[string]interface{}` / `[]interface{}`)
- Prior: new
- What: the storage backend leaks into game logic. Each handler has to rediscover this and code around it (reflection helpers, ToMap round trips). A plain `.(map[string]interface{})` or `.([]interface{})` on nested data works on three stores and fails on the default one.
- Failure scenario: a game author follows the memory-store tests, writes `g.PublicData["scores"].(map[string]interface{})`, and it fails with "interface conversion" (recovered as a handler error) on MongoDB only.
- Suggested fix: normalize in the Mongo load path (JSON round trip after decode, the same form the SQL stores produce), and delete the per-handler workarounds.

### [P3] Writes that change `code` or `id` diverge across stores (SQLite and memory keep stale indexes, SQL saves by `g.ID`)
- ID: GS-8
- Category: bug
- Location: internal/repo/game/sqlite.go:207-209 (never updates the `code` column; WHERE uses `g.ID`), postgres.go:222-225 (updates `code`; WHERE uses `g.ID`), memory.go:362-376 (never updates `s.codes`), mongo.go:184-187 (WHERE uses the `id` argument)
- Confidence: CONFIRMED (probed: after `UpdateField(id,"code",new)`, `FindByCode(new)` fails on memory and SQLite and succeeds on Mongo; `FindByCode(old)` still succeeds on memory and SQLite)
- Prior: new
- What: nothing stops an apply or UpdateField from editing `code` or `id` (`editPath` rewrites any top-level JSON key). Each store then indexes the game differently. If `g.ID` changes, SQLite and Postgres match 0 rows, report "not committed", and `mutation.Run` spins 10 attempts before returning a misleading `ErrConflict`. Mongo writes the right row. Memory stores the game under the old key with a new `g.ID`.
- Failure scenario: `UpdateField(id, "code", "NEWCODE")` → on SQLite, `join NEWCODE` says not found and `join OLDCODE` loads a game whose blob says NEWCODE. On Postgres the same call succeeds, or fails on the UNIQUE constraint with a non-ErrDuplicate error.
- Suggested fix: treat `id` and `code` as immutable in the shared save path. Restore them from the loaded game after apply (as `editPath` does for Version/DeletedAt), or reject the write, and use the `id` argument in every WHERE.

### [P3] Delta ordering and cutoff rely on each instance's wall clock; in multi-instance mode a skewed clock shows stale values or drops deltas
- ID: GS-9
- Category: concurrency
- Location: internal/repo/game/changes.go:42,49,81 (`Timestamp: time.Now()`); consumer at client/services/game-state-parser.ts:49-59,122-126
- Confidence: PLAUSIBLE (mechanism confirmed by reading; the impact needs clock skew between instances). Confirm by running two instances with skewed clocks, or with a faked `time.Now`.
- Prior: new
- What: the per-game lock does make commits and publishes happen in order, but the client re-sorts deltas by `t` and drops anything older than the keyframe cutoff. `t` is taken from whichever instance committed the write.
- Failure scenario: instance B's clock runs 50 ms behind A's. W1 (A, `board[0][0]="X"`, t=1000) then W2 (B, `board[0][0]=""` after a restart, t=990). The client sorts W2 before W1 and ends with "X" while the server holds "". The same skew against a keyframe's cutoff silently discards a committed delta.
- Suggested fix: order by the game's `Version` (already bumped once per commit, under the fence). Publish it in the event and the keyframe, and have the client order and cut off on it instead of wall time.

### [P3] Lock waits and apply are unbounded: no request context, InProcess ignores cancellation, and Redis polling has no fairness
- ID: GS-10
- Category: concurrency
- Location: internal/services/lock/inprocess.go:26-43 (ctx only checked before `entry.mu.Lock()`); lock.go:8-9 (contract says "blocks until the lock is held or ctx is cancelled"); redis.go:47-62 (fixed 25 ms poll, no jitter, no deadline); all four `Mutate`s pass the store's root ctx (e.g. memory.go:326, mongo.go:139); interface.go (Storer methods take no ctx)
- Confidence: CONFIRMED (InProcess ignoring ctx, and there being no per-call ctx); PLAUSIBLE (Redis starvation under contention, which a load test on one hot game would show)
- Prior: new
- What: a caller can wait on a game lock indefinitely. InProcess breaks its own interface contract, and a hung apply (for example a looping Lua handler script) wedges that game forever in single-instance mode, because InProcess has no lease. With Redis the lease runs out after 10 s. On Redis, many waiters poll `SETNX` every 25 ms in lockstep with no queueing, so an unlucky waiter can lose indefinitely on a busy game.
- Failure scenario: an operator's Lua handler has an infinite loop in one branch → the first player to hit it holds `game:<id>` forever → every later join/move/disconnect for that game blocks its goroutine permanently. Nothing times out, because the ctx is the process root.
- Suggested fix: thread a request ctx (with a deadline) through `Mutate`/`mutation.Run`. Make `InProcess.Acquire` honor ctx (a channel-based semaphore). Add jitter and backoff to the Redis poll.

### [P3] `update()` always overwrites `Private`, so a partial Update makes a private game public
- ID: GS-11
- Category: bug (latent)
- Location: internal/repo/game/operations.go:243-266 (line 264)
- Confidence: CONFIRMED by reading; latent, because `GameService.Update` (`internal/services/game/game.go:154`) has no caller
- Prior: related to Sep-9 #f624 (the fix for "can't set private:false" made the field unconditional)
- What: `UpdateGame.Private` is a plain `bool`, so "not set" and "false" can't be told apart. Every Update rewrites it.
- Failure scenario: `Update(id, &UpdateGame{PublicData: x})` on a game created with `private:true` → `Private=false` → it appears in `FindOpen`/`inquire` for everyone.
- Suggested fix: make it `*bool` and copy it only when non-nil, or drop `Update` if it stays unused.

### [P4] `FindOpen` limit semantics differ per store, and memory panics on a negative limit
- ID: GS-12
- Category: bug (latent)
- Location: internal/repo/game/memory.go:234-248; sqlite.go:164-167; postgres.go:183-186; mongo.go:91-93
- Confidence: CONFIRMED (probed)
- Prior: new
- What: `FindOpen(0)` returns 1 game on memory (it appends before checking the limit), 0 on SQLite, and all games on Mongo (`Limit(0)` means unlimited). `FindOpen(-1)` panics on memory (`makeslice: cap out of range`) and returns all games on SQLite. None of the stores defines an order. The only caller passes a constant 100 (`inquire/handler.go:74`), so this is latent.
- Suggested fix: validate `limit > 0` once in the service, add `ORDER BY`/sort for deterministic listings, and add `FindOpen` to the contract suite.

### [P4] Duplication across the four stores: wrappers and Mutate glue
- ID: GS-13
- Category: maintainability
- Location: internal/repo/game/{mongo,memory,sqlite,postgres}_{host,player}.go (8 identical files); the Mutate bodies at mongo.go:135-169, memory.go:322-357, sqlite.go:265-296, postgres.go:252-283; the Update/UpdateField/DeleteField/GetIDHex one-liners in each store
- Confidence: CONFIRMED
- Prior: Review B cleanup (wrappers: STILL-OPEN; snapshot/diff: partly fixed)
- What: each backend repeats the same 8 host/player methods, 3 field methods, GetIDHex, and the snapshot → save → publish Mutate wrapper. Only `load(id)` and `saveIfVersion(g, v)` actually differ.
- Failure scenario: a fix to publish-on-commit, or to `before` snapshotting, has to be made four times. GS-8 shows the stores have already drifted (id vs g.ID, code column).
- Suggested fix: a shared `gameOps` type that embeds a small `backend` interface (`load`, `save`, `insert`, lookups) and implements Mutate and all the shared methods once, with each store supplying only the backend.

### [P4] `isPgDuplicateKey` has three copies and matches `"23505"` anywhere in the error text
- ID: GS-14
- Category: maintainability
- Location: internal/repo/game/postgres.go:79-89; internal/repo/user/postgres.go:51; internal/repo/session/postgres.go:56
- Confidence: CONFIRMED
- Prior: Review A leftover (STILL-OPEN; the SQLite half was moved to `clients/sqlite.IsUniqueViolation`)
- What / Failure scenario: any Postgres error whose message happens to contain "23505" (for example a value echoed in an error) is reported as `ErrDuplicate`.
- Suggested fix: add `postgres.IsUniqueViolation` in `internal/clients/postgres`, mirroring SQLite, and use `errors.As(err, *pgconn.PgError)` with `Code == "23505"`.

### [P4] `repo.ErrConflict` is dead and duplicates `mutation.ErrConflict`
- ID: GS-15
- Category: maintainability
- Location: internal/repo/errors.go:11; internal/services/mutation/mutation.go:23
- Confidence: CONFIRMED (no references)
- Prior: Review A leftover (STILL-OPEN)
- Suggested fix: delete it, or have `mutation.ErrConflict` wrap it so callers have one sentinel.

### [P4] Postgres games DDL creates a redundant index on `code`
- ID: GS-16
- Category: performance
- Location: internal/repo/game/postgres.go:22,27
- Confidence: CONFIRMED
- Prior: 09-28 leftover (was "not verified"; now verified for games)
- What: `code TEXT UNIQUE` already creates a unique index, so `idx_games_code` doubles the write cost for no gain.
- Suggested fix: drop `idx_games_code`. `idx_games_private` is only useful if FindOpen gets an ORDER BY / partial index.

### [P4] Postgres JSONB rejects `\u0000` in game data
- ID: GS-17
- Category: bug
- Location: internal/repo/game/postgres.go:215-229; display names flow in via operations.go:159
- Confidence: PLAUSIBLE (Postgres not available here; JSONB is known to reject `\u0000`). Confirm with `INDRI_TEST_POSTGRES_URI` set, by calling `AssignSlot(..., "a\u0000b")`.
- Prior: 09-28 leftover
- Failure scenario: a player whose display name contains NUL can't join on Postgres ("unsupported Unicode escape sequence"), but can on every other store. The damage is limited to that one write.
- Suggested fix: strip or reject control characters in display names and client-supplied data at the edge.

### [P4] Test gaps in the concurrency core
- ID: GS-18
- Category: tests
- Location: internal/services/lock/ (no tests); internal/services/mutation/mutation_test.go (3 tests); internal/repo/game/store_contract_test.go
- Confidence: CONFIRMED
- Prior: new
- What: there is no test for the Redis token-checked release (an expired holder must not delete a new holder's lock), for InProcess map cleanup or ctx behaviour, for `mutation.Run` returning `ErrConflict` after 10 failed fences, for load/save error propagation, or for Acquire failure. The contract suite doesn't cover ErrNotFound, FindOpen, Update with empty maps, or UpdateField on arrays, which is why GS-2, GS-3, GS-5 and GS-12 went unnoticed. Postgres runs nowhere locally, and whether CI does is unknown per the prior doc.
- Suggested fix: add `lock` tests (miniredis or a real Redis with skip), `mutation.Run` retry-exhaustion and error tests, and move the per-store Get/FindOpen/Update tests into `eachStore`.

### [P4] Small cleanups
- ID: GS-19
- Category: maintainability
- Location / What:
  - `mongo.go:210-212` `getBsonDocForID` is dead (no references).
  - `mongo.go:26,52` stores `ctx *context.Context` (a pointer to an interface), unlike the other stores.
  - `mongo.go:90` comment "FindOpen retrieves game data by its game code" is wrong.
  - `repo/utils/bson.go:6` comment refers to "UpdateUser". `CreateBSONDoc` doesn't wrap its errors and has no tests.
  - `operations.go:177,198` call `ValidateGameAndUser(id, slotId)`, so an empty slot id reports "userId is required". `assignSlot` doesn't validate an empty `userId`, while its siblings do. An empty userId would seat a ghost (possibly host) whose slot still looks empty.
  - Mongo `New` accepts a nil script (probed: `err=<nil>`), while memory, SQLite and Postgres reject it.
  - `mutation.Run` (`mutation.go:44-68`) returns Acquire/load/save errors without context, and `defer _ = handle.Release(ctx)` silently drops release failures. It uses the same possibly-cancelled root ctx, so a release during shutdown leaves the Redis key for its full 10 s TTL.
  - `redis.go:88` compares `err != redis.Nil` with `!=` rather than `errors.Is`.
  - `models/scene.go` uses `bson:"private_data"` while every other model uses `privateData`. This is harmless to clients (ClientView works on the JSON form) but inconsistent in stored documents.
  - The Storer methods `HasHost`, `PlayerIsHost`, `UnsetHost`, `SetPlayerAsHost` and `Update` have no production callers. `internal/services/stage` (the only UpdateField/DeleteField caller) is not imported anywhere, which is for the services reviewer. In it, `LoadSceneFromScript` writes `"stage.scene."` instead of `"stage.scenes."`, which `fromMap` silently drops.
- Confidence: CONFIRMED
- Prior: new
- Suggested fix: delete the dead code, fix the comments, and wrap the errors per the conventions.

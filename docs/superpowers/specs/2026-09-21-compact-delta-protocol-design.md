# Compact Delta Protocol Design

**Date:** 2026-09-21  
**Status:** Approved

## Problem

The current WebSocket delta protocol is verbose and does not scale well to larger, more dynamic game boards. Key inefficiencies:

- Every delta repeats a 24-char game `id` and `type` field the client already knows
- Timestamps are ISO-8601 strings (35 chars) instead of integers
- `updatedAt`, `createdAt`, and `version` metadata fields appear in every delta as changed paths
- When any array element changes, the entire array is replaced wholesale
- Long dotted-path strings (e.g. `stage.scenes.board.data.board.1.1` = 35 chars) dominate delta payloads
- Keyframes bundle static layout/script data (~1500 bytes in tic-tac-toe) with dynamic game state on every reconnect and refresh
- Wire encoding is plain JSON with no binary option

## Goals

- Significantly reduce delta and keyframe payload sizes to support larger boards and more dynamic game state
- Maintain human-readable JSON as a debug mode
- Require no external diff-format library dependency

## Non-Goals

- Compression of client→server messages (player moves are small)
- Backward compatibility with the current delta format (clean break)

---

## Design

### Section 1: Compact delta envelope

**Current shape (~295 bytes for a board move):**
```json
{"id":"6ab1994cce7f70e07f05a210","op":"update","ts":"2026-09-21T15:53:56.082127-05:00","type":"game","updated":{...},"removed":[...]}
```

**New shape:**
```json
{"o":1,"t":1758556436082,"u":[[path,value],...],"r":[path,...]}
```

Changes:
- Drop `id` — client knows its game
- Drop `type` — always `game`; add back only when a multi-collection protocol is needed
- Rename `op`→`o`, `ts`→`t`, `updated`→`u`, `removed`→`r`
- Opcode `o` is a `uint8` integer, not a string (see opcode table below)
- Timestamp `t` is int64 Unix milliseconds — saves 22+ bytes vs ISO-8601 string, and 27 bytes in MessagePack
- Strip `updatedAt`, `createdAt`, and `version` from the `u` field server-side — these are document metadata the client never uses and they appear in every delta today
- `u` changes from a `map[string]interface{}` to an array of `[path, value]` pairs — avoids JSON key-type constraints and enables positional integer-array paths (Section 5)
- `r` changes from `[]string` to an array of paths (same encoding as `u` paths)

**Opcode table** (shared constants in Go and TypeScript):

| Value | Meaning |
|---|---|
| `1` | update |
| `2` | insert |
| `3` | delete |
| `4` | layout |

**Struct changes:** `ChangeEvent.UpdatedFields` changes from `map[string]interface{}` to `[][]interface{}` (a slice of two-element slices, not `[][2]interface{}` — fixed-length Go arrays do not round-trip through `encoding/json` on unmarshal because `json.Unmarshal` into `interface{}` always produces `[]interface{}`, breaking type assertions on the subscriber side). `RemovedFields` changes from `[]string` to `[]interface{}`. Use-sites must assert `len(pair) == 2` before accessing `pair[0]` and `pair[1]`. This change propagates to:
- The `Publisher` interface (both in-process and Redis implementations)
- `SanitizeDelta` — signature and implementation
- `HasChanges` — length checks on new types
- The Redis transport — `ChangeEvent` is serialized to JSON for pub/sub; the subscriber unmarshals into `ChangeEvent` directly; with `[][]interface{}` the round-trip is correct

The client message classifier changes from `op === "update"` to `o === 1`.

---

### Section 2: Array index diffing

**Current:** any element change replaces the entire array.

```
u: [["stage.scenes.board.data.board", [["","",""],["","X",""],["","O",""]]]]
```

**New:** `diffInto` recurses into arrays and emits per-index paths.

```
u: [["stage.scenes.board.data.board.1.1", "X"], ["stage.scenes.board.data.board.2.1", "O"]]
```

(In normal mode, string paths become integer-array paths per Section 5.)

For a 20×20 board with one cell changed: current ~800 bytes (whole array), new ~12 bytes (one indexed path). The crossover where whole-array replacement is cheaper than indexed paths is ~50% of cells changing simultaneously — this never occurs in practice for game moves.

**Diffing logic** in `diffInto` gains one new branch alongside the existing map/scalar branches:

- If both values are `[]interface{}`: call `diffSlice(path, before, after, ...)`
- `diffSlice` walks index-by-index: if element is a map on both sides, recurse into `diffInto`; if scalar or mismatched types, compare with `reflect.DeepEqual` and emit if different
- Indices only in `before` → emit removed paths (`path.0`, `path.1`, ...)
- Indices only in `after` → emit updated paths

**Client-side:** `GameStateParser` requires changes to support both positional integer-array paths (normal mode) and numeric-dotted string paths (debug mode). The existing `updateJSONKeyByDotPath` handles *array container creation* for numeric next-segments but does not resolve positional integer indices to string keys. A new resolution step is needed: before walking a path, map each integer segment through the positional schema map built from the keyframe to recover the string key. The parser must also handle writing to array slots when the current container is an array and the segment is a numeric index.

---

### Section 3: Static/dynamic keyframe split

The tic-tac-toe keyframe is ~2200 bytes; ~1500 bytes are `data.layout` (Lua scripts, widget configs from `config.json`) which never changes during a game session.

**New protocol: layout frame + slim keyframe**

On `join`, `create`, and `reconnect`, the server sends two messages in order:

1. **Layout frame** (new message type, `o: 4`, sent once per connection):
```json
{"o":4,"v":"<sha256-of-layout>","data":{...layout...}}
```

2. **Slim keyframe** (game state, `data.layout` omitted, wrapped with `sv`):
```json
{"sv":"<schema-hash>","game":{"id":"...","code":"TAA","teams":{...},"players":{...},"stage":{...},"data":{}}}
```

The presence of the top-level `"sv"` field is the client's discriminator for this message type. The `messageType()` classifier checks `"sv" in parsedMessage` to identify slim keyframe wrappers — this takes priority over the existing `"code" in parsedMessage && "id" in parsedMessage` keyframe check, which no longer applies since those fields are now nested under `.game`.

On `refresh` (re-sends state while still connected), only the slim keyframe wrapper is sent — layout is already cached client-side from the original join.

**`v` vs `sv` are independent hashes:**
- `v` hashes the layout content. A new layout frame invalidates the cached layout but does not affect the positional schema (layout keys are absent from the slim keyframe and therefore absent from the positional map).
- `sv` hashes the sorted key structure of the slim keyframe. It changes only when the schema of the dynamic game state changes (e.g. server restart with a new script that adds or removes a game-state field).
- Because `data.layout` is stripped before the slim keyframe is written, layout key changes never affect `sv`. The two hashes are fully independent.

If a client receives a layout frame with a matching `v`, it is a no-op. If it receives a slim keyframe with an `sv` it has not seen, it re-derives the positional map from the new keyframe. The ordering guarantee: the server always sends the layout frame before the slim keyframe; the client must not derive the positional map until the slim keyframe arrives.

**`sv` field placement:** `sv` is added to a new response wrapper struct (not to `models.Game`) so it does not become a field in the game's JSON representation and does not affect index positions in the positional map.

**Server:** a helper at boot extracts and hashes the layout from `i.Script` and holds it on the injector. The keyframe write path strips `data.layout` and wraps the result with `sv` before writing to the connection.

**Client:** the `messageType()` classifier is rewritten to use integer field `o` instead of string field `op`, and the classifier priority order becomes:
1. `"sv" in parsedMessage` → slim keyframe wrapper
2. `o === 4` → layout frame
3. `o === 1` → delta update
4. `o === "inquiryResponse"` (or equivalent) → inquiry response
5. `authenticated === true` → auth payload
6. `disconnected === true` → disconnect

`message-handler.ts` handles `o === 4` by storing layout keyed by hash. When a slim keyframe wrapper arrives (`"sv" in parsedMessage`), the client re-derives the positional map if `sv` is new, then merges the cached layout into `.game.data.layout` before passing state to `GameStateParser`.

After this change: typical keyframe ~400 bytes (down from ~2200). Layout sent once per connection (~1500 bytes), never again.

---

### Section 4: Binary transport (MessagePack + debug mode)

All server→client messages are encoded as MessagePack by default. Client→server messages remain JSON (player moves are small).

**Library choices** (exact module paths to be verified during implementation):
- Go: a MessagePack library with struct-tag support (e.g. in the `vmihailab` or `vmihaiela` family — verify the correct import path before use)
- TypeScript: `@msgpack/msgpack`

**Per-value savings in MessagePack vs JSON:**

| Type | JSON | MessagePack |
|---|---|---|
| boolean `true` | 4 bytes | 1 byte |
| integer `0`–`127` | 1–3 bytes | 1 byte |
| int64 timestamp | 13 bytes | 9 bytes |
| string key `"u"` | 3 bytes | 2 bytes |

**Debug mode:** the WebSocket upgrade URL accepts `?debug=1`. The Melody connection handler reads this at upgrade time and stores a boolean on the connection (`s.Set("debug", true)`). The broadcast and direct-write helpers check this flag and fall back to plain JSON. All handler code is unaffected — the encoding decision lives entirely in the write path.

| Direction | Default | Debug |
|---|---|---|
| Server → client | MessagePack | JSON (`?debug=1`) |
| Client → server | JSON | JSON |

---

### Section 5: Positional path encoding + player slots

#### Full schema declaration

The game state shape is fully determined before a game starts, derived from `config.json`. **Every field that a delta could ever set must be present in the initial game state with a default value.** The server validates this at boot: if a field is absent from the config-produced game state, it can never appear in a delta path without breaking the positional encoding.

Example gap in current tic-tac-toe config — `winningTeam` is referenced in the Lua script but not declared in `stage.scenes.board.data`. This must be added:

```json
"stage": {
  "scenes": {
    "board": {
      "data": {
        "board": [["","",""],["","",""],["","",""]],
        "winningTeam": null
      }
    }
  }
}
```

The server asserts at boot that the config produces a complete, consistent game shape and fails loudly if not.

#### Player slots

The current model keys the `players` map by MongoDB ObjectID — these are unknown at script-definition time and would break positional encoding when a new player joins mid-game.

**New model:** player slots are pre-declared at game creation. Total slots = number of declared teams × `config.maxPlayersPerTeam`. For tic-tac-toe (2 teams × 1 player): slots `p0`, `p1`.

Game state `players` map at creation:
```json
"players": {
  "p0": {"userId":"","name":"","score":0,"connected":false,"host":false,"controller":false},
  "p1": {"userId":"","name":"","score":0,"connected":false,"host":false,"controller":false}
}
```

The `userId` field is retained on slot records (empty string when the slot is unfilled) so that the `kick` handler can resolve `slotId → userId → Melody connection` without a new service method. `userId` is included in sanitized keyframes and deltas so the client can associate display identity with a slot.

Teams reference slot IDs in `playerIds`: `["p0"]`, `["p1"]`. When a player leaves, their slot is reset to default values; the slot key remains so the schema is stable.

**Authorization migration:** the current authorization pattern resolves the acting player via `sessionId → session.UserID → game.Players[userID]`. With slots:
- `models.Session` gains a `SlotID string` field
- Slot assignment happens in the `join`/`create` handlers: the first available slot in the requested team is claimed and written to `session.SlotID`
- All handler player lookups change from `game.Players[session.UserID]` to `game.Players[session.SlotID]`
- The `kick` action takes a `slotId` parameter instead of `userId`. To force-disconnect the target's Melody connection, the `kick` handler needs to resolve `slotId → userId → session`. Player slot records therefore retain a `UserID` field alongside the existing slot data. The `kick` handler reads `game.Players[slotId].UserID` and then calls `SessionService.GetByUserID(userID)` as today.
- On reconnect the session already holds `SlotID`, so the player reclaims their slot automatically

The server validates at boot that `config.maxTeams` matches the number of entries in the `teams` object.

#### Positional path encoding

At join time, both server and client independently walk the **slim keyframe** (after `data.layout` is stripped), sort each object's keys alphanumerically (case-sensitive, standard Go/JS string sort), and build the same positional map — no extra wire cost. The slim keyframe is the schema.

**Sanitized-out fields excluded:** fields that `Sanitize` strips before the client sees the keyframe (e.g. `privateData`) are not present in the slim keyframe and therefore not in the positional map. The positional map is derived from exactly what the client receives — no special exclusion logic is needed.

**Path encoding format:**

In normal mode, delta paths are integer arrays. In JSON debug mode, paths are numeric-dotted strings. Both encode the same positional sequence.

```
Normal (MessagePack):  u: [[[2,4,0,1,0,1,1], "X"]]
Debug (JSON):          "u": [["2.4.0.1.0.1.1", "X"]]
```

Note: the concrete indices above are illustrative only. Actual indices depend on the full sorted key list of the sanitized game document, which must be derived at implementation time from the real `models.Game` JSON output after sanitization.

In MessagePack, a 7-element fixarray of small integers is **8 bytes** vs a 35-byte dotted-path string. For a delta with 3 changed paths that is ~80 bytes of key savings.

**Array index vs object key disambiguation:** when resolving a path segment, the client checks the type of the current container:
- If the current container is an **object**, look up the integer segment in the positional map to get the string key, then access `container[stringKey]`
- If the current container is an **array**, treat the integer segment as a raw numeric index and access `container[segment]` directly — no positional map lookup

This rule is unambiguous because the schema is fixed and the container type at each path depth is deterministic from the game shape. Array lengths are frozen by the schema contract — delta paths into an array never reference an index that was not present at game creation.

**`GameStateParser` changes:** the parser receives a positional map (built from the slim keyframe) and uses it to resolve integer path segments to string keys before walking the game state, using the container-type rule above. In JSON debug mode, numeric-dotted strings are resolved the same way. The existing `updateJSONKeyByDotPath` structure is preserved but augmented with the resolution step and the container-type check.

---

## Config.json contract

Every game `config.json` must satisfy:

1. `config.maxTeams` equals the number of entries in the top-level `teams` object
2. `config.maxPlayersPerTeam` × number of teams = total player slots pre-declared at game creation
3. Every field referenced by game logic (Lua scripts, server handlers) must be present in its initial section with a default value
4. No new keys are added to the game state after creation

The server enforces rules 1–3 at boot and panics with a descriptive message if violated.

---

## File changes summary

### Server (Go)
| File | Change |
|---|---|
| `internal/services/events/events.go` | `OpCode uint8` type + constants; `ChangeEvent` fields `UpdatedFields` → `[][2]interface{}`, `RemovedFields` → `[]interface{}`, short json tags |
| `internal/services/events/delta.go` | `diffInto` gains array-index branch; `SanitizeDelta` strips metadata paths and adapts to new pair-array types; `HasChanges` updated; `Diff` returns pair-array |
| `internal/services/events/inprocess.go` | Adapt to new `ChangeEvent` field types |
| `internal/services/events/redis.go` | Adapt serialization/deserialization to new `ChangeEvent` shape |
| `internal/clients/melody/client.go` | Detect `?debug=1` at upgrade; encode outbound as MessagePack or JSON |
| `internal/services/game/service.go` | `Sanitize` strips `data.layout`; keyframe write path wraps result with `sv`; layout frame sent before slim keyframe on join/create/reconnect |
| `internal/models/script.go` | `Script.Config` gains `MaxPlayersPerTeam`; validated against `Teams` count at boot |
| `internal/models/session.go` | `Session`, `CreateSession`, and `UpdateSession` all gain `SlotID string` field — without `UpdateSession` carrying `SlotID` the slot is never persisted and reconnect breaks |
| `internal/repo/game/game.go` | `New` pre-declares player slots; new keyframe response wrapper struct with `sv` field |
| `internal/repo/game/player.go` | Player lookups change from `userId` key to `slotId` key; slot assignment on join |
| `internal/repo/game/team.go` | `playerIds` references use slot IDs |
| `internal/repo/game/host.go` | Host resolution uses slot ID from session |
| `internal/handlers/actions/join/handler.go` | Assign slot ID to session on join |
| `internal/handlers/actions/create/handler.go` | Assign slot ID to session on create |
| `internal/handlers/actions/kick/handler.go` | Accept `slotId` parameter instead of `userId` |
| `example/tictactoe/server/handlers/*/handler.go` | All action handlers: update player lookups to use slot ID |
| `boot/boot.go` | Boot-time schema validation: complete field coverage + config consistency |

### Client (TypeScript)
| File | Change |
|---|---|
| `client/models/models.ts` | Update `UpdateMessage`: `o: number`, `t: number`, `u` and `r` as pair arrays; add slim keyframe wrapper type with `sv` |
| `client/services/message-handler.ts` | Decode MessagePack; handle `o === 4` layout frame; detect slim keyframe wrapper; re-derive positional map on new `sv`; merge layout |
| `client/services/game-state-parser.ts` | Accept positional map; resolve integer-array or numeric-dotted-string paths through positional map before walking game state |

### Docs
| File | Change |
|---|---|
| `docs/PROTOCOL.md` | Document new delta envelope, opcode table, layout frame, slim keyframe wrapper, positional paths, debug mode, player slot model |
| `example/tictactoe/config.json` | Add `winningTeam: null` to `stage.scenes.board.data` |

---

## Size comparison (tic-tac-toe board move)

| Message | Before | After (MessagePack) | Reduction |
|---|---|---|---|
| Delta (1 cell change) | ~295 bytes | ~25 bytes | ~91% |
| Keyframe (join) | ~2200 bytes | ~500 bytes (layout once + slim keyframe) | ~77% |
| Keyframe (refresh) | ~2200 bytes | ~150 bytes (slim keyframe only) | ~93% |

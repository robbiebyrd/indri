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
{"o":1,"t":1758556436082,"u":[...],"r":[...]}
```

Changes:
- Drop `id` — client knows its game
- Drop `type` — always `game`; add back only when a multi-collection protocol is needed
- Rename `op`→`o`, `ts`→`t`, `updated`→`u`, `removed`→`r`
- Opcode `o` is a `uint8` integer, not a string (see opcode table below)
- Timestamp `t` is int64 Unix milliseconds — saves 22+ bytes vs ISO-8601 string, and 27 bytes in MessagePack
- Strip `updatedAt`, `createdAt`, and `version` from the `u` map server-side — these are document metadata the client never uses and they appear in every delta today

**Opcode table** (shared constants in Go and TypeScript):

| Value | Meaning |
|---|---|
| `1` | update |
| `2` | insert |
| `3` | delete |
| `4` | layout |
| `5` | keydict (schema) |
| `6` | addkeys |

`ChangeEvent` on the Go side gets new json tags (`json:"o"`, `json:"t"`, etc.). The Redis pub/sub and in-process bus pass the struct internally, so the wire format only changes at the final encode step — no internal churn.

The client message classifier changes from `op === "update"` to `o === 1`.

---

### Section 2: Array index diffing

**Current:** any element change replaces the entire array.

```json
{"u": [["stage.scenes.board.data.board", [["","",""],["","X",""],["","O",""]]]]}
```

**New:** `diffInto` recurses into arrays and emits per-index paths.

```json
{"u": [[[1,4,0,1,0,1,1], "X"], [[1,4,0,1,0,2,1], "O"]]}
```

For a 20×20 board with one cell changed: current ~800 bytes (whole array), new ~12 bytes (one indexed path). The crossover where whole-array replacement is cheaper than indexed paths is ~50% of cells changing simultaneously — this never occurs in practice for game moves.

**Diffing logic** in `diffInto` gains one new branch alongside the existing map/scalar branches:

- If both values are `[]interface{}`: call `diffSlice(path, before, after, ...)`
- `diffSlice` walks index-by-index: if element is a map on both sides, recurse into `diffInto`; if scalar or mismatched types, compare with `reflect.DeepEqual` and emit if different
- Indices only in `before` → emit removed paths
- Indices only in `after` → emit updated paths

The client-side `updateJSONKeyByDotPath` in `GameStateParser` already handles numeric-segment paths correctly (it checks `if /^\d+$/.test(parts[i+1])`), so no change is needed there.

---

### Section 3: Static/dynamic keyframe split

The tic-tac-toe keyframe is ~2200 bytes; ~1500 bytes are `data.layout` (Lua scripts, widget configs from `config.json`) which never changes during a game session.

**New protocol: layout frame + slim keyframe**

On `join`, `create`, and `reconnect`, the server sends two messages:

1. **Layout frame** (new message type, `o: 4`, sent once per connection):
```json
{"o":4,"v":"<sha256-of-layout>","data":{...layout...}}
```

2. **Slim keyframe** (same shape as today, `data.layout` omitted):
```json
{"id":"...","code":"TAA","teams":{...},"players":{...},"stage":{...},"data":{}}
```

On `refresh` (re-sends state while still connected), only the slim keyframe is sent — layout is already cached client-side from the original join.

The `v` field is a short hash of the layout content. If a layout frame arrives with a matching hash, the client treats it as a no-op. This guards against a server restart with a new config while the client is connected.

**Server:** a helper at boot extracts and hashes the layout from `i.Script` and holds it on the injector. `GameService.Sanitize` (or the keyframe write path) strips `data.layout` before writing to the connection.

**Client:** `message-handler.ts` handles `o === 4` by storing layout keyed by hash. When a slim keyframe arrives, the client merges the cached layout into `data.layout` before passing state to `GameStateParser`.

After this change: typical keyframe ~400 bytes (down from ~2200). Layout sent once per connection (~1500 bytes), never again.

---

### Section 4: Binary transport (MessagePack + debug mode)

All server→client messages are encoded as MessagePack by default. Client→server messages remain JSON (player moves are small).

**Library choices** (to be verified during implementation):
- Go: `github.com/vmihaiela/msgpack/v5`
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
  "p0": {"name":"","score":0,"connected":false,"host":false,"controller":false},
  "p1": {"name":"","score":0,"connected":false,"host":false,"controller":false}
}
```

Teams reference slot IDs in `playerIds`: `["p0"]`, `["p1"]`. A joining player fills a pre-declared slot; the `userId → slotId` mapping is stored in the session. No new keys are added to `players` after game creation.

The server validates at boot that `config.maxTeams` matches the number of entries in the `teams` object.

#### Positional path encoding

At join time, both the server and client independently walk the keyframe, sort each object's keys alphanumerically (case-sensitive), and build the same positional map — no extra wire cost. The keyframe is the schema.

Path `"stage.scenes.board.data.board.1.1"` becomes `[5, 4, 0, 1, 0, 1, 1]` (indices into sorted key lists at each level).

The `u` (updated) and `r` (removed) fields change from string-keyed maps to arrays of `[path, value]` pairs, avoiding JSON key-type constraints:

```
MessagePack:  "u": [[[5,4,0,1,0,1,1], "X"]]
JSON debug:   "u": [["5.4.0.1.0.1.1", "X"]]
```

In MessagePack a 7-element fixarray of small integers is **8 bytes** vs a 35-byte dotted-path string. For a delta with 3 changed paths that is ~80 bytes of key savings.

**Schema version field:** the slim keyframe includes `sv` (schema version) — a short hash of the sorted key structure. If a client receives a keyframe with an `sv` it has not seen before, it re-derives the positional map from that keyframe. This guards against a server restart with a new script while a client is connected.

**JSON debug mode:** paths are emitted as numeric-dotted strings (`"5.4.0.1.0.1.1"`). The existing `GameStateParser.updateJSONKeyByDotPath` already handles numeric segments, so the parser works with minimal changes in debug mode.

---

## Config.json contract

Every game `config.json` must satisfy:

1. `config.maxTeams` equals the number of entries in the top-level `teams` object
2. `config.maxPlayersPerTeam` × number of teams = total player slots pre-declared at game creation
3. Every field referenced by game logic (Lua scripts, server handlers) must be present in its initial section with a default value
4. No new top-level keys are added to the game state after creation

The server enforces rules 1–3 at boot and panics with a descriptive message if violated.

---

## File changes summary

### Server (Go)
| File | Change |
|---|---|
| `internal/services/events/events.go` | New `ChangeEvent` struct with short field names; `OpCode uint8` type + constants |
| `internal/services/events/delta.go` | `diffInto` gains array-index branch; `SanitizeDelta` strips metadata paths; `Diff` output is `[][2]interface{}` pairs |
| `internal/clients/melody/client.go` | Detect `?debug=1` at upgrade; encode outbound as MessagePack or JSON |
| `internal/services/game/service.go` | `Sanitize` strips `data.layout`; layout frame sent before keyframe on join/reconnect |
| `internal/models/script.go` | `Script.Config` gains `MaxPlayersPerTeam`, validated against `Teams` count at boot |
| `internal/repo/game/game.go` | `New` pre-declares player slots from `maxPlayersPerTeam × len(teams)` |
| `boot/boot.go` | Boot-time schema validation: complete field coverage + config consistency |

### Client (TypeScript)
| File | Change |
|---|---|
| `client/models/models.ts` | Update `UpdateMessage`: `o uint8`, `t number`, `u` and `r` as pair arrays |
| `client/services/message-handler.ts` | Decode MessagePack; handle `o === 4` layout frame; merge layout into keyframe |
| `client/services/game-state-parser.ts` | Accept integer-array paths in normal mode; numeric-dotted strings in debug mode |

### Docs
| File | Change |
|---|---|
| `docs/PROTOCOL.md` | Document new delta envelope, opcode table, layout frame, positional paths, debug mode |
| `example/tictactoe/config.json` | Add `winningTeam: null` to `stage.scenes.board.data` |

---

## Size comparison (tic-tac-toe board move)

| Message | Before | After (MessagePack) | Reduction |
|---|---|---|---|
| Delta (1 cell change) | ~295 bytes | ~25 bytes | ~91% |
| Keyframe (join) | ~2200 bytes | ~500 bytes (layout once + slim keyframe) | ~77% |
| Keyframe (refresh) | ~2200 bytes | ~150 bytes (slim keyframe only) | ~93% |

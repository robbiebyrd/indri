# Compact Delta Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the verbose JSON WebSocket delta protocol with a compact MessagePack format using positional path encoding, array-index diffing, and layout/state keyframe splitting — targeting ~90% reduction in delta sizes and ~77–93% in keyframe sizes.

**Architecture:** The server computes diffs as before but now emits `ChangeEvent` as pair-arrays `[[path, value], ...]` with positional integer paths; at join time the server sends a one-time layout frame (`o:4`) followed by a slim keyframe wrapper (`{sv, game}`); all server→client messages are MessagePack by default with JSON fallback on `?debug=1`.

**Tech Stack:** Go (server) with a MessagePack library — verify the exact module path via `go get` before using (see Task 8 Step 1; do NOT hardcode a guessed path); TypeScript client with `@msgpack/msgpack`.

**Spec:** `docs/superpowers/specs/2026-09-21-compact-delta-protocol-design.md`

---

## File Map

### Created
| File | Purpose |
|---|---|
| `internal/services/events/positional.go` | `BuildPositionalMap`, `EncodePath`, `ToDeltaPairs` |
| `internal/services/events/positional_test.go` | Tests for positional encoding |
| `internal/services/events/wire.go` | `KeyframeWrapper`, `LayoutFrame`, `writeEncoded` helper |
| `client/services/positional-map.ts` | `buildPositionalMap`, path resolution helpers |
| `client/services/positional-map.node-test.ts` | Tests for positional map |

### Modified (Go)
| File | What changes |
|---|---|
| `internal/services/events/events.go` | `OpCode uint8` + constants; `ChangeEvent` fields renamed + retyped |
| `internal/services/events/delta.go` | `diffSlice` added; `SanitizeDelta` signature + metadata strip |
| `internal/services/events/delta_test.go` | Update for new types; add array-index and metadata-strip tests |
| `internal/services/events/redis.go` | No change needed (marshals `ChangeEvent` struct directly) |
| `internal/services/events/inprocess.go` | No change needed (passes struct through channel) |
| `internal/models/player.go` | Add `UserID string` |
| `internal/models/session.go` | Add `SlotID *string` to `Session`, `CreateSession`, `UpdateSession` |
| `internal/models/config.go` | No change — `MaxPlayersPerTeam` already exists |
| `internal/repo/game/game.go` | `New` pre-declares slots + pre-populates team PlayerIDs; `publish` builds positional map + pair-array |
| `internal/repo/game/player.go` | `AssignSlot` (new); `RemovePlayer` clears slot not deletes key; `DisconnectPlayer` accepts slotId |
| `internal/repo/game/team.go` | Player lookups in HasPlayerOnTeam updated to use slot keys |
| `internal/repo/game/host.go` | Host resolution updated to use slot keys |
| `internal/repo/game/interface.go` | Add `AssignSlot` to Storer interface |
| `internal/services/game/game.go` | `RemovePlayer` and `DisconnectPlayer` accept slotId |
| `internal/injector/injector.go` | Add `LayoutHash string` and `LayoutData map[string]interface{}` to `Injector` struct |
| `internal/handlers/actions/join/handler.go` | Assign slot; write layout frame + slim keyframe |
| `internal/handlers/actions/create/handler.go` | Assign slot; write layout frame + slim keyframe |
| `internal/handlers/actions/reconnect/handler.go` | Write layout frame + slim keyframe |
| `internal/handlers/actions/refresh/handler.go` | Write slim keyframe only |
| `internal/handlers/actions/kick/handler.go` | Accept `slotId`; resolve userId via slot record |
| `internal/handlers/actions/leave/handler.go` | Pass `*session.SlotID` to RemovePlayer |
| `internal/entrypoints/websocket.go` | Pass `*session.SlotID` to DisconnectPlayer |
| `internal/transport/ws/ws.go` | Detect `?debug=1` at connect; expose `WriteEncoded` |
| `internal/services/broadcast/broadcast.go` | Encode delta per-connection using `ws.WriteEncoded` |
| `internal/services/boot/boot.go` | Boot validation: maxTeams == len(teams), maxPlayersPerTeam > 0 |
| `example/tictactoe/server/handlers/move/handler.go` | Update player lookup to use session SlotID |
| `example/tictactoe/server/handlers/restart/handler.go` | Update player lookup to use session SlotID |

### Modified (TypeScript)
| File | What changes |
|---|---|
| `client/models/models.ts` | Update `UpdateMessage`; add `SlimKeyframe`, `LayoutFrame` |
| `client/services/game-state-parser.ts` | Accept `positionalMap`; resolve integer paths |
| `client/services/game-state-parser.node-test.ts` | Add positional-path tests; update `delta()` helper |
| `client/services/message-handler.ts` | MessagePack decode; new classifier; layout/keyframe handling |

### Modified (Config + Docs)
| File | What changes |
|---|---|
| `example/tictactoe/config.json` | Add `"winningTeam": null` to `stage.scenes.board.data` |
| `docs/PROTOCOL.md` | Document new protocol |

---

## Task 1: ChangeEvent types and OpCode constants

**Files:**
- Modify: `internal/services/events/events.go`
- Modify: `internal/services/events/delta_test.go`

- [ ] **Step 1: Remove old `TestSanitizeDelta_*` tests that use the old map/slice signature**

In `internal/services/events/delta_test.go`, delete `TestSanitizeDelta_DropsPrivatePaths` and `TestSanitizeDelta_StripsNestedPrivateFromValue`. Both call `events.SanitizeDelta(map[string]interface{}, []string)`. When Step 4 changes that signature they will produce a compile error of the wrong kind (type mismatch instead of the expected "failing test"). Step 6 adds their replacements using the new signature.

- [ ] **Step 2: Write failing test for new ChangeEvent shape**

Add to `internal/services/events/delta_test.go`:
```go
func TestSanitizeDelta_PairArrayFormat(t *testing.T) {
    updated := [][]interface{}{
        {"players.p0.host", true},
        {"stage.privateData.secret", "x"},
    }
    removed := []interface{}{"players.p1", "teams.t1.privateData.key"}

    gotUpdated, gotRemoved := events.SanitizeDelta(updated, removed)

    if len(gotUpdated) != 1 || gotUpdated[0][0] != "players.p0.host" {
        t.Errorf("expected one public update, got %v", gotUpdated)
    }
    if len(gotRemoved) != 1 || gotRemoved[0] != "players.p1" {
        t.Errorf("expected one public removal, got %v", gotRemoved)
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/services/events/ -run TestSanitizeDelta_PairArrayFormat -v
```
Expected: compile error — `SanitizeDelta` not yet accepting new types.

- [ ] **Step 4: Update `events.go`**

Replace the file contents:
```go
package events

import (
    "context"
    "time"
)

type OpCode uint8

const (
    OpUpdate OpCode = 1
    OpInsert OpCode = 2
    OpDelete OpCode = 3
    OpLayout OpCode = 4
)

// ChangeEvent is the delta broadcast to clients. UpdatedFields is a slice of
// [path, value] pairs where path is a dotted string or positional int slice.
// RemovedFields is a slice of paths.
type ChangeEvent struct {
    ID            string          `json:"id"`
    OperationType OpCode          `json:"o"`
    Timestamp     time.Time       `json:"t"`
    Collection    string          `json:"type,omitempty"`
    UpdatedFields [][]interface{} `json:"u,omitempty"`
    RemovedFields []interface{}   `json:"r,omitempty"`
}

func (e ChangeEvent) HasChanges() bool {
    return len(e.UpdatedFields) > 0 || len(e.RemovedFields) > 0
}

type Publisher interface {
    Publish(ctx context.Context, event ChangeEvent) error
    Subscribe(ctx context.Context) (<-chan ChangeEvent, error)
}
```

- [ ] **Step 5: Update `SanitizeDelta` in `delta.go`**

Replace the `SanitizeDelta` function and its helper signature:
```go
// SanitizeDelta removes private data from a pair-array delta.
func SanitizeDelta(updated [][]interface{}, removed []interface{}) ([][]interface{}, []interface{}) {
    cleanUpdated := make([][]interface{}, 0, len(updated))
    for _, pair := range updated {
        if len(pair) != 2 {
            continue
        }
        path, ok := pair[0].(string)
        if !ok || pathHasSegment(path, privateDataKey) {
            continue
        }
        cleanUpdated = append(cleanUpdated, []interface{}{path, stripKey(pair[1], privateDataKey)})
    }

    var cleanRemoved []interface{}
    for _, item := range removed {
        path, ok := item.(string)
        if ok && !pathHasSegment(path, privateDataKey) {
            cleanRemoved = append(cleanRemoved, path)
        }
    }

    return cleanUpdated, cleanRemoved
}
```

- [ ] **Step 6: Update `game.go:publish` to use new types**

In `internal/repo/game/game.go`, update the `publish` method signature and body:
```go
func (s *Store) publish(id string, op events.OpCode, updated [][]interface{}, removed []interface{}) {
    if s.publisher == nil {
        return
    }

    updated, removed = events.SanitizeDelta(updated, removed)

    event := events.ChangeEvent{
        ID:            id,
        OperationType: op,
        Timestamp:     time.Now(),
        Collection:    collectionName,
        UpdatedFields: updated,
        RemovedFields: removed,
    }

    if !event.HasChanges() {
        return
    }

    if err := s.publisher.Publish(*s.ctx, event); err != nil {
        log.Printf("could not publish change event for game %v: %v", id, err)
    }
}
```

Update `publishDiff` to convert `Diff` output to pair-array format. Do **not** strip metadata keys here — that is `SanitizeDelta`'s responsibility (added in Task 3). `publish()` already calls `SanitizeDelta`:
```go
func (s *Store) publishDiff(id string, before map[string]interface{}, after *models.Game) {
    if s.publisher == nil {
        return
    }

    afterMap, err := events.ToMap(after)
    if err != nil {
        log.Printf("could not snapshot game %v for change event: %v", id, err)
        return
    }

    updatedMap, removedSlice := events.Diff(before, afterMap)

    // Convert to pair-array format; SanitizeDelta (called inside publish) strips private/metadata.
    var updated [][]interface{}
    for k, v := range updatedMap {
        updated = append(updated, []interface{}{k, v})
    }
    var removed []interface{}
    for _, r := range removedSlice {
        removed = append(removed, r)
    }

    s.publish(id, events.OpUpdate, updated, removed)
}
```

Update `UpdateField`, `DeleteField`, and `markPlayerConnected` in `game.go` and `player.go` to pass pair-array format to `publish`.

For `UpdateField`:
```go
s.publish(id, events.OpUpdate, [][]interface{}{{key, value}}, nil)
```

For `DeleteField`:
```go
s.publish(id, events.OpUpdate, nil, []interface{}{key})
```

For `markPlayerConnected` in `player.go`:
```go
s.publish(id, events.OpUpdate, [][]interface{}{{playerKey + ".connected", connected}}, nil)
```

- [ ] **Step 7: Update old delta tests to compile with new SanitizeDelta signature**

The existing `TestSanitizeDelta_*` tests use the old map/slice signature. Replace them:

```go
func TestSanitizeDelta_DropsPrivatePaths(t *testing.T) {
    updated := [][]interface{}{
        {"players.p0.host", true},
        {"stage.privateData.answer", "42"},
        {"teams.t1.privateData", map[string]interface{}{"role": "spy"}},
    }
    removed := []interface{}{"players.p1", "stage.scenes.s1.privateData.key"}

    gotUpdated, gotRemoved := events.SanitizeDelta(updated, removed)

    if len(gotUpdated) != 1 || gotUpdated[0][0] != "players.p0.host" {
        t.Errorf("expected one public update, got %v", gotUpdated)
    }
    if len(gotRemoved) != 1 || gotRemoved[0] != "players.p1" {
        t.Errorf("wrong removals, got %v", gotRemoved)
    }
}

func TestSanitizeDelta_StripsNestedPrivateFromValue(t *testing.T) {
    updated := [][]interface{}{
        {"players.p0", map[string]interface{}{
            "host":        true,
            "privateData": map[string]interface{}{"secret": "role"},
        }},
    }
    gotUpdated, _ := events.SanitizeDelta(updated, nil)

    player, ok := gotUpdated[0][1].(map[string]interface{})
    if !ok {
        t.Fatalf("expected player object, got %T", gotUpdated[0][1])
    }
    if _, ok := player["privateData"]; ok {
        t.Error("nested privateData leaked")
    }
    if player["host"] != true {
        t.Error("public field was stripped")
    }
}
```

- [ ] **Step 7: Run all events tests**

```bash
go test ./internal/services/events/ -v
```
Expected: all pass.

- [ ] **Step 8: Run full Go build to catch type errors across packages**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 9: Commit**

```bash
git add internal/services/events/events.go internal/services/events/delta.go \
        internal/services/events/delta_test.go internal/repo/game/game.go \
        internal/repo/game/player.go
git commit -m "feat(events): new ChangeEvent shape with pair-array fields and OpCode type"
```

---

## Task 2: Array index diffing

**Files:**
- Modify: `internal/services/events/delta.go`
- Modify: `internal/services/events/delta_test.go`

- [ ] **Step 1: Remove `TestDiff_ArrayReplacedWhole`**

In `internal/services/events/delta_test.go`, delete the `TestDiff_ArrayReplacedWhole` function. After Task 2 adds `diffSlice`, this test asserts the old whole-array behaviour (`updated["board"] == ["X","O"]`) which will be wrong — the diff will now emit `updated["board.1"] = "O"`. The function `TestDiff_ArrayIndexed_NoChange` added below replaces it.

- [ ] **Step 2: Write failing tests for per-index array diffs**

Add to `delta_test.go`:
```go
func TestDiff_ArrayIndexed_ScalarChange(t *testing.T) {
    before := map[string]interface{}{"board": []interface{}{"X", "", ""}}
    after  := map[string]interface{}{"board": []interface{}{"X", "O", ""}}

    updated, removed := events.Diff(before, after)

    if _, ok := updated["board"]; ok {
        t.Error("whole board must not be replaced")
    }
    if updated["board.1"] != "O" {
        t.Errorf("expected board.1=O, got %v", updated)
    }
    if len(removed) != 0 {
        t.Errorf("unexpected removals: %v", removed)
    }
}

func TestDiff_ArrayIndexed_2D(t *testing.T) {
    before := map[string]interface{}{
        "board": []interface{}{
            []interface{}{"", "", ""},
            []interface{}{"", "", ""},
        },
    }
    after := map[string]interface{}{
        "board": []interface{}{
            []interface{}{"", "", ""},
            []interface{}{"", "X", ""},
        },
    }

    updated, _ := events.Diff(before, after)

    if updated["board.1.1"] != "X" {
        t.Errorf("expected board.1.1=X, got %v", updated)
    }
    if _, ok := updated["board"]; ok {
        t.Error("whole board must not be in updated")
    }
}

func TestDiff_ArrayIndexed_ElementAdded(t *testing.T) {
    before := map[string]interface{}{"ids": []interface{}{"p0"}}
    after  := map[string]interface{}{"ids": []interface{}{"p0", "p1"}}

    updated, removed := events.Diff(before, after)

    if updated["ids.1"] != "p1" {
        t.Errorf("expected ids.1=p1, got %v", updated)
    }
    if len(removed) != 0 {
        t.Errorf("unexpected removals: %v", removed)
    }
}

func TestDiff_ArrayIndexed_ElementRemoved(t *testing.T) {
    before := map[string]interface{}{"ids": []interface{}{"p0", "p1"}}
    after  := map[string]interface{}{"ids": []interface{}{"p0"}}

    _, removed := events.Diff(before, after)

    if len(removed) != 1 || removed[0] != "ids.1" {
        t.Errorf("expected [ids.1] removed, got %v", removed)
    }
}
```

Also update the old whole-array test (it now expects indexed behavior):
```go
func TestDiff_ArrayIndexed_NoChange(t *testing.T) {
    before := map[string]interface{}{"board": []interface{}{"X", ""}}
    after  := map[string]interface{}{"board": []interface{}{"X", ""}}

    updated, removed := events.Diff(before, after)

    if len(updated) != 0 || len(removed) != 0 {
        t.Errorf("expected no delta, got updated=%v removed=%v", updated, removed)
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/services/events/ -run "TestDiff_ArrayIndexed" -v
```
Expected: FAIL — `board.1` not in updated, `board` is.

- [ ] **Step 4: Add `diffSlice` and wire it into `diffInto`**

In `delta.go`, add after the existing `diffInto` function:
```go
func diffSlice(prefix string, before, after []interface{}, updated map[string]interface{}, removed *[]string) {
    minLen := len(before)
    if len(after) < minLen {
        minLen = len(after)
    }

    for i := 0; i < minLen; i++ {
        path := joinPath(prefix, strconv.Itoa(i))
        bMap, bIsMap := before[i].(map[string]interface{})
        aMap, aIsMap := after[i].(map[string]interface{})
        bSlice, bIsSlice := before[i].([]interface{})
        aSlice, aIsSlice := after[i].([]interface{})

        switch {
        case bIsMap && aIsMap:
            diffInto(path, bMap, aMap, updated, removed)
        case bIsSlice && aIsSlice:
            diffSlice(path, bSlice, aSlice, updated, removed)
        case !reflect.DeepEqual(before[i], after[i]):
            updated[path] = after[i]
        }
    }

    for i := minLen; i < len(before); i++ {
        *removed = append(*removed, joinPath(prefix, strconv.Itoa(i)))
    }
    for i := minLen; i < len(after); i++ {
        updated[joinPath(prefix, strconv.Itoa(i))] = after[i]
    }
}
```

Add `"strconv"` to the import block. Then update the array branch inside `diffInto`:

```go
beforeSlice, beforeIsSlice := beforeVal.([]interface{})
afterSlice, afterIsSlice := afterVal.([]interface{})

switch {
case beforeIsMap && afterIsMap:
    diffInto(path, beforeMap, afterMap, updated, removed)
case beforeIsSlice && afterIsSlice:
    diffSlice(path, beforeSlice, afterSlice, updated, removed)
case !reflect.DeepEqual(beforeVal, afterVal):
    updated[path] = afterVal
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/services/events/ -v
```
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/services/events/delta.go internal/services/events/delta_test.go
git commit -m "feat(events): array index diffing — emit per-index paths instead of whole-array"
```

---

## Task 3: Metadata field stripping in publishDiff

**Files:**
- Modify: `internal/repo/game/game.go`
- Modify: `internal/services/events/delta_test.go` (add test)

- [ ] **Step 1: Write failing test**

In `delta_test.go`, add:
```go
func TestSanitizeDelta_StripsMetadataFields(t *testing.T) {
    updated := [][]interface{}{
        {"updatedAt", "2026-09-21T00:00:00Z"},
        {"createdAt", "2026-09-21T00:00:00Z"},
        {"version", 42},
        {"stage.currentScene", "board"},
    }

    gotUpdated, _ := events.SanitizeDelta(updated, nil)

    for _, pair := range gotUpdated {
        key := pair[0].(string)
        if key == "updatedAt" || key == "createdAt" || key == "version" {
            t.Errorf("metadata key %q leaked into sanitized delta", key)
        }
    }
    if len(gotUpdated) != 1 || gotUpdated[0][0] != "stage.currentScene" {
        t.Errorf("expected only stage.currentScene, got %v", gotUpdated)
    }
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/services/events/ -run TestSanitizeDelta_StripsMetadataFields -v
```
Expected: FAIL — metadata fields are not stripped yet.

- [ ] **Step 3: Add metadata stripping to `SanitizeDelta`**

Add a package-level set and update `SanitizeDelta` in `delta.go`:
```go
var metadataKeys = map[string]bool{
    "updatedAt": true,
    "createdAt": true,
    "version":   true,
}

func SanitizeDelta(updated [][]interface{}, removed []interface{}) ([][]interface{}, []interface{}) {
    cleanUpdated := make([][]interface{}, 0, len(updated))
    for _, pair := range updated {
        if len(pair) != 2 {
            continue
        }
        path, ok := pair[0].(string)
        if !ok || pathHasSegment(path, privateDataKey) || metadataKeys[path] {
            continue
        }
        cleanUpdated = append(cleanUpdated, []interface{}{path, stripKey(pair[1], privateDataKey)})
    }

    var cleanRemoved []interface{}
    for _, item := range removed {
        path, ok := item.(string)
        if ok && !pathHasSegment(path, privateDataKey) && !metadataKeys[path] {
            cleanRemoved = append(cleanRemoved, path)
        }
    }

    return cleanUpdated, cleanRemoved
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/services/events/ -v
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/services/events/delta.go internal/services/events/delta_test.go
git commit -m "feat(events): strip updatedAt/createdAt/version from delta pairs"
```

---

## Task 4: Player model — add UserID field

**Files:**
- Modify: `internal/models/player.go`
- Modify: `internal/repo/game/player.go`
- Modify: `internal/repo/game/player_test.go` (or create alongside)

- [ ] **Step 1: Write failing test**

In `internal/repo/game/player_test.go` (use `package game` so it can access the Store directly):
```go
func TestAddPlayer_SetsUserID(t *testing.T) {
    // A freshly added player must carry its userId in the UserID field.
    p := models.Player{UserID: "u1", Name: "Alice"}
    if p.UserID != "u1" {
        t.Fatalf("UserID field not present on Player struct; compile this test to see the error")
    }
}
```

Run it:
```bash
go test ./internal/repo/game/ -run TestAddPlayer_SetsUserID -v
```
Expected: compile error — `models.Player` has no field `UserID`.

- [ ] **Step 2: Add `UserID` to Player**

In `internal/models/player.go`:
```go
type Player struct {
    UserID      string                  `bson:"userId"      json:"userId"`
    Name        string                  `bson:"name"        json:"name"`
    Score       int                     `bson:"score"       json:"score"`
    Connected   bool                    `bson:"connected"   json:"connected"`
    Host        bool                    `bson:"host"        json:"host"`
    Controller  bool                    `bson:"controller"  json:"controller"`
    PublicData  *map[string]interface{} `bson:"data"        json:"data,omitempty"`
    PrivateData *map[string]interface{} `bson:"privateData" json:"privateData,omitempty"`
}
```

- [ ] **Step 3: Set UserID in `AddPlayer`**

In `internal/repo/game/player.go`, update the `AddPlayer` call to set `UserID`:
```go
g.Players[userId] = models.Player{
    UserID:    userId,
    Name:      goaway.Censor(displayName),
    Host:      !gameHasHost(g),
    Connected: false,
}
```

- [ ] **Step 4: Run the new test to verify it passes, plus full regression check**

```bash
go test ./internal/repo/game/ -run TestAddPlayer_SetsUserID -v && go build ./... && go test ./...
```
Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/models/player.go internal/repo/game/player.go
git commit -m "feat(models): add UserID field to Player for slot→user resolution"
```

---

## Task 5: Session SlotID + player slot pre-declaration + auth migration

**Files:**
- Modify: `internal/models/session.go`
- Modify: `internal/repo/game/game.go`
- Modify: `internal/repo/game/player.go`
- Modify: `internal/repo/game/team.go`
- Modify: `internal/repo/game/host.go`
- Modify: `internal/repo/game/interface.go`
- Modify: `internal/services/game/game.go` (delete `ConnectPlayer`; update `RemovePlayer`/`DisconnectPlayer`)
- Modify: `internal/handlers/actions/join/handler.go`
- Modify: `internal/handlers/actions/create/handler.go`
- Modify: `internal/handlers/actions/kick/handler.go`
- Modify: `internal/handlers/actions/leave/handler.go`
- Modify: `internal/entrypoints/websocket.go`
- Modify: `example/tictactoe/server/handlers/move/handler.go`
- Modify: `example/tictactoe/server/handlers/restart/handler.go`
- Create: `internal/repo/game/game_slots_test.go`

- [ ] **Step 1: Add `SlotID` to session models**

In `internal/models/session.go`:
```go
type Session struct {
    ID        bson.ObjectID `bson:"_id,omitempty" json:"id" mongox:"autoID"`
    Token     string        `bson:"token,omitempty" json:"-"`
    GameID    *string       `bson:"gameId,omitempty" json:"gameId,omitempty"`
    UserID    *string       `bson:"userId,omitempty" json:"userId,omitempty"`
    TeamID    *string       `bson:"teamId,omitempty" json:"teamId,omitempty"`
    SlotID    *string       `bson:"slotId,omitempty" json:"slotId,omitempty"`
    CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
    UpdatedAt time.Time     `bson:"updatedAt" json:"updatedAt"`
    DeletedAt time.Time     `bson:"deletedAt,omitempty" json:"-"`
}

type CreateSession struct {
    Token     string    `bson:"token"     json:"-"`
    GameID    string    `bson:"gameId"     json:"gameId"`
    UserID    string    `bson:"userId"     json:"userId"`
    TeamID    string    `bson:"teamId"     json:"teamId"`
    SlotID    string    `bson:"slotId"     json:"slotId"`
    CreatedAt time.Time `bson:"createdAt"  json:"createdAt"`
}

type UpdateSession struct {
    GameID    string    `bson:"gameId"    json:"gameId"`
    UserID    string    `bson:"userId"    json:"userId"`
    TeamID    string    `bson:"teamId"    json:"teamId"`
    SlotID    string    `bson:"slotId"    json:"slotId"`
    UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
```

- [ ] **Step 2: Write failing test for slot pre-declaration**

Add `internal/repo/game/game_slots_test.go`. Use `package game` (not `package game_test`) because `preDeclareSlots` is unexported:
```go
package game

import (
    "sort"
    "testing"
    "github.com/robbiebyrd/indri/internal/models"
)

func TestPreDeclareSlots(t *testing.T) {
    script := &models.Script{
        Config: models.Config{MaxPlayersPerTeam: 2},
        Teams: map[string]models.Team{
            "Red":  {Name: "Red"},
            "Blue": {Name: "Blue"},
        },
    }
    players, teams := preDeclareSlots(script)
    // 2 teams × 2 players = 4 slots
    if len(players) != 4 {
        t.Errorf("expected 4 slots, got %d", len(players))
    }
    for id, p := range players {
        if p.UserID != "" || p.Name != "" || p.Connected {
            t.Errorf("slot %s not empty: %+v", id, p)
        }
    }

    // Teams' PlayerIDs should be pre-populated with slot IDs.
    // Teams sorted: Blue=[p0,p1], Red=[p2,p3]
    blueIDs := teams["Blue"].PlayerIDs
    sort.Strings(blueIDs)
    if len(blueIDs) != 2 || blueIDs[0] != "p0" || blueIDs[1] != "p1" {
        t.Errorf("Blue team slots wrong: %v", blueIDs)
    }
    redIDs := teams["Red"].PlayerIDs
    sort.Strings(redIDs)
    if len(redIDs) != 2 || redIDs[0] != "p2" || redIDs[1] != "p3" {
        t.Errorf("Red team slots wrong: %v", redIDs)
    }
}
```

- [ ] **Step 3: Run to verify it fails**

```bash
go test ./internal/repo/game/ -run TestPreDeclareSlots -v
```
Expected: compile error — `preDeclareSlots` not defined.

- [ ] **Step 4: Add `preDeclareSlots` helper and update `New`**

In `internal/repo/game/game.go`, add before `New`. Slots are assigned in sorted team order so each team owns a contiguous range — `preDeclareSlots` returns both the players map and an updated teams map with pre-populated `PlayerIDs`:
```go
// preDeclareSlots creates the fixed player-slot map from the script config.
// Teams are sorted alphanumerically; team i gets slots [i*max … (i+1)*max-1].
// Returns the players map and a copy of the teams map with PlayerIDs pre-populated.
func preDeclareSlots(script *models.Script) (map[string]models.Player, map[string]models.Team) {
    players := make(map[string]models.Player)
    teams := make(map[string]models.Team, len(script.Teams))

    // Copy teams from script so we can mutate PlayerIDs.
    for k, v := range script.Teams {
        teams[k] = v
    }

    // Sort team names for deterministic slot assignment.
    sortedNames := make([]string, 0, len(script.Teams))
    for name := range script.Teams {
        sortedNames = append(sortedNames, name)
    }
    sort.Strings(sortedNames)

    idx := 0
    for _, name := range sortedNames {
        team := teams[name]
        team.PlayerIDs = make([]string, 0, script.Config.MaxPlayersPerTeam)
        for j := 0; j < script.Config.MaxPlayersPerTeam; j++ {
            slotID := "p" + strconv.Itoa(idx)
            players[slotID] = models.Player{}
            team.PlayerIDs = append(team.PlayerIDs, slotID)
            idx++
        }
        teams[name] = team
    }

    return players, teams
}
```

Add `"sort"` and `"strconv"` to imports if not already present. Update `New` to use it:
```go
if script != nil {
    slots, teamsWithSlots := preDeclareSlots(script)
    gameDataModel.Teams = &teamsWithSlots
    gameDataModel.Stage = &script.Stage
    gameDataModel.PublicData = script.PublicData
    gameDataModel.PrivateData = script.PrivateData
    gameDataModel.Players = &slots
}
```

- [ ] **Step 5: Add `AssignSlot` helper in `player.go`**

`AssignSlot` finds the first empty slot in the team's pre-declared `PlayerIDs` list (not globally, because global search would assign across team boundaries):
```go
// AssignSlot claims the first empty slot in the given team for userId.
// It looks only at slots pre-declared in g.Teams[teamId].PlayerIDs so a
// player joining team A never lands in team B's slot.
func (s *Store) AssignSlot(id string, teamId string, userId string, displayName string) (string, error) {
    var assignedSlot string

    err := s.Mutate(id, func(g *models.Game) error {
        team, ok := g.Teams[teamId]
        if !ok {
            return fmt.Errorf("team %v not found in game %v", teamId, id)
        }
        for _, slotID := range team.PlayerIDs {
            player := g.Players[slotID]
            if player.UserID == "" {
                player.UserID = userId
                player.Name = goaway.Censor(displayName)
                player.Connected = true
                player.Host = !gameHasHost(g)
                g.Players[slotID] = player
                assignedSlot = slotID
                return nil
            }
        }
        return fmt.Errorf("no available player slots in team %v of game %v", teamId, id)
    })

    return assignedSlot, err
}
```

- [ ] **Step 6: Update `join` handler to use `AssignSlot`**

In `internal/handlers/actions/join/handler.go`, replace the `ConnectPlayer` call:
```go
slotId, err := h.i.GameRepo.AssignSlot(g.ID.Hex(), *teamId, *session.UserID, displayName)
if err != nil {
    log.Printf("error assigning slot for player %v in game %v: %v\n", *session.UserID, *gameCode, err)
    return err
}
```

Update the `SessionService.Update` call to include `SlotID`:
```go
if err = h.i.SessionService.Update(*sessionId, &models.UpdateSession{
    GameID:    g.ID.Hex(),
    UserID:    *session.UserID,
    TeamID:    *teamId,
    SlotID:    slotId,
    UpdatedAt: time.Now(),
}); err != nil {
    return err
}
```

- [ ] **Step 7: Update `kick` handler to accept `slotId`**

In `internal/handlers/actions/kick/handler.go`, replace the `userId` extraction:
```go
targetSlotId, ok := decodedMsg["slotId"].(string)
if !ok || targetSlotId == "" {
    return fmt.Errorf("slotId to kick must be provided")
}
```

Replace the host check (now uses session's slot to look up caller). Check for nil before dereferencing:
```go
if callerSession.SlotID == nil || *callerSession.SlotID == "" {
    return fmt.Errorf("calling session has no slot id")
}
callerSlot := *callerSession.SlotID
if !g.Players[callerSlot].Host {
    return fmt.Errorf("caller %v is not the host of game %v", callerSlot, *gameCode)
}
```

Replace the target resolution (get userId from slot's UserID field):
```go
targetPlayer, ok := g.Players[targetSlotId]
if !ok || targetPlayer.UserID == "" {
    return fmt.Errorf("slot %v is empty or not found in game %v", targetSlotId, *gameCode)
}
targetUserId := targetPlayer.UserID
targetSession, err := h.i.SessionService.GetByUserID(targetUserId)
```

Replace `RemovePlayer` call:
```go
if err = h.i.GameRepo.RemovePlayer(gameId, targetSlotId); err != nil {
    log.Printf("could not remove player %v from game %v: %v\n", targetSlotId, gameId, err)
}
```

- [ ] **Step 8: Update `RemovePlayer` in `player.go` to clear slot, not delete key**

```go
func (s *Store) RemovePlayer(id string, slotId string) error {
    if err := sessionUtils.ValidateGameAndUser(id, slotId); err != nil {
        return err
    }

    return s.Mutate(id, func(g *models.Game) error {
        slot, ok := g.Players[slotId]
        if !ok {
            return fmt.Errorf("slot %v not found in game %v", slotId, id)
        }
        // Clear the slot, preserving the key for schema stability.
        slot.UserID = ""
        slot.Name = ""
        slot.Connected = false
        slot.Host = false
        slot.Controller = false
        g.Players[slotId] = slot
        return nil
    })
}
```

Also update `DisconnectPlayer` in `player.go` — it calls `markPlayerConnected` which uses `players.<userId>` as the MongoDB field path; it now takes a slotId:
```go
func (s *Store) DisconnectPlayer(id string, slotId string) error {
    return s.markPlayerConnected(id, slotId, false)
}

func (s *Store) ConnectPlayer(id string, slotId string) error {
    return s.markPlayerConnected(id, slotId, true)
}
```

The `markPlayerConnected` function body already uses `"players." + userId` as the field path — since slot IDs are now the map keys, this path is still correct (`players.p0.connected`).

- [ ] **Step 9: Update `interface.go` (Storer) — add `AssignSlot`, remove now-invalid slot-keyed methods**

In `internal/repo/game/interface.go`:

1. Add `AssignSlot`:
```go
AssignSlot(id string, teamId string, userId string, displayName string) (string, error)
```

2. Update the parameter names for `RemovePlayer`, `ConnectPlayer`, and `DisconnectPlayer`:
```go
RemovePlayer(id string, slotId string) error
ConnectPlayer(id string, slotId string) error
DisconnectPlayer(id string, slotId string) error
```

3. Remove `AddPlayer`, `AddPlayerToTeam`, `RemovePlayerFromTeam`, `ChangePlayerTeam`, `HasPlayerOnTeam`, `PlayerOnWhichTeam`, and `PlayerOnATeam` from the interface. These methods work by inserting `g.Players[userId]` entries with dynamic MongoDB ObjectID keys — after slot pre-declaration, inserting arbitrary new keys corrupts the schema. Replace all callers with `AssignSlot`. Delete the corresponding `Store` method bodies in `player.go` and `team.go` (or panic-guard them if external game code still calls them — but none does in the built-in handlers after this task). The `var _ Storer = (*Store)(nil)` assertion will fail at compile time until the interface and implementation are in sync.

The `var _ Storer = (*Store)(nil)` compile-time assertion will fail until `AssignSlot` is added here and the deleted methods are removed from both the interface and the Store.

- [ ] **Step 10: Update `GameService` in `internal/services/game/game.go`**

**Delete `ConnectPlayer`** entirely. It calls `AddPlayer`, `HasPlayerOnTeam`, `ChangePlayerTeam`, `AddPlayerToTeam`, and `ConnectPlayer` on the game repo — all of which are removed from the Storer interface in Step 9. After Step 9, `ConnectPlayer` will produce a compile error. It is superseded by `AssignSlot` (called directly from handlers). Check that no handler still calls `GameService.ConnectPlayer` after Steps 6 and 7 migrate `join`/`create` to `AssignSlot` — if any remain, update them.

**Update `RemovePlayer` and `DisconnectPlayer`** — change parameter name to `slotId`:
```go
func (gs *Service) RemovePlayer(id string, slotId string) error {
    return gs.gameRepo.RemovePlayer(id, slotId)
}

func (gs *Service) DisconnectPlayer(id string, slotId string) error {
    return gs.gameRepo.DisconnectPlayer(id, slotId)
}
```

Note: the field name is `gs.gameRepo` (lowercase), not `gs.GameRepo` — check the actual struct field name in `game.go` before writing.

- [ ] **Step 11: Update `leave` handler to use `SlotID`**

In `internal/handlers/actions/leave/handler.go`, replace:
```go
err = h.i.GameService.RemovePlayer(g.ID.Hex(), *session.UserID)
```
with:
```go
if session.SlotID == nil || *session.SlotID == "" {
    return fmt.Errorf("session has no slot id; player cannot leave")
}
err = h.i.GameService.RemovePlayer(g.ID.Hex(), *session.SlotID)
```

- [ ] **Step 12: Update `HandleDisconnect` in `internal/entrypoints/websocket.go` to use `SlotID`**

Replace:
```go
err = gs.DisconnectPlayer(*session.GameID, *session.UserID)
```
with:
```go
if session.SlotID == nil || *session.SlotID == "" {
    log.Print("session has no slotId; cannot mark player disconnected")
    return
}
err = gs.DisconnectPlayer(*session.GameID, *session.SlotID)
```

- [ ] **Step 13: Update `team.go` and `host.go` — player lookups now use slot keys**

In `internal/repo/game/team.go`, any function that does `g.Players[userId]` must change to `g.Players[slotId]`. The function signatures for `HasPlayerOnTeam`, `ChangePlayerTeam`, `AddPlayerToTeam`, `RemovePlayerFromTeam`, and `PlayerOnWhichTeam` all take `userId string` — rename the parameter to `slotId` in the implementation bodies. The public API signature stays the same (it still accepts a string); only the internal field access changes.

In `internal/repo/game/host.go`, `PlayerIsHost(id string, playerId string)` does `g.Players[playerId]` — this is correct as-is if the caller passes a slotId; verify all callers pass slotId after the auth migration.

- [ ] **Step 14: Verify example tictactoe handlers for slot-model compatibility**

The move and restart handlers resolve the acting player via `GetGameIDAndTeamID` which returns `(gameId, teamId)` — they do not access `g.Players` directly by UserId or SlotId. Read both handlers before making any changes. If either accesses `session.UserID` directly for player lookup (not just for session resolution), replace with `session.SlotID`. In most cases no change is needed here for authorization — the `winningTeam` and key-deletion fixes are handled in Task 9, which covers the substantive changes to these files.

- [ ] **Step 15: Build and run all tests**

```bash
go build ./... && go test ./... -race
```
Expected: pass. Fix any remaining compilation errors from call-site changes.

- [ ] **Step 16: Commit**

```bash
git add internal/models/session.go internal/models/player.go \
        internal/repo/game/game.go internal/repo/game/player.go \
        internal/repo/game/team.go internal/repo/game/host.go \
        internal/repo/game/interface.go internal/repo/game/game_slots_test.go \
        internal/services/game/game.go \
        internal/handlers/actions/join/handler.go \
        internal/handlers/actions/create/handler.go \
        internal/handlers/actions/kick/handler.go \
        internal/handlers/actions/leave/handler.go \
        internal/entrypoints/websocket.go \
        example/tictactoe/server/handlers/move/handler.go \
        example/tictactoe/server/handlers/restart/handler.go
git commit -m "feat(game): player slot model — pre-declared slots, team-aware assignment, SlotID in session, auth migration"
```

---

## Task 6: Positional path encoding

**Files:**
- Create: `internal/services/events/positional.go`
- Create: `internal/services/events/positional_test.go`
- Modify: `internal/repo/game/game.go`

- [ ] **Step 1: Create failing tests**

Create `internal/services/events/positional_test.go`:
```go
package events_test

import (
    "testing"
    "github.com/robbiebyrd/indri/internal/services/events"
)

func TestBuildPositionalMap_SortsKeys(t *testing.T) {
    obj := map[string]interface{}{
        "stage":   map[string]interface{}{},
        "players": map[string]interface{}{},
        "code":    "X",
        "data":    map[string]interface{}{},
    }
    posMap := events.BuildPositionalMap(obj)

    // Sorted: code=0, data=1, players=2, stage=3
    if posMap["code"] != 0 || posMap["data"] != 1 || posMap["players"] != 2 || posMap["stage"] != 3 {
        t.Errorf("unexpected positions: %v", posMap)
    }
}

func TestEncodePath_ObjectSegments(t *testing.T) {
    // Schema has only one key at each level, so every object key sorts to index 0.
    schema := map[string]interface{}{
        "stage": map[string]interface{}{
            "scenes": map[string]interface{}{
                "board": map[string]interface{}{
                    "data": map[string]interface{}{
                        "board": []interface{}{},
                    },
                },
            },
        },
    }
    posMap := events.BuildPositionalMap(schema)

    path := events.EncodePath("stage.scenes.board.data.board.1.1", schema, posMap)

    // stage→0 (only root key), scenes→0, board→0, data→0, board→0, then raw indices 1,1
    if len(path) != 7 {
        t.Errorf("expected 7 segments, got %d: %v", len(path), path)
    }
    want := []interface{}{0, 0, 0, 0, 0, 1, 1}
    for i, seg := range want {
        if path[i] != seg {
            t.Errorf("segment %d: expected %v, got %v (full path: %v)", i, seg, path[i], path)
        }
    }
}

func TestEncodePath_RoundTrip(t *testing.T) {
    schema := map[string]interface{}{
        "a": map[string]interface{}{
            "b": "value",
        },
    }
    posMap := events.BuildPositionalMap(schema)
    path := events.EncodePath("a.b", schema, posMap)

    if len(path) != 2 {
        t.Errorf("expected 2 segments, got %v", path)
    }
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/services/events/ -run "TestBuildPositionalMap|TestEncodePath" -v
```
Expected: compile error — functions not defined.

- [ ] **Step 3: Implement `positional.go`**

Create `internal/services/events/positional.go`:
```go
package events

import (
    "sort"
    "strconv"
    "strings"
)

// BuildPositionalMap walks obj and assigns each string key a 0-based integer
// index based on its sorted (case-sensitive, alphanumeric) position among its
// siblings. The map is flat: keys at any depth are present with their full
// dotted path. Array indices are not included — they are always raw integers.
func BuildPositionalMap(obj map[string]interface{}) map[string]int {
    result := make(map[string]int)
    buildPositionalMapInto("", obj, result)
    return result
}

func buildPositionalMapInto(prefix string, obj map[string]interface{}, result map[string]int) {
    keys := make([]string, 0, len(obj))
    for k := range obj {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    for i, k := range keys {
        path := joinPath(prefix, k)
        result[path] = i

        if child, ok := obj[k].(map[string]interface{}); ok {
            buildPositionalMapInto(path, child, result)
        }
    }
}

// EncodePath converts a dotted string path (e.g. "stage.scenes.board.data.board.1.1")
// into a slice of path segments. Object-key segments are replaced with their
// positional integer index; array-index segments (numeric) are kept as raw ints.
// The schema is used to determine whether each container is a map or array.
func EncodePath(path string, schema map[string]interface{}, posMap map[string]int) []interface{} {
    segments := strings.Split(path, ".")
    result := make([]interface{}, 0, len(segments))
    var current interface{} = schema

    for _, seg := range segments {
        switch container := current.(type) {
        case map[string]interface{}:
            // Object container: use positional index.
            fullPath := pathFromSegments(segments[:indexOf(segments, seg)+1])
            if idx, ok := posMap[fullPath]; ok {
                result = append(result, idx)
            } else {
                // Unknown key: fall back to string (defensive).
                result = append(result, seg)
            }
            current = container[seg]
        default:
            // Array container or unknown: treat segment as raw int if numeric.
            if n, err := strconv.Atoi(seg); err == nil {
                result = append(result, n)
                if arr, ok := current.([]interface{}); ok && n < len(arr) {
                    current = arr[n]
                } else {
                    current = nil
                }
            } else {
                result = append(result, seg)
                current = nil
            }
        }
    }

    return result
}

func pathFromSegments(segs []string) string {
    return strings.Join(segs, ".")
}

func indexOf(segs []string, target string) int {
    for i, s := range segs {
        if s == target {
            return i
        }
    }
    return -1
}
```

**Note:** `EncodePath` as written has an issue with `indexOf` when segments repeat. Rewrite to use the loop index directly:

```go
func EncodePath(path string, schema map[string]interface{}, posMap map[string]int) []interface{} {
    segments := strings.Split(path, ".")
    result := make([]interface{}, 0, len(segments))
    var current interface{} = schema

    for i, seg := range segments {
        partialPath := strings.Join(segments[:i+1], ".")

        switch container := current.(type) {
        case map[string]interface{}:
            if idx, ok := posMap[partialPath]; ok {
                result = append(result, idx)
            } else {
                result = append(result, seg)
            }
            current = container[seg]
        default:
            if n, err := strconv.Atoi(seg); err == nil {
                result = append(result, n)
                if arr, ok := current.([]interface{}); ok && n < len(arr) {
                    current = arr[n]
                } else {
                    current = nil
                }
            } else {
                result = append(result, seg)
                current = nil
            }
        }
    }

    return result
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/services/events/ -run "TestBuildPositionalMap|TestEncodePath" -v
```
Expected: pass.

- [ ] **Step 5: Wire positional encoding into `publishDiff`**

In `game.go`, update `publishDiff` to build the positional map and encode all paths:
```go
func (s *Store) publishDiff(id string, before map[string]interface{}, after *models.Game) {
    if s.publisher == nil {
        return
    }

    afterMap, err := events.ToMap(after)
    if err != nil {
        log.Printf("could not snapshot game %v for change event: %v", id, err)
        return
    }

    posMap := events.BuildPositionalMap(afterMap)
    updatedMap, removedSlice := events.Diff(before, afterMap)

    var updated [][]interface{}
    for k, v := range updatedMap {
        if k == "updatedAt" || k == "createdAt" || k == "version" {
            continue
        }
        encodedPath := events.EncodePath(k, afterMap, posMap)
        updated = append(updated, []interface{}{encodedPath, v})
    }
    var removed []interface{}
    for _, r := range removedSlice {
        encodedPath := events.EncodePath(r, afterMap, posMap)
        removed = append(removed, encodedPath)
    }

    s.publish(id, events.OpUpdate, updated, removed)
}
```

- [ ] **Step 6: Run all Go tests**

```bash
go test ./... -race
```
Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add internal/services/events/positional.go internal/services/events/positional_test.go \
        internal/repo/game/game.go
git commit -m "feat(events): positional path encoding for delta pairs"
```

---

## Task 7: Keyframe wrapper, layout frame, and slim keyframe

**Files:**
- Create: `internal/services/events/wire.go`
- Modify: `internal/injector/clients.go`
- Modify: `internal/handlers/actions/join/handler.go`
- Modify: `internal/handlers/actions/create/handler.go`
- Modify: `internal/handlers/actions/reconnect/handler.go`
- Modify: `internal/handlers/actions/refresh/handler.go`

- [ ] **Step 1: Create layout and keyframe wire types**

Create `internal/services/events/wire.go`:
```go
package events

import "github.com/robbiebyrd/indri/internal/models"

// LayoutFrame is sent once per connection before the first keyframe. It carries
// the static rendering config (Lua scripts, widget definitions) that never
// changes during a game session.
type LayoutFrame struct {
    O    OpCode                 `json:"o"`
    V    string                 `json:"v"`
    Data map[string]interface{} `json:"data"`
}

// KeyframeWrapper wraps a slim game keyframe (data.layout stripped) with a
// schema version hash. The "sv" field is the discriminator the client uses to
// identify this message type.
type KeyframeWrapper struct {
    SV   string       `json:"sv"`
    Game *models.Game `json:"game"`
}
```

- [ ] **Step 2: Add layout hash to `Injector` struct**

`Script` lives on `Injector` (not `ClientsInjector`) and `ClientsInjector` is wired before `Script` is available. Add `LayoutHash` and `LayoutData` directly to `Injector` in `internal/injector/injector.go`:
```go
type Injector struct {
    *ReposInjector
    *ClientsInjector
    *ServicesInjector
    Script        *models.Script
    LayoutHash    string
    LayoutData    map[string]interface{}
    GlobalContext context.Context
}
```

Add an exported helper function in `internal/injector/injector.go` (must be exported so `boot.go` in the `boot` package can call it). `script.PublicData` is a `map[string]interface{}` (not a pointer), so access it directly without dereferencing:
```go
import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
)

// ComputeLayoutHash extracts the layout data from the script's public data,
// computes a short stable hash, and returns both. Called once at boot time.
func ComputeLayoutHash(script *models.Script) (string, map[string]interface{}) {
    if script == nil || script.PublicData == nil {
        return "", nil
    }
    layout, _ := script.PublicData["layout"].(map[string]interface{})
    raw, _ := json.Marshal(layout)
    hash := sha256.Sum256(raw)
    return hex.EncodeToString(hash[:8]), layout
}
```

In `internal/services/boot/boot.go`, after `i.Script` is set, add:
```go
i.LayoutHash, i.LayoutData = injector.ComputeLayoutHash(i.Script)
```

- [ ] **Step 3: Add `WriteKeyframe` helper to `GameService`**

`GameService` will import the `transport` package for `transport.Conn`. This is a one-directional dependency (service → transport interface), which is acceptable — `transport` contains only interfaces and no concrete implementations, so no cycle results.

Add to `internal/services/game/game.go`:
```go
import (
    // existing imports...
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "github.com/robbiebyrd/indri/internal/transport"
    "github.com/robbiebyrd/indri/internal/services/events"
    ws "github.com/robbiebyrd/indri/internal/transport/ws"
)

// slimKeyframe returns a sanitized, layout-stripped copy of the game wrapped
// with a schema-version hash. It deep-clones via JSON round-trip before
// mutating so the caller's *models.Game is never modified.
func (svc *Service) slimKeyframe(g *models.Game) (events.KeyframeWrapper, error) {
    // Deep-clone through JSON so Sanitize and delete cannot mutate the live game.
    raw, err := json.Marshal(g)
    if err != nil {
        return events.KeyframeWrapper{}, err
    }
    var clone models.Game
    if err := json.Unmarshal(raw, &clone); err != nil {
        return events.KeyframeWrapper{}, err
    }

    slim := svc.Sanitize(&clone)
    if slim.PublicData != nil {
        delete(*slim.PublicData, "layout")
    }

    slimMap, _ := events.ToMap(slim)
    // Hash the slim keyframe's JSON directly. encoding/json sorts map keys
    // deterministically, so the same schema always produces the same sv hash.
    // Do NOT hash posMap (map[string]int) — Go maps have non-deterministic iteration.
    svRaw, _ := json.Marshal(slimMap)
    svHash := sha256.Sum256(svRaw)
    sv := hex.EncodeToString(svHash[:8])

    return events.KeyframeWrapper{SV: sv, Game: slim}, nil
}

// WriteKeyframe sends a layout frame then a slim keyframe wrapper.
// Call on join, create, and reconnect.
func (svc *Service) WriteKeyframe(conn transport.Conn, g *models.Game, layoutHash string, layoutData map[string]interface{}) error {
    layoutFrame := events.LayoutFrame{O: events.OpLayout, V: layoutHash, Data: layoutData}
    if err := ws.WriteEncoded(conn, layoutFrame); err != nil {
        return err
    }
    wrapper, err := svc.slimKeyframe(g)
    if err != nil {
        return err
    }
    return ws.WriteEncoded(conn, wrapper)
}

// WriteSlimKeyframe sends only the slim keyframe wrapper (no layout frame).
// Call on refresh when the client already has the layout.
func (svc *Service) WriteSlimKeyframe(conn transport.Conn, g *models.Game) error {
    wrapper, err := svc.slimKeyframe(g)
    if err != nil {
        return err
    }
    return ws.WriteEncoded(conn, wrapper)
}
```

- [ ] **Step 4: Update `join`, `create`, `reconnect`, `refresh` handlers**

In each handler, replace the direct `cs.Write(gameJSONBytes)` call with `h.i.GameService.WriteKeyframe(s, g, h.i.LayoutHash, h.i.LayoutData)`.

For `refresh`, use `WriteSlimKeyframe` instead.

- [ ] **Step 5: Build and run tests**

```bash
go build ./... && go test ./... -race
```
Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add internal/services/events/wire.go internal/injector/ \
        internal/services/game/ internal/handlers/actions/
git commit -m "feat(protocol): layout frame + slim keyframe wrapper with schema version"
```

---

## Task 8: MessagePack transport

**Files:**
- Modify: `internal/transport/ws/ws.go`
- Modify: `go.mod` / `go.sum`

- [ ] **Step 1: Verify and add the MessagePack library**

Do not guess the import path. Search pkg.go.dev for a Go MessagePack library first:

```bash
open "https://pkg.go.dev/search?q=msgpack&m=package" 2>/dev/null || \
  echo "Open https://pkg.go.dev/search?q=msgpack&m=package in a browser"
```

Pick a well-maintained library (check stars, recent commits, and Go module compatibility). Then install it:

```bash
cd /Users/robbiebyrd/Projects/indri && go get <confirmed-import-path>@latest
```

**Do not write any import path into source code until `go get` succeeds and the module appears in `go.sum`.** Use whatever import path `go get` confirms. The rest of this task uses `msgpack "VERIFIED_IMPORT_PATH"` as a placeholder — replace with the actual path found in Step 1.

- [ ] **Step 2: Add debug-flag detection in `ws.go` HandleConnect**

In `internal/transport/ws/ws.go`, inside the `Handle` method's `HandleConnect` closure, read the debug flag from the melody session's HTTP request before calling `h.Connect`. `melody.Session` has a `Request *http.Request` field available at connect time:

```go
if h.Connect != nil {
    t.m.HandleConnect(func(s *melody.Session) {
        if s.Request != nil && s.Request.URL.Query().Get("debug") == "1" {
            s.Set("debug", true)
        }
        h.Connect(conn{s})
    })
}
```

- [ ] **Step 3: Add `WriteEncoded` to `ws.go`**

Add an exported helper that any package can call with a `transport.Conn`:
```go
import (
    "encoding/json"
    msgpack "VERIFIED_IMPORT_PATH" // replace with the path confirmed by go get in Step 1
    "github.com/robbiebyrd/indri/internal/transport"
)

// WriteEncoded writes payload as MessagePack by default, or JSON when the
// connection has the "debug" key set to true (set by ?debug=1 at upgrade).
func WriteEncoded(c transport.Conn, payload interface{}) error {
    debug, _ := c.Get("debug")
    if debug == true {
        data, err := json.Marshal(payload)
        if err != nil {
            return err
        }
        return c.Write(data)
    }
    data, err := msgpack.Marshal(payload)
    if err != nil {
        return err
    }
    return c.Write(data)
}
```

- [ ] **Step 4: Direct-write path already covered by `WriteKeyframe`/`WriteSlimKeyframe`**

The `WriteKeyframe` and `WriteSlimKeyframe` helpers added in Task 7 already call `ws.WriteEncoded`. No further changes needed for the direct-write path — keyframes and layout frames are already encoded correctly.

- [ ] **Step 5: Update `broadcast.go` — encode delta per-connection**

The broadcast path for deltas goes through `internal/services/broadcast/broadcast.go`. The existing `broadcastToSessions(sessionIds []string, jsonData []byte)` method calls `bs.t.BroadcastFilter(jsonData, ...)` which sends the same bytes to all matched sessions. Per-connection encoding requires iterating connections individually.

Replace `broadcastToSessions` with a version that uses `bs.t.Conns()` and `ws.WriteEncoded`:

```go
import (
    "slices"
    ws "github.com/robbiebyrd/indri/internal/transport/ws"
)

// broadcastToSessions sends payload to connections whose "sessionId" key is in sessionIds.
// Encoding is per-connection so debug-mode connections receive JSON while others receive MessagePack.
func (bs *Service) broadcastToSessions(sessionIds []string, payload interface{}) error {
    if len(sessionIds) == 0 {
        return nil
    }

    conns, err := bs.t.Conns()
    if err != nil {
        return err
    }

    for _, conn := range conns {
        value, ok := conn.Get("sessionId")
        if !ok {
            continue
        }
        id, ok := value.(string)
        if !ok || !slices.Contains(sessionIds, id) {
            continue
        }
        if err := ws.WriteEncoded(conn, payload); err != nil {
            log.Printf("broadcast write error to session %s: %v", id, err)
        }
    }
    return nil
}
```

Because `broadcastToSessions`'s parameter changes from `jsonData []byte` to `payload interface{}`, also update all call sites — `sendToGame`, `sendToTeam`, `sendToPlayer`, `sendToPlayers` — to pass the raw payload instead of pre-marshaling. Remove the `json.Marshal(data)` calls in `Broadcast`, `BroadcastToPlayer`, and `BroadcastToPlayers`; pass `data` directly instead of `jsonData`. The `sendToAll` (which calls `bs.t.Broadcast`) still needs to marshal — keep it as-is.

Keep error writes in handlers as plain JSON — errors are always human-readable and don't need binary encoding.

- [ ] **Step 6: Build and run**

```bash
go build ./... && go test ./... -race
```

- [ ] **Step 7: Commit**

```bash
git add internal/transport/ws/ws.go internal/services/broadcast/broadcast.go \
        internal/services/game/ go.mod go.sum
git commit -m "feat(transport): MessagePack encoding with JSON debug fallback on ?debug=1"
```

---

## Task 9: Boot-time config validation + tictactoe config fix

**Files:**
- Modify: `internal/services/boot/boot.go`
- Modify: `example/tictactoe/config.json`
- Modify: `example/tictactoe/server/handlers/move/handler.go`
- Modify: `example/tictactoe/server/handlers/restart/handler.go`

- [ ] **Step 1: Add `winningTeam: null` to tictactoe config**

In `example/tictactoe/config.json`, update `stage.scenes.board.data`:
```json
"data": {
  "board": [["","",""],["","",""],["","",""]],
  "winningTeam": null
}
```

- [ ] **Step 2: Fix `sceneHasWinner` in the move handler**

`sceneHasWinner` currently does `_, ok := sceneData["winningTeam"]` which returns `true` when the key is present even with a `nil` value — after Step 1 adds the key with `null`, every move would incorrectly report "game over" before any piece is placed.

In `example/tictactoe/server/handlers/move/handler.go`, change:
```go
// Before:
func sceneHasWinner(sceneData map[string]interface{}) bool {
    _, ok := sceneData["winningTeam"]
    return ok
}

// After:
func sceneHasWinner(sceneData map[string]interface{}) bool {
    v, ok := sceneData["winningTeam"]
    return ok && v != nil
}
```

- [ ] **Step 3: Fix restart handler to clear `winningTeam` without deleting the key**

`restart/handler.go` calls `delete(updated, "winningTeam")`. After the schema-stability requirement, deleting a key emits a `removed` delta which breaks the client's positional map.

In `example/tictactoe/server/handlers/restart/handler.go`, change:
```go
// Before:
delete(updated, "winningTeam")

// After:
updated["winningTeam"] = nil
```

- [ ] **Step 4: Add boot validation**

In `internal/services/boot/boot.go`, after the script is loaded, add:
```go
func validateScript(script *models.Script) error {
    if script.Config.MaxTeams != len(script.Teams) {
        return fmt.Errorf(
            "config.maxTeams (%d) does not match number of declared teams (%d)",
            script.Config.MaxTeams, len(script.Teams),
        )
    }
    if script.Config.MaxPlayersPerTeam <= 0 {
        return fmt.Errorf("config.maxPlayersPerTeam must be > 0, got %d", script.Config.MaxPlayersPerTeam)
    }
    return nil
}
```

Call `validateScript(script)` and `log.Fatal` on error.

- [ ] **Step 5: Build and run the server briefly to verify it starts**

```bash
go build ./example/tictactoe && echo "Build OK"
```
Expected: `Build OK`.

- [ ] **Step 6: Commit**

```bash
git add internal/services/boot/boot.go example/tictactoe/config.json \
        example/tictactoe/server/handlers/move/handler.go \
        example/tictactoe/server/handlers/restart/handler.go
git commit -m "feat(boot): validate script config at startup; fix winningTeam handling for schema stability"
```

---

## Task 10: TypeScript client — models and positional map

**Files:**
- Modify: `client/models/models.ts`
- Create: `client/services/positional-map.ts`
- Create: `client/services/positional-map.node-test.ts`

- [ ] **Step 1: Update `models.ts`**

Replace the `UpdateMessage` interface and add new types. In debug mode (`?debug=1`) paths are numeric-dotted strings; in normal mode they are integer arrays. `PathSegment` accommodates both:
```typescript
// In normal (MessagePack) mode: integer array. In debug (JSON) mode: string.
export type PathSegment = number[] | string

export declare interface UpdateMessage {
    o: number           // OpCode: 1=update, 2=insert, 3=delete
    t: number           // Unix milliseconds
    u?: [PathSegment, unknown][]   // [[path, value], ...]
    r?: PathSegment[]              // [path, ...]
}

export declare interface LayoutFrame {
    o: 4
    v: string
    data: Record<string, unknown>
}

export declare interface SlimKeyframe {
    sv: string
    game: Game
}
```

Also add `userId` to the `Player` interface:
```typescript
export declare interface Player {
    userId: string
    name: string
    score: number
    connected: boolean
    host: boolean
    controller: boolean
    data?: Record<string | number, any>
    privateData?: Record<string | number, any>
}
```

- [ ] **Step 2: Create failing tests for `buildPositionalMap`**

Create `client/services/positional-map.node-test.ts`:
```typescript
import test from "node:test"
import assert from "node:assert/strict"
import { buildPositionalMap, resolvePath } from "./positional-map.ts"

test("buildPositionalMap sorts keys alphanumerically", () => {
    const obj = { stage: {}, players: {}, code: "X", data: {} }
    const map = buildPositionalMap(obj)
    assert.equal(map["code"], 0)
    assert.equal(map["data"], 1)
    assert.equal(map["players"], 2)
    assert.equal(map["stage"], 3)
})

test("resolvePath converts integer array to string dot-path", () => {
    const obj = { a: { b: { c: "v" } } }
    const posMap = buildPositionalMap(obj)
    // a=0, a.b=0, a.b.c=0
    const result = resolvePath([0, 0, 0], obj, posMap)
    assert.equal(result, "a.b.c")
})

test("resolvePath passes array indices through unchanged", () => {
    const obj = { board: [["", ""], ["", ""]] }
    const posMap = buildPositionalMap(obj)
    // board=0, then raw array indices 1, 1
    const result = resolvePath([0, 1, 1], obj, posMap)
    assert.equal(result, "board.1.1")
})
```

- [ ] **Step 3: Run to verify they fail**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm test 2>&1 | grep -E "(FAIL|positional-map)"
```
Expected: file not found / fail.

- [ ] **Step 4: Implement `positional-map.ts`**

Create `client/services/positional-map.ts`:
```typescript
export type PositionalMap = Record<string, number>

export function buildPositionalMap(obj: Record<string, unknown>, prefix = ""): PositionalMap {
    const result: PositionalMap = {}
    const keys = Object.keys(obj).sort()

    for (let i = 0; i < keys.length; i++) {
        const key = keys[i]
        const path = prefix ? `${prefix}.${key}` : key
        result[path] = i

        const child = obj[key]
        if (child !== null && typeof child === "object" && !Array.isArray(child)) {
            const nested = buildPositionalMap(child as Record<string, unknown>, path)
            Object.assign(result, nested)
        }
    }

    return result
}

// resolvePath converts a positional integer-array path back to a dotted string
// by walking schema to determine at each level whether the container is an
// object (use positional map) or array (pass index through raw).
export function resolvePath(
    path: number[],
    schema: unknown,
    posMap: PositionalMap,
    prefix = ""
): string {
    const parts: string[] = []
    let current: unknown = schema

    for (const seg of path) {
        if (Array.isArray(current)) {
            // Array container: raw numeric index
            parts.push(String(seg))
            current = (current as unknown[])[seg]
        } else if (current !== null && typeof current === "object") {
            // Object container: look up key by sorted index
            const keys = Object.keys(current as object).sort()
            const key = keys[seg]
            if (key === undefined) {
                parts.push(String(seg))
                current = undefined
            } else {
                parts.push(key)
                current = (current as Record<string, unknown>)[key]
            }
        } else {
            parts.push(String(seg))
            current = undefined
        }
    }

    return parts.join(".")
}
```

- [ ] **Step 5: Run tests**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm test
```
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add client/models/models.ts client/services/positional-map.ts client/services/positional-map.node-test.ts
git commit -m "feat(client): positional map + updated models for compact protocol"
```

---

## Task 11: Client — GameStateParser positional path support

**Files:**
- Modify: `client/services/game-state-parser.ts`
- Modify: `client/services/game-state-parser.node-test.ts`

- [ ] **Step 1: Write failing tests for positional path application**

Add to `game-state-parser.node-test.ts`:
```typescript
import { buildPositionalMap } from "./positional-map.ts"

test("positional integer-array path applies correctly", () => {
    const schema = { board: [["", "", ""], ["", "", ""], ["", "", ""]] }
    const posMap = buildPositionalMap(schema)
    const p = new GameStateParser<any>()
    p.setSchema(schema)
    p.set(JSON.parse(JSON.stringify(schema)), new Date(1000))

    // board is key 0 at root; then raw array indices 1, 1
    p.update({o: 1, t: 2000, u: [[[0, 1, 1], "X"]], r: []})
    assert.equal(p.current().board[1][1], "X", "positional path applied")
})

test("debug mode numeric-dotted path applies correctly", () => {
    const schema = { board: [["", "", ""], ["", "", ""], ["", "", ""]] }
    const p = new GameStateParser<any>()
    p.setSchema(schema)
    p.set(JSON.parse(JSON.stringify(schema)), new Date(1000))

    // Debug mode: string path "0.1.1" (root key 0 = board, then array[1][1])
    p.update({o: 1, t: 2000, u: [["0.1.1", "O"]], r: []})
    assert.equal(p.current().board[1][1], "O", "debug dotted-numeric path applied")
})
```

Also update the existing `delta()` helper at the top of the test file to use the new pair-array format:
```typescript
function delta(ts: number, updated?: [PathSegment, unknown][], removed?: PathSegment[]): any {
    return {o: 1, t: ts, u: updated, r: removed}
}
```

Then update every existing test call site that uses the old `delta(ts, {key: val})` map format. The existing test file (`game-state-parser.node-test.ts`) has 6 call sites; 5 need to change (line 55 is already correct):

```typescript
// Line 17: delta(2000, {n: 2})
delta(2000, [["n", 2]])

// Line 24: delta(3000, {x: "new"})
delta(3000, [["x", "new"]])

// Line 31: delta(5000, {a: 1})
delta(5000, [["a", 1]])

// Line 40: delta(2000, {"__proto__.polluted": "yes"})
delta(2000, [["__proto__.polluted", "yes"]])

// Line 47: delta(2000, {"board.0.1": "X"})
delta(2000, [["board.0.1", "X"]])

// Line 55: delta(2000, undefined, ["b"])  — no change needed
// removed paths are already strings; they remain strings in the new format
```

The parser's `toDotPath` handles plain string paths (non-numeric) as pass-through, so these string-path tests remain correct after the parser is updated.

- [ ] **Step 2: Run to verify they fail**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm test 2>&1 | grep -E "FAIL|positional"
```

- [ ] **Step 3: Update `GameStateParser`**

Modify `client/services/game-state-parser.ts`:

```typescript
import { buildPositionalMap, resolvePath, type PositionalMap } from "./positional-map.ts"
import type { UpdateMessage } from "@/models/models"

// ... keep existing UNSAFE_KEYS and deepClone ...

export class GameStateParser<T> {
    private deltas: Delta[] = []
    private cutoff: number = new Date(0).getTime()
    private baseState?: T = undefined
    private currentState?: T = undefined
    private schema?: Record<string, unknown> = undefined
    private posMap?: PositionalMap = undefined

    setSchema(schema: Record<string, unknown>): void {
        this.schema = schema
        this.posMap = buildPositionalMap(schema)
    }

    set(data: T, timestamp: Date): void {
        this.setCutoff(timestamp)
        this.baseState = deepClone(data)
        if (!this.schema) {
            this.setSchema(data as Record<string, unknown>)
        }
        this.deleteBefore(timestamp)
        this.reapply()
    }

    // ... keep sort(), current(), deleteBefore(), setCutoff() unchanged ...

    update(data: UpdateMessage): void {
        const timestamp = new Date(data.t)
        if (timestamp.getTime() < this.cutoff) return
        this.deltas.push({data, timestamp})
        this.sort()
        this.reapply()
    }

    private reapply(): void {
        if (this.baseState === undefined) {
            this.currentState = undefined
            return
        }
        let state: T = deepClone(this.baseState)

        for (const updateMsg of this.deltas) {
            if (updateMsg.data.r) {
                for (const rawPath of updateMsg.data.r) {
                    const dotPath = this.toDotPath(rawPath)
                    state = this.deleteJSONKeyByDotPath(state, dotPath)
                }
            }
            if (updateMsg.data.u) {
                for (const [rawPath, value] of updateMsg.data.u) {
                    const dotPath = this.toDotPath(rawPath)
                    state = this.updateJSONKeyByDotPath(state, dotPath, value)
                }
            }
        }

        this.currentState = state
    }

    private toDotPath(rawPath: unknown): string {
        if (typeof rawPath === "string") {
            // Debug mode: numeric-dotted string like "0.1.1" or regular string "stage.currentScene"
            if (this.schema && /^\d/.test(rawPath)) {
                const ints = rawPath.split(".").map(Number)
                return resolvePath(ints, this.schema, this.posMap ?? {})
            }
            return rawPath
        }
        if (Array.isArray(rawPath) && this.schema) {
            return resolvePath(rawPath as number[], this.schema, this.posMap ?? {})
        }
        return String(rawPath)
    }

    // Keep updateJSONKeyByDotPath and deleteJSONKeyByDotPath unchanged.
}
```

- [ ] **Step 4: Run all client tests**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm test
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add client/services/game-state-parser.ts client/services/game-state-parser.node-test.ts
git commit -m "feat(client): GameStateParser resolves positional integer-array paths"
```

---

## Task 12: Client message handler — MessagePack + new classifier

**Files:**
- Modify: `client/services/message-handler.ts`
- Modify: `client/package.json` (add `@msgpack/msgpack`)

- [ ] **Step 1: Add MessagePack dependency**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm add @msgpack/msgpack
```

- [ ] **Step 2: Update `message-handler.ts`**

Key changes:

1. Import and decode MessagePack:
```typescript
import { decode } from "@msgpack/msgpack"
```

2. Update `routeIncomingMessage` to handle binary data:
```typescript
routeIncomingMessage(message: MessageEvent) {
    let parsed: unknown
    if (message.data instanceof ArrayBuffer) {
        parsed = decode(new Uint8Array(message.data))
    } else {
        parsed = parseJsonSafely<JsonObject>(message.data)
    }
    // ...
}
```

3. Update WebSocket to receive binary:
```typescript
this.ws.binaryType = "arraybuffer"
```

4. Rewrite `messageType()` with new classifier:
```typescript
messageType(parsedMessage: any): string | undefined {
    if (typeof parsedMessage !== "object" || parsedMessage === null) return undefined
    if ("authenticated" in parsedMessage && parsedMessage.authenticated === true) return "authenticated"
    if ("sv" in parsedMessage) return "keyframe"        // slim keyframe wrapper
    if ("o" in parsedMessage && parsedMessage.o === 4)  return "layout"
    if ("o" in parsedMessage && parsedMessage.o === 1)  return "update"
    if ("o" in parsedMessage && parsedMessage.o === "inquiryResponse") return "inquiryResponse"
    if ("disconnected" in parsedMessage) return "disconnect"
    return undefined
}
```

5. Add `layout` to the parsers array:
```typescript
{
    name: "indri_layout",
    action: "layout",
    parser: (d) => this.handleLayout(d)
},
{
    name: "indri_keyframe",
    action: "keyframe",
    parser: (d) => this.keyframe(d)
},
```

6. Add `handleLayout` and update `keyframe`:
```typescript
private cachedLayout?: { v: string; data: Record<string, unknown> }

handleLayout(msg: any) {
    if (!this.cachedLayout || this.cachedLayout.v !== msg.v) {
        this.cachedLayout = { v: msg.v, data: msg.data }
    }
}

keyframe(wrapperData: any) {
    const g = wrapperData.game as Game
    // Build the positional schema from the SLIM keyframe (no layout).
    // The server strips layout before computing the positional map, so the
    // client must do the same — layout keys must not be in the positional map.
    this.stateList.setSchema(g as Record<string, unknown>)

    // Now merge layout back into the game object for rendering.
    if (this.cachedLayout) {
        if (!g.data) g.data = {}
        g.data.layout = this.cachedLayout.data as any
    }
    this.stateList.set(g as JsonObject, new Date(g.updatedAt ?? new Date().toISOString()))
    this.updateGameState()
}
```

- [ ] **Step 3: Run typecheck and tests**

```bash
cd /Users/robbiebyrd/Projects/indri/client && pnpm run typecheck && pnpm test
```
Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add client/services/message-handler.ts client/package.json client/pnpm-lock.yaml
git commit -m "feat(client): MessagePack decode, new classifier, layout frame handling"
```

---

## Task 13: Protocol docs update

**Files:**
- Modify: `docs/PROTOCOL.md`

- [ ] **Step 1: Update the Server → client section of `docs/PROTOCOL.md`**

Add sections for:
- Layout frame (`o: 4`) — fields `o`, `v`, `data`
- Slim keyframe wrapper — fields `sv`, `game`
- New delta format — fields `o`, `t`, `u` (pair-array), `r` (path array), opcode table
- Debug mode — `?debug=1` query param
- Positional path encoding — how paths are derived from slim keyframe schema

- [ ] **Step 2: Run full test suite one final time**

```bash
cd /Users/robbiebyrd/Projects/indri
go test ./... -race
cd client && pnpm run typecheck && pnpm test
```
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add docs/PROTOCOL.md
git commit -m "docs(protocol): document compact delta format, layout frame, positional paths"
```

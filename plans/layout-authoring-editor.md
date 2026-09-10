# Plan B: Layout authoring — server action & editor

**Created:** 2026-09-09 | **Status:** Draft | **Effort:** L | **Branch:** `POC-00001/layout-authoring-editor`

> Plan 2 of 2. **Depends on Plan A** (`plans/layout-engine-renderer.md`) for the schema, grid/collision
> core, widget registry and renderer. Do not start before Plan A's Step 8 is green.
>
> Together with Plan A this supersedes the combined `plans/layout-widget-engine.md` (deleted; never
> committed). Lineage is recorded here rather than in mdkb — this repo has no mdkb index.

## Summary

Make layouts editable at runtime. A new host-only Go `layout` action writes into `game.data.layout` through
`GameRepo.Mutate`, so every edit publishes a delta and reaches every connected player like any other state
change. On the client, a host can drag, resize, add and remove widgets on the board, and edit each widget's
configuration through a panel auto-drawn from the field descriptors declared in Plan A.

## Architecture Context

- One action, many ops. The handler mutates the in-memory `*models.Game` inside `Mutate`; `events.Diff` then
  computes the minimal delta for free. A whole-layout replace would re-send every widget on every edit and
  waste the entire delta pipeline.
- Writes are confined to `game.PublicData["layout"]`. Nothing else in the game document is reachable.
- **Handlers are now connection-independent** (`internal/handlers/actions/handler.go`). A handler takes an
  `actions.Request{Session, Payload}` and returns an `actions.Result`. The transport authenticates and
  resolves the session *before* dispatch, so a handler cannot get caller identity wrong — the old "resolve
  from your own connection's `sessionId`, never from a client-supplied `userId`" rule is now structural
  rather than a discipline each handler had to keep. `kick` is still the shape to copy.
- **Two transports serve the same handlers.** `internal/transport/ws` and `internal/transport/graphql` both
  dispatch into the same action logic, so a new action is only half-added until it exists in *both*: the WS
  action registry AND `schema.graphqls` + a resolver (gqlgen codegen). Otherwise it is silently unreachable
  for GraphQL clients.
- Structural validation (bounds, overlap, depth, reserved keys) runs **in Go**. The TS validator from Plan A
  is UX feedback; the client is not a security boundary. This duplication across two languages is
  unavoidable and intentional.
- Editor gestures compute a candidate rect, run Plan A's `canPlace()` locally for instant feedback, then
  emit the op. The server re-validates and is the authority; the local check only avoids a doomed round trip.
- Key existing files: `internal/handlers/actions/kick/handler.go` (auth + Result shape reference),
  `internal/handlers/actions/handler.go` (Request/Result/MessageHandler),
  `internal/transport/graphql/schema.graphqls` and `resolvers/schema.resolvers.go` (mutation parity),
  `internal/repo/game/game.go:235` (`Mutate`), `internal/repo/game/interface.go` (`Storer` — must be
  updated in step with any new exported method), `internal/services/mutation/mutation_test.go` (test style).

ADR: **One `layout` action with an `op` discriminator, not one action per operation.** Keeps the protocol
surface and the `Storer` interface small, and lets a single `Mutate` + `Diff` produce granular deltas.

ADR: **Go validates structure, not widget semantics.** Indri is a framework; it does not know what a "text
widget" is. Placement, bounds, overlap, depth and reserved keys are structural and enforced. Widget `config`
contents are opaque to the server and validated only by the client registry.

## Verified Go signatures

Re-verified after the transport refactor (`d03e6b0`, `204445a`) — the handler contract changed.

```go
// internal/handlers/actions/handler.go — the CURRENT handler contract.
// Session is already authenticated and resolved by the transport.
type Request struct {
    Session *models.Session        // nil only for register/login/reconnect
    Payload map[string]interface{}
}
type Result struct {
    Responses     [][]byte  // messages back to the caller, in order
    Session       *models.Session
    DisconnectIDs []string
}
type MessageHandler interface{ Handle(Request) (Result, error) }

// internal/repo/game/game.go — note: Mutate takes NO context
func (s *Store) Mutate(id string, apply func(g *models.Game) error) error
func (s *Store) UpdateField(id string, key string, value interface{}) error
func (s *Store) PlayerIsHost(id string, playerId string) bool
// internal/services/mutation — apply may return this to skip the write entirely
var mutation.ErrAbort = errors.New("mutation aborted")
// internal/models/game.go
PublicData map[string]interface{} `bson:"data" json:"data,omitempty"`
```

Gone since the first draft: `*melody.Session`, `h.i.MelodyClient`, `connection.NewService(...)` and
`cs.GetKeyAsString("sessionId")`. A handler no longer touches the connection at all.

## Op vocabulary

| `op` | Required fields | Writes | Resulting delta path |
|---|---|---|---|
| `addWidget` | `sceneId`, `widgetId`, `widget` | `scenes[s].widgets[w]` | `data.layout.scenes.S.widgets.W` (whole object) |
| `removeWidget` | `sceneId`, `widgetId` | deletes the key | `removed: [data.layout.scenes.S.widgets.W]` |
| `setPlacement` | `sceneId`, `widgetId`, `placement` | `…widgets[w].placement` | `…widgets.W.placement.col` etc. (leaf paths) |
| `setWidgetConfig` | `sceneId`, `widgetId`, `config` | merges into `…widgets[w].config` | `…widgets.W.config.text` |
| `setStyle` | `scope`, `sceneId?`, `widgetId?`, `style` | board / scene / widget `style` | `data.layout.style.*` etc. |
| `setGrid` | `grid` | `layout.grid` | `data.layout.grid.cols` |
| `setScript` | `scope`, `sceneId?`, `widgetId?`, `source` | the matching `script` | `data.layout.scenes.S.script` |

`setPlacement` covers both move and resize — they write the same field, so a separate `resizeWidget` op would
be two names for one write.

## Authorization matrix

| Condition | Result |
|---|---|
| `req.Session == nil` (unauthenticated) | reject — "must be logged in to edit a layout" |
| `req.Session.UserID == nil` | reject — "calling session has no user id" |
| `req.Session.GameID` ≠ the target game | reject — "caller is not in game" |
| `g.Players[userId].Host` is false | reject — "caller is not the host" |
| Host, but op fails structural validation | reject with the specific issue; no write |
| Host, valid op | `Mutate` → `Diff` → publish → broadcast |

The first two rows are now cheap because the transport resolved the session already. Caller identity comes
from `req.Session` and there is no code path by which a `userId` in `req.Payload` could become the caller —
which is what makes the old spoofing risk structurally absent rather than merely tested for.

## Research Findings

- **The transport was refactored mid-plan** (`d03e6b0`, `204445a`, `f3397ef`). Handlers are now
  connection-independent and a GraphQL transport sits alongside WebSocket. Everything below was re-verified
  against the current tree, not the tree this plan was first written against.
- `internal/handlers/actions/kick/handler.go` is still the only handler doing host authorization, but now
  reads `req.Session` directly: nil check → `UserID` nil check → `GameID` match → `g.Players[uid].Host`. It
  returns `actions.Result{DisconnectIDs: [...]}` and lets the transport do the disconnecting. Copy this shape.
- `TestRegisterHandlers_CoversEveryActionPackage` (`internal/services/boot/handlers_test.go:49`) fails if a
  package under `internal/handlers/actions/` is never wired into `boot.registerHandlers` — an unregistered
  action is silently unreachable. **Note this guards the WS registry only; nothing equivalent guards GraphQL
  mutation parity**, so a layout action reachable over WS but absent from `schema.graphqls` would pass CI.
- GraphQL resolvers are thin: each mutation is one line, `r.dispatch(ctx, "<action>", payload)`
  (`resolvers/schema.resolvers.go:60`). Adding an action means a `Mutation` field in `schema.graphqls`, a
  gqlgen regeneration, and a one-line resolver. Auth comes from the Authorization bearer token there, not a
  socket key — another reason the handler must not care where the session came from.
- `internal/repo/game/interface.go` asserts `var _ Storer = (*Store)(nil)`. Adding an exported `Store` method
  without updating `Storer` breaks the build by design. This plan adds none — it uses `Mutate` only.
- `Store.Mutate` snapshots before `apply`, diffs after a committed save, and publishes automatically. No
  explicit `publish` call is needed, and none should be added.
- `events.Diff` recurses to leaf paths through nested maps but replaces arrays whole — hence the map-keyed
  widget model from Plan A. A newly added key is inserted whole, un-walked, so `addWidget` produces exactly
  one delta entry.
- `SanitizeDelta` strips any path containing a `privateData` segment at any depth.
- No off-the-shelf schema-to-form renderer works in React Native — `@rjsf/core` and `@autoform/react` both
  emit DOM elements. The two RN-specific packages found (`rhfa-react-native`, `react-native-dform`) are
  small, low-adoption, and use their own descriptor formats. A hand-rolled descriptor→component registry is
  the de facto standard, which is what Plan A's `FieldDescriptor` already is.
- RNGH 3's hook API requires RN ≥ 0.82; on RN 0.79 the current API is `Gesture.Pan()` + `GestureDetector`.
  `GestureHandlerRootView` must wrap the app root on **every** platform including web — it is currently
  absent from `client/app/_layout.tsx`.

## Non-goals (explicit)

- No undo/redo. A PoC editor without history is honest; a half-working history stack is not.
- No multi-select or group operations.
- No push-cascade collision resolution — reject-and-snap-back only, matching Plan A.
- No layout versioning or migration. The schema will change; regenerate fixtures.
- No permissions beyond host/not-host. No per-widget locking, no co-editing presence.
- No URI allow-listing (see Security).

## Security Considerations

- **This action lets a client write game state.** Authorization is host-only. Since the transport refactor
  the caller comes from `req.Session`, already authenticated before dispatch, so identity spoofing from the
  payload is structurally impossible rather than something each handler must remember to avoid. The matrix
  above is the contract.
- **Both transports must enforce the same thing.** WS authenticates from a socket key, GraphQL from an
  Authorization bearer token. The handler is indifferent, which is the point — but it means a weakness in
  either transport's session resolution reaches this action. Do not re-derive identity here to compensate.
- Write scope is `game.PublicData["layout"]` and nothing else. The handler must never touch `privateData`,
  `Players`, `Teams`, or `Stage`. Assert this in a test that mutates a full game and diffs everything else.
- Structural validation is enforced in Go. Bounds, overlap, sub-grid depth and reserved keys are rejected
  server-side; client-side checks are UX only.
- Reject any layout containing a `privateData` key at any depth — `SanitizeDelta` would silently strip it,
  so accepting it stores data that can never be read back.
- Payload size cap on the whole `layout` object (suggest 256 KB) and a widget-count cap (300, matching
  Plan A's tested ceiling). Without these, one host can push a payload that every client must deep-clone on
  every subsequent delta.
- Cap embedded script source length. A script field is arbitrary text that every client will execute.
- **Accepted residual risk, documented not fixed:** a host can set arbitrary media URIs and arbitrary Lua
  source, both of which are delivered to and executed by every player in their game. Mitigating that needs
  URI allow-listing and a script review path; neither is in scope for a PoC. Do not ship this to a public
  deployment without them.

## Performance Considerations

- Drag emits at most one op on gesture **end**, never per frame. A per-frame op would be a delta storm — one
  full deep-clone-and-replay per message on every connected client.
- Gesture position lives in Reanimated shared values on the UI thread; only the committed rect crosses to JS
  via `runOnJS`.
- Go-side overlap validation is O(n²) over one grid level. At the 300-widget cap this is bounded and runs
  once per op, inside the lock — acceptable, but it is inside `Mutate`'s critical section, so keep it
  allocation-free where easy.
- `Mutate` retries up to 10 times under contention before `ErrConflict`. Two hosts cannot exist in one game,
  so layout-vs-layout contention is impossible; layout-vs-gameplay contention is possible and handled.

## Open Questions

### Important (P2)
1. Should `removeWidget` on a non-existent id be an error or a no-op? Default: return `mutation.ErrAbort` so
   no write and no delta occur — idempotent delete is friendlier for a client retrying after a reconnect.
2. Should the editor be a separate route (`/board/edit`) or a mode toggle on `/board`? Default: a mode toggle
   on the live board, so the host edits against real state and sees deltas land while editing.

### Unresolved / the fog
- Whether non-host players should ever edit anything (e.g. their own player-scoped widgets). Waiting on: a
  game that actually needs it. Do not generalise the permission model speculatively.

## Steps

### Step 1: op envelope — types and decoding
- **Test:** `internal/handlers/actions/layout/op_test.go` — each op in the vocabulary table decodes from its
  wire form; a missing required field is a specific error naming the field; an unknown `op` is rejected; an
  unknown top-level key is rejected. Table-driven, matching the style of
  `internal/services/mutation/mutation_test.go`.
- **Implement:** `internal/handlers/actions/layout/op.go`
- **Code:**
```go
type Op struct {
    Op        string                 `json:"op"`
    SceneID   string                 `json:"sceneId,omitempty"`
    WidgetID  string                 `json:"widgetId,omitempty"`
    Scope     string                 `json:"scope,omitempty"`   // board|scene|widget
    Widget    map[string]interface{} `json:"widget,omitempty"`
    Placement map[string]interface{} `json:"placement,omitempty"`
    Config    map[string]interface{} `json:"config,omitempty"`
    Style     map[string]interface{} `json:"style,omitempty"`
    Grid      map[string]interface{} `json:"grid,omitempty"`
    Source    *string                `json:"source,omitempty"`
}

func decodeOp(msg map[string]interface{}) (*Op, error)
```
- **Constraint:** errors wrap with `%w` and lowercase context, per repo convention:
  `fmt.Errorf("decoding layout op %q: %w", name, err)`.
- **Validation:** `go test ./internal/handlers/actions/layout/ && go vet ./...`

### Step 2: structural layout validator (Go)
- **Test:** `internal/handlers/actions/layout/validate_test.go` — table-driven over: grid dims below 8 and
  above 4096; negative/zero/fractional `w`/`h`; rect exceeding grid bounds; two overlapping non-absolute
  widgets; two overlapping **absolute** widgets (allowed); absolute placement containing a pixel value
  (rejected); sub-grid at depth 4 (allowed) and 5 (rejected); a `privateData` key nested three levels deep
  (rejected); a widget count over 300; a serialised layout over 256 KB.
- **Implement:** `internal/handlers/actions/layout/validate.go`
- **Code:**
```go
const (minDim, maxDim = 8, 4096; maxDepth = 4; maxWidgets = 300; maxBytes = 256 << 10)

// Same AABB test as the client's collides(); edge-touching is not a collision.
func overlaps(a, b rect) bool {
    return a.col < b.col+b.w && b.col < a.col+a.w &&
           a.row < b.row+b.h && b.row < a.row+a.h
}

// validateLayout walks the whole layout after the op has been applied in memory,
// so an op can never leave the document in a state the renderer must defend against.
func validateLayout(layout map[string]interface{}) error
```
- **Constraint:** validate the layout **after** applying the op, not the op in isolation. Only the resulting
  document tells you whether an overlap now exists.
- **Constraint:** absolute placements are excluded from the overlap set entirely — as subject and obstacle.
- **Constraint:** this deliberately duplicates the TS validator from Plan A. They cannot share code across
  languages; keep the constants and the AABB comparison textually identical so a future reader sees the pair.
- **Constraint:** do not validate widget `config` contents. The server does not know widget types (ADR).
- **Validation:** `go test -race ./internal/handlers/actions/layout/`

### Step 3: the `layout` handler — auth, mutate, register
- **Depends on:** Steps 1, 2
- **Test:** `internal/handlers/actions/layout/handler_test.go` — a nil `req.Session` rejects; a session in a
  different game rejects; non-host rejects; **a `userId` in `req.Payload` claiming host is ignored** (it is
  not a code path, so the test pins that it stays one); a valid `addWidget` mutates only `data.layout` and
  leaves `players`, `teams`, `stage` and `privateData` byte-identical; `removeWidget` on a missing id returns
  `mutation.ErrAbort` and produces no write.
- **Implement:** `internal/handlers/actions/layout/handler.go`; register in `boot.registerHandlers`; add the
  `layout` mutation to `internal/transport/graphql/schema.graphqls` and a resolver.
- **Code:**
```go
// Connection-independent: the transport authenticated the caller and resolved
// the session before dispatch, so identity cannot be spoofed from the payload.
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
    if req.Session == nil || req.Session.UserID == nil {
        return actions.Result{}, fmt.Errorf("must be logged in to edit a layout")
    }

    gameCode, err := handlerUtils.RequireGameCode(req.Payload)
    if err != nil { return actions.Result{}, err }

    g, err := h.i.GameService.GetByCode(*gameCode)
    if err != nil { return actions.Result{}, err }
    gameId := g.ID.Hex()

    if req.Session.GameID == nil || *req.Session.GameID != gameId {
        return actions.Result{}, fmt.Errorf("caller %v is not in game %v", *req.Session.UserID, *gameCode)
    }
    if !g.Players[*req.Session.UserID].Host {
        return actions.Result{}, fmt.Errorf("caller %v is not the host of game %v", *req.Session.UserID, *gameCode)
    }

    op, err := decodeOp(req.Payload)
    if err != nil { return actions.Result{}, err }

    // Mutate diffs before/after and publishes automatically — do not publish here.
    // The delta IS the response; nothing goes in Result.Responses.
    return actions.Result{}, h.i.GameRepo.Mutate(gameId, func(g *models.Game) error {
        return applyLayoutOp(g, op)   // writes only g.PublicData["layout"], then validates
    })
}
```
```graphql
# schema.graphqls — without this the action is unreachable for GraphQL clients
layout(code: String!, op: String!, args: JSON): JSON
```
- **Constraint:** `Mutate` takes **no context** — do not invent one. It publishes on commit; adding an
  explicit publish would double-broadcast.
- **Constraint:** `applyLayoutOp` returns `mutation.ErrAbort` when the op is a no-op, so no version bump and
  no delta occur.
- **Constraint:** the package must be registered in `boot.registerHandlers` or
  `TestRegisterHandlers_CoversEveryActionPackage` fails.
- **Constraint:** **the action is not done until BOTH transports expose it.** Nothing in CI catches a missing
  GraphQL mutation, so add the schema field, run gqlgen, and add the one-line
  `r.dispatch(ctx, "layout", ...)` resolver in the same change. Consider extending the coverage test to
  assert WS/GraphQL parity — that gap is worth closing while it is small.
- **Constraint:** errors are returned, not written to a connection. The transport decides how to surface
  them, which is why `models.WSError`/`WriteError` no longer appear here.
- **Validation:** `go test -race ./... && go vet ./...`

### Step 4: client layout mutation sender
- **Depends on:** Step 3, Plan A Step 4
- **Test:** `client/layout/edit/ops.node-test.ts` — each op builder emits the exact wire shape from the
  vocabulary table; a move that fails `canPlace()` is **not** sent; a move onto an absolute widget **is**
  sent (absolutes don't block); a send before the socket opens is dropped with a warning.
- **Implement:** `client/layout/edit/ops.ts`
- **Code:**
```ts
export function moveWidget(ws: MessageHandler, ctx: EditContext, id: string, next: GridRect): boolean {
    if (!canPlace(next, id, ctx.siblings, ctx.grid)) return false   // local reject; avoids a doomed round trip
    ws.send({action: "layout", op: "setPlacement", code: ctx.gameCode,
             sceneId: ctx.sceneId, widgetId: id, placement: {kind: "grid", ...next}})
    return true
}
```
- **Constraint:** the local check is UX only. The server re-validates and is the authority; never treat a
  local pass as success. Apply nothing optimistically — the delta is the confirmation.
- **Constraint:** widget ids for `addWidget` are generated client-side as short stable strings. Do not reuse
  an id that already exists in the scene; a collision would overwrite silently.
- **Validation:** `pnpm test`

### Step 5: editor — drag and resize
- **Depends on:** Step 4
- **Test:** snapping and rejection logic is covered by Steps 4 and Plan A Step 4; the gesture layer itself is
  visual.
- **Implement:** `client/components/board/editor/drag-resize.tsx`; add `GestureHandlerRootView` to
  `client/app/_layout.tsx`; add an edit-mode toggle to `client/app/board/index.tsx`.
- **Code:**
```tsx
const drag = Gesture.Pan()
    .onChange(e => { tx.value += e.changeX; ty.value += e.changeY })
    .onEnd(() => runOnJS(commit)())        // one op per gesture, never per frame

function commit() {
    const next = snapToCell(tx.value, ty.value, grid, size)
    if (!moveWidget(ws, ctx, id, next)) { tx.value = 0; ty.value = 0 }   // reject → snap back
}
```
- **Constraint:** `GestureHandlerRootView` must wrap the app root on every platform including web. It is
  currently absent from `_layout.tsx`; without it gestures silently do nothing.
- **Constraint:** stay on `Gesture.Pan()` + `GestureDetector`. RNGH 3's hook API needs RN ≥ 0.82.
- **Constraint:** emit exactly one op on gesture end. A per-frame op is a delta storm that forces a full
  deep-clone-and-replay on every connected client, per Plan A's performance note.
- **Constraint:** draw the grid-line overlay only when `cols*rows <= 4096`. At 4096×4096 it would be 8192
  lines; suppress it and show the dimensions as text instead.
- **Constraint:** shared values stay on the UI thread; only the committed rect crosses via `runOnJS`.
- **Visual — requires human verification:** drag feel, snap threshold, resize-handle hit targets (minimum
  44pt on touch), rejection feedback.
- **Validation:** `pnpm run typecheck && pnpm run lint`

### Step 6: editor — widget palette, add and remove
- **Depends on:** Step 4
- **Test:** `client/layout/edit/palette.node-test.ts` — the palette lists exactly the registered widget
  types; a new widget is placed at the first free rect scanning row-major; a full grid returns no placement
  rather than overlapping; the emitted `addWidget` payload validates against the widget's own schema using
  its registry `defaults`.
- **Implement:** `client/components/board/editor/palette.tsx`, `client/layout/edit/place.ts`
- **Code:**
```ts
// row-major first-fit; pure rect math, no cell array even at 4096x4096
export function firstFree(size: {w: number; h: number}, sibs: PlacedWidget[], g: GridSize): GridRect | undefined {
    for (let row = 0; row + size.h <= g.rows; row++)
        for (let col = 0; col + size.w <= g.cols; col++) {
            const r = {col, row, ...size}
            if (canPlace(r, "", sibs, g)) return r
        }
    return undefined
}
```
- **Constraint:** `firstFree` is O(cols × rows × n) in the worst case, which at 4096×4096 is far too slow to
  run naively. Bound the scan to `min(rows, 64)` × `min(cols, 64)` from the origin and fail over to "place
  at 0,0 as absolute" — a first-fit search across 16.7M positions is not worth doing well for a PoC.
- **Constraint:** the palette is generated from the widget registry, so a newly registered widget appears
  with no palette change.
- **Visual — requires human verification:** palette layout, drag-from-palette affordance if added.
- **Validation:** `pnpm test`

### Step 7: config panel — descriptor to picker
- **Depends on:** Plan A Step 6
- **Test:** `client/layout/edit/panel.node-test.ts` — every `FieldDescriptor["kind"]` maps to a registered
  picker component (exhaustive over the union, so adding a kind without a picker fails the build/test); a
  value edited through a descriptor produces a `setWidgetConfig` payload that validates against the widget's
  schema; an invalid value is reported and **not** sent.
- **Implement:** `client/components/board/editor/config-panel.tsx`
- **Code:**
```tsx
const PICKERS: {[K in FieldDescriptor["kind"]]: React.ComponentType<PickerProps>} = {
    text: TextPicker, number: NumberPicker, color: ColorPicker, boolean: BooleanPicker,
    date: DatePicker, select: SelectPicker, multiselect: MultiSelectPicker, uri: UriPicker,
}   // mapped type over the union: a new kind without a picker is a type error

export function ConfigPanel({def, value, onChange}: Props) {
    return <>{def.fields.map(f => {
        const P = PICKERS[f.kind]
        return <P key={f.key} field={f} value={value[f.key]} onChange={v => onChange(f.key, v)}/>
    })}</>
}
```
- **Constraint:** the `PICKERS` mapped type is the enforcement mechanism — it makes "descriptor kind with no
  picker" unrepresentable rather than a runtime surprise.
- **Constraint:** validate against the widget's zod schema before emitting the op. The panel is the one place
  a human types arbitrary values.
- **Constraint:** debounce text/number edits (250 ms) into a single op. Per-keystroke ops are a delta storm.
- **Visual — requires human verification:** panel layout, label alignment, error display.
- **Validation:** `pnpm test && pnpm run typecheck`

### Step 8: the pickers
- **Depends on:** Step 7
- **Test:** `client/layout/edit/pickers.node-test.ts` — pure parse/format helpers only: colour string
  parse/normalise (`#rgb` → `#rrggbb`, reject invalid), number clamp to `min`/`max`, date ISO round-trip,
  multiselect value dedupe and order stability.
- **Implement:** `client/components/board/editor/pickers/{text,number,color,boolean,date,select,multiselect,uri}.tsx`
- **Code:**
```tsx
// SelectPicker reuses the existing dependency-free control rather than adding a library
import Select from "@/components/display/select"
```
- **Constraint:** `client/components/display/select.tsx` already exists as a cross-platform, dependency-free
  replacement for `react-select`. Reuse it for `select` and extend it for `multiselect`; do not add a picker
  library. It is also the only file in the client using `StyleSheet.create` — match its style.
- **Constraint:** no DOM inputs. `@rjsf/core` and `@autoform/react` do not work in RN; these are hand-rolled
  by necessity, not by preference.
- **Constraint:** keep parse/format logic in pure helpers beside the components so it is testable without a
  renderer — the same discipline as Plan A's core/renderer split.
- **Visual — requires human verification:** every picker on web, iOS and Android; the colour and date
  pickers especially, since they have no shared cross-platform primitive.
- **Validation:** `pnpm test && pnpm run lint`

### Step 9: documentation
- **Test:** none — documentation.
- **Implement:** `docs/PROTOCOL.md`, `docs/ARCHITECTURE.md`, `CLAUDE.md`
- **Constraint:** `docs/PROTOCOL.md` gains a `### layout` section under "Client → server" with the full op
  table and the authorization rules, matching the existing per-action format.
- **Constraint:** `CLAUDE.md` records that `game.data.layout` is the canonical layout location, that
  `privateData` is a reserved key anywhere inside it, and that the Go and TS validators are a deliberate
  duplicated pair that must be changed together.
- **Constraint:** document the accepted residual risk (host-supplied URIs and script source) where a reader
  will find it before deploying, not buried in a plan file.
- **Validation:** `go build ./... && pnpm run typecheck` (docs only, but confirm nothing else drifted)

## Acceptance Criteria

- [ ] A host drags a widget; every other connected client sees it move, delivered as a delta on
      `data.layout.scenes.*.widgets.*.placement.*` — not a whole-layout replace.
- [ ] A host adds a widget from the palette and removes it; both propagate to all clients.
- [ ] A host edits a widget's config through the auto-drawn panel; the change propagates.
- [ ] A non-host attempting any layout op is rejected with a clear error and no state change.
- [ ] A `userId` in the payload claiming host is ignored — authorization comes from `req.Session`.
- [ ] The `layout` action is reachable over **both** transports: registered in `boot.registerHandlers` for
      WebSocket, and present in `schema.graphqls` with a resolver for GraphQL.
- [ ] The server rejects overlapping non-absolute widgets, out-of-bounds rects, sub-grids past depth 4, and
      any `privateData` key — independently of the client.
- [ ] A layout op mutates `data.layout` and nothing else in the game document.
- [ ] Dragging produces exactly one op per gesture, not one per frame.
- [ ] `go test -race ./...` and `pnpm test` pass.

## Checklist (non-TDD cleanup)

- [ ] `go vet ./...`, `pnpm run lint`, `pnpm run typecheck` clean
- [ ] `layout` package registered in `boot.registerHandlers`; coverage test passes
- [ ] `docs/PROTOCOL.md` and `docs/ARCHITECTURE.md` updated; `CLAUDE.md` notes the duplicated validator pair
- [ ] Residual risk (host-supplied URIs and Lua source) documented outside this plan file
- [ ] Payload and widget-count caps enforced and their values recorded

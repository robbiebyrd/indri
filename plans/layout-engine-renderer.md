---
status: in_progress
approved_at: "2026-09-09T19:04:59.197Z"
updated: "2026-09-09T19:09:58.256Z"
started_at: "2026-09-09T19:09:58.256Z"
---
# Plan A: Layout engine & renderer (client)

**Created:** 2026-09-09 | **Status:** Draft | **Effort:** L | **Branch:** `POC-00001/layout-engine-renderer`

> Plan 1 of 2. Plan B (`plans/layout-authoring-editor.md`) adds the Go persistence action and the drag/resize
> editor on top of this. **Plan A is independently shippable**: it ends with tic-tac-toe fully playable
> through the data-driven engine, with layouts authored by hand in `config.json`.
>
> Together with Plan B this supersedes the combined `plans/layout-widget-engine.md` (deleted; never
> committed). Lineage is recorded here rather than in mdkb — this repo has no mdkb index.

## Summary

Replace the hardcoded tic-tac-toe grid in the Expo client with a data-driven layout engine. A game-level
`layout` object — grid dimensions, styling, and a per-scene widget map — arrives through the existing
keyframe+delta pipeline and renders with RN primitives on web and native. Embedded Lua (fengari) supplies
widget behaviour and acts as the binding layer between authoritative game state and widget presentation.
Read-only in this plan: nothing writes layouts back.

## Architecture Context

- Layout lives at **`game.data.layout`** — one object per game, with a `scenes` map inside it keyed by
  scene id. `stage.currentScene` selects which entry renders. Nothing is stored under `scene.data`.
- Wire path is unchanged: Go write → `events.Diff` → publish → `BroadcastService` → `MessageHandler` →
  `GameStateParser` → `gameState` context → `<BoardView/>`.
- **Pure core / thin renderer split** (mirrors react-grid-layout v2's `core/` package): all schema,
  geometry, collision, style-compile and Lua logic is dependency-free TS under `client/layout/`, runnable
  in bare Node. RN components under `client/components/board/` only render what the core computes. This is
  what makes the feature testable at all given the client has no React test renderer.
- Lua never mutates game state. It writes to a local **override layer** composited over the server layout
  at render time, and reaches the server only through `indri.send(...)` → existing `MessageHandler.send`.
- Key existing files: `client/services/game-state-parser.ts` (delta replay, deep-clone-per-message),
  `client/services/message-handler.ts` (WS + the `parsers` extension array),
  `internal/services/events/delta.go:19-45` (`diffInto`), `client/app/index.tsx` (the grid being replaced).

ADR: **Lua is the binding layer** — no declarative `$bind` expression language. Rejected a binding DSL
because a scene script already maps `data.board` → widget text, and a second expression evaluator over the
same state is redundant surface area.

ADR: **Widgets are a map keyed by id, never an array.** `diffInto` replaces arrays whole, so an array would
re-send every widget on any single change, defeating the delta pipeline this engine is built on.

ADR: **`game.data.layout` is one object holding all scenes**, not a fragment per `scene.data`. Gives Plan B
a single subtree to authorize and validate, and keeps scene-switching a pure lookup.

## The data shape

```jsonc
// game.data.layout — the whole contract, in one place
{
  "grid": { "cols": 12, "rows": 12 },                    // 8..4096 each
  "style": { "backgroundGradient": { "colors": ["#0f172a", "#1e293b"] } },
  "script": "indri.on('sceneChanged', function(id) indri.log('scene', id) end)",
  "scenes": {
    "board": {
      "style": { "padding": 8 },
      "script": "-- see Step 12",
      "widgets": {
        "title": {
          "type": "text",
          "placement": { "kind": "grid", "col": 0, "row": 0, "w": 12, "h": 2 },
          "style": { "border": { "width": 2, "color": "#334155", "radius": 8 } },
          "config": { "text": "Tic Tac Toe", "fontSize": 32, "align": "center" }
        },
        "cells": {
          "type": "subgrid",
          "placement": { "kind": "grid", "col": 3, "row": 3, "w": 6, "h": 6 },
          "config": {
            "grid": { "cols": 3, "rows": 3 },
            "widgets": {
              "c00": { "type": "text",
                       "placement": { "kind": "grid", "col": 0, "row": 0, "w": 1, "h": 1 },
                       "config": { "text": "" } }
            }
          }
        },
        "badge": {
          "type": "image",
          "placement": { "kind": "absolute",                 // % only — pixels unrepresentable
                         "left": "80%", "top": "4%", "width": "16%", "height": "16%", "z": 10 },
          "config": { "uri": "https://example/turn.png", "resizeMode": "contain" }
        }
      }
    }
  }
}
```

Because every level above is a JSON object, `diffInto` walks to leaves and a single cell update arrives as
one tight path:

```
data.layout.scenes.board.widgets.cells.config.widgets.c01.config.text  =  "X"
```

## File tree

```
client/layout/                        # pure TS, zero RN imports, node-testable
  schema/{style,placement,widget,layout}.ts
  grid/{coords,collision}.ts
  style/compile.ts
  registry/{fields,registry}.ts
  lua/{state,runtime,host-api,overrides,events,bridge}.ts
  fixtures/tictactoe.json
client/components/board/              # RN only, no logic
  {styled-box,board-view,scene-view,widget-host}.tsx
  widgets/{text,image,subgrid}/index.tsx
client/app/board/index.tsx            # live renderer route
```

## Research Findings

- `internal/services/events/delta.go:19-45` — `diffInto` recurses only while **both** sides are
  `map[string]interface{}`; arrays and scalars are whole-value replaced via `reflect.DeepEqual`. A key
  present on only one side is inserted whole, un-walked (so a newly added widget arrives as one object).
- `SanitizeDelta` strips any path containing a `privateData` segment **at any depth** — reserved name.
- Game/scene `data` is `map[string]interface{}` with **zero** server-side schema.
- `zod` is absent from the repo (new dep). `type-fest` is properly declared. `tsconfig` is `strict: true`,
  `@/* → ./*`. `app.json` has `newArchEnabled: true`, `typedRoutes: true`, `web.output: "static"`. No shared
  theme module exists in `client/`; only `components/display/select.tsx` uses `StyleSheet.create`.
- **fengari under Metro/Hermes is unverified** — no case study, issue, or article found. Core `fengari`
  declares Node-only deps (`readline-sync`, `tmp`) backing its CLI, and its `io`/`os` libs are documented
  Node-only, so `luaL_openlibs` is the likely break point. Its `lua_sethook` JS callback signature is also
  unverified. Both resolved in Step 2.
- react-grid-layout's `collides()` is the plain 4-comparison AABB test; neither RGL nor gridstack ever
  materialises cells. `preventCollision: true` = reject-and-snap-back (RGL's default is push-cascade).
  **No maintained RN equivalent of RGL exists** — this is written from scratch.
- Neither RGL nor gridstack supports *per-item* overlap exemption; it is a grid-wide mode. The `absolute`
  widget rule is an extension beyond prior art, though trivial in a rect model (exclude from the set).
- Zod v4 moved `._def` → `._zod.def` and broke real consumers (MCP TS SDK #1296); zod's library-author
  guidance forbids reaching into internals. Descriptors are hand-written; zod is validation-only.
- Lua sandbox prior art: Roblox Luau strips unsafe stdlib and makes globals read-only at VM level; WoW's
  taint-tracking is a decade-long unwon arms race. Lesson: expose a small explicit host table, do not try to
  sandbox a general environment.
- Instruction-count hooks are per-coroutine and cannot interrupt inside a host function — best-effort only.
- Media/style: `expo-av` deprecated in SDK 53 → `expo-video`; `expo-linear-gradient` is linear-only; WebM
  does not decode on iOS; `boxShadow` is cross-platform since RN 0.76 but 0.79 rejects unitless lengths;
  `expo-image` ~2.4.0 covers png/jpg/gif/webp/avif/svg on all three targets.
- CI (`.github/workflows/ci.yml:46`) runs `pnpm test`, a **hardcoded single file path** — new
  `*.node-test.ts` files silently will not run until it globs.

## Non-goals (explicit)

- No video widget. Spec says "just a sub-grid, a text and an image widget for now." Video is designed for in
  the schema (`uri` field kind accepts `"video"`) but not built.
- No radial or conic gradients, therefore **no `react-native-svg` dependency**. `expo-linear-gradient` is
  linear-only; adding SVG solely for radial fills is not worth a dependency at PoC stage.
- No declarative binding language (see ADR).
- No push-cascade collision resolution. Reject-and-snap-back only.
- No reusable/shared layout templates referenced by id — so sub-grid cycles are impossible by construction.
- No layout editing. That is Plan B.

## Security Considerations

- Lua sandbox is deny-by-default: open only `base`/`string`/`table`/`math`. Never `io`, `os`, `package`,
  `require`, `load`, `loadstring`, `dofile`, `debug`. Blocking `load` matters most — otherwise a script
  synthesises new chunks at runtime and every static guard is bypassed.
- Lua has no write path to game state by construction. The host API exposes presentation setters plus
  `indri.send`, which lands in the normal client→server action path and is validated server-side like any
  other message.
- Layout content arrives from the server and is rendered, including media URIs. In this plan layouts come
  from `config.json` (operator-authored, trusted). Plan B introduces host-authored content and inherits a
  documented residual risk there.
- `parseLayout` must not be a crash surface: malformed server data degrades to an issue list, never throws.
- Reserved key: nothing in the layout may be named `privateData` — `SanitizeDelta` would silently delete it
  in transit, producing undebuggable data loss.

## Performance Considerations

- `GameStateParser.reapply()` deep-clones the **entire** game via `JSON.parse(JSON.stringify(...))` and
  replays **all** retained deltas on **every** message. Deltas are pruned only on keyframe, and keyframes
  arrive only on create/join/refresh/reconnect. Putting layouts in game state directly inflates both terms.
  Step 13 measures it; nobody optimises before that.
- Every delta yields fresh object identity, defeating naive memoisation → widgets must be id-keyed and
  `React.memo`-wrapped, or the whole tree re-reconciles per message.
- Collision is O(n²) per check. At the PoC cap (≤300 widgets) that is ~90k comparisons — fine. No spatial
  index. RGL itself shipped its O(n log n) compactor only as an opt-in extra.
- The grid is a **coordinate space, never cells**: 4096×4096 is 16.7M cells but only ever a divisor.
- Lua runs synchronously on the UI thread. A runaway script freezes the app; the instruction budget in
  Step 9 is the only guard, and it is best-effort.

## Flow coverage

| # | Flow | Covered by |
|---|---|---|
| 1 | Cold start → keyframe → initial render | 7 |
| 2 | In-place widget update via delta, identity survives | 7, 13 |
| 3 | Widget added via delta (whole-object insert) | 5, 7 |
| 4 | Widget removed via delta (unmount, local state discarded) | 7, 10 |
| 5 | Widget resized via delta, geometry recompute | 4, 7 |
| 6 | Sub-grid recursion, depth-bounded | 5, 8 |
| 7 | Scene switch — old scene tree unmounts | 7, 10 |
| 8 | Reconnect/refresh → fresh keyframe, overrides cleared | 10 |
| 9 | Interaction → Lua → `indri.send` → server → delta closes loop | 11, 12 |
| 10 | Inbound game-specific `op` → Lua handler | 11 |
| 11 | Malformed server data → clamp/skip, never crash | 5, 7 |
| 12 | Media load states: error placeholder, unsupported format | 8 |

## Open Questions

### Critical (P1)
1. **Does fengari bundle and execute under Metro + Hermes on a real device?** Step 2 is the gate for Steps
   9-12. If it fails: Lua becomes web-only, and Step 12 falls back to a minimal declarative `$bind` on the
   `text` config key. Do not start Step 9 before Step 2 is green on both targets.

### Important (P2)
2. Does `lua_sethook` + `LUA_MASKCOUNT` work in fengari, and what is the callback signature? Resolved inside
   Step 2. If unavailable, the runaway guard degrades to a wall-clock check inside host callbacks — weaker,
   and must be documented as such rather than implied.
3. Z-order tie-break among overlapping `absolute` widgets when `z` is absent. Default: map-insertion order,
   documented as a temporary rule, not a layering feature.

### Unresolved / the fog
- Whether layouts should ever be reusable across games (a template registry keyed by id). Deliberately not
  designed for. Waiting on: a second game using the engine. Revisit then; do not invent it now.

## Steps

### Step 1: test runner globs, and new dependencies
- **Test:** `client/package.json` — `pnpm test` must discover every `*.node-test.ts`, not one hardcoded path.
- **Implement:** switch to Node's built-in test runner; add dependencies.
- **Code:**
```jsonc
"test": "node --experimental-strip-types --test \"**/*.node-test.ts\""
```
```bash
cd client && pnpm add zod && npx expo install expo-linear-gradient && pnpm add fengari fengari-interop
```
- **Constraint:** `services/game-state-parser.node-test.ts` currently prints its own PASS/FAIL and calls
  `process.exit()`. Convert it to `node:test` `test()` + `node:assert`, preserving all 6 assertions
  unchanged. Do not weaken any of them — the prototype-pollution one in particular is load-bearing.
- **Constraint:** pin what `npx expo install` resolves; do not hand-write version ranges.
- **Validation:** `pnpm test` reports the 6 existing parser assertions via the runner; `pnpm run typecheck`.

### Step 2: SPIKE — fengari under Metro, web + native
- **Test:** `client/layout/lua/state.node-test.ts` — a chunk evaluates with only base/string/table/math open;
  `io`, `os`, `require`, `load`, `loadstring`, `dofile`, `debug` all resolve to `nil`; `lua_sethook` with
  `LUA_MASKCOUNT` fires and its callback signature is recorded.
- **Implement:** `client/layout/lua/state.ts`; throwaway `client/app/board/spike.tsx`.
- **Code:**
```ts
import {lua, lauxlib, lualib, to_luastring} from "fengari"
import {luaopen_base} from "fengari/src/lbaselib.js"

const SAFE = {_G: luaopen_base, string: lualib.luaopen_string,
              table: lualib.luaopen_table, math: lualib.luaopen_math}

export function createSandboxedState(): lua_State {
    const L = lauxlib.luaL_newstate()
    for (const [name, open] of Object.entries(SAFE)) {
        lauxlib.luaL_requiref(L, to_luastring(name), open, 1)
        lua.lua_pop(L, 1)
    }
    // base opens load/loadstring/dofile — remove them explicitly.
    for (const g of ["load", "loadstring", "dofile", "rawset", "rawget"]) {
        lua.lua_pushnil(L); lua.lua_setglobal(L, to_luastring(g))
    }
    return L
}
```
- **Constraint:** never call `luaL_openlibs`. It pulls fengari's Node-only `io`/`os` paths — simultaneously
  the sandbox hole and the likely Metro bundling failure.
- **Constraint:** `luaopen_base` installs `load`/`dofile`, so selective `requiref` alone is not a sandbox.
  Nil them explicitly and assert it in the test.
- **Validation:** `pnpm test`; then `pnpm run web` **and** `pnpm run ios` (or `android`) — the spike screen
  must render a Lua-computed value on both. If Metro fails to resolve a Node builtin, record the exact
  module and try a `resolver.extraNodeModules` shim in `metro.config.js` before declaring failure.
- **Gate:** report the result before starting Step 9.

### Step 3: style schema and compiler
- **Test:** `client/layout/style/compile.node-test.ts` — a Style compiles to an RN `ViewStyle` plus an
  ordered background-layer list; layer order is gradient-then-image-then-children; unknown keys are rejected
  (`.strict()`); an unitless `boxShadow` is rejected; per-corner radius expands correctly; `padding` accepts
  both a scalar and per-edge form.
- **Implement:** `client/layout/schema/style.ts`, `client/layout/style/compile.ts`
- **Code:**
```ts
export const StyleSchema = z.object({
    backgroundColor: z.string().optional(),
    backgroundImage: z.object({uri: z.string().url(), resizeMode: ResizeMode.optional()}).optional(),
    backgroundGradient: z.object({
        colors: z.array(z.string()).min(2), locations: z.array(z.number()).optional(),
        start: Point.optional(), end: Point.optional()}).optional(),
    border: z.object({width: z.number().nonnegative(), color: z.string(),
        style: z.enum(["solid", "dotted", "dashed"]).optional(), radius: Radius.optional()}).optional(),
    padding: z.union([z.number(), Edges]).optional(),
    opacity: z.number().min(0).max(1).optional(),
    boxShadow: z.string().regex(/\d(px|em|rem)/).optional(),
    overflow: z.enum(["visible", "hidden"]).optional(),
}).strict()

export function compileStyle(s?: Style): {viewStyle: ViewStyle; layers: BackgroundLayer[]}
```
- **Constraint:** this is a **closed vocabulary, not a CSS subset**. RN has no `background-image` property,
  no multiple backgrounds, no `background-size`/`repeat`, no CSS Grid, no `calc()`. Gradients and background
  images compile to absolutely-positioned **layers** rendered beneath children, never to style props.
- **Constraint:** `.strict()` is deliberate — a typo'd style key must surface as an issue, not be ignored.
- **Constraint:** `boxShadow` is the cross-platform shadow (RN ≥0.76). Do not emit iOS `shadow*` or Android
  `elevation`; 0.79 requires units in the string.
- **Validation:** `pnpm test`

### Step 4: grid geometry and collision
- **Test:** `client/layout/grid/grid.node-test.ts` —
  rect→percentage at 8×8, 12×12 and 4096×4096 (exact string output, no float drift beyond tolerance);
  `cols`/`rows` outside 8..4096 rejected; zero, negative and fractional `w`/`h` rejected; out-of-bounds rect
  clamped; the AABB truth table (disjoint, edge-touching = **no** collision, corner-touching, containment,
  identical, self-vs-self = no collision); absolute widgets absent from the sibling set entirely.
- **Implement:** `client/layout/grid/coords.ts`, `client/layout/grid/collision.ts`
- **Code:**
```ts
export const MIN_DIM = 8, MAX_DIM = 4096

export function toPercentBox(r: GridRect, g: GridSize) {
    return {left: `${(r.col / g.cols) * 100}%`,  top:    `${(r.row / g.rows) * 100}%`,
            width: `${(r.w   / g.cols) * 100}%`, height: `${(r.h   / g.rows) * 100}%`} as const
}

// identical to react-grid-layout's collides(); edge-touching is NOT a collision
export function collides(a: GridRect, b: GridRect): boolean {
    return a.col < b.col + b.w && b.col < a.col + a.w
        && a.row < b.row + b.h && b.row < a.row + a.h
}

// reject-and-snap-back (RGL's preventCollision), NOT push-cascade
export function canPlace(r: GridRect, id: string, siblings: PlacedWidget[], g: GridSize): boolean
```
- **Constraint:** never materialise cells. 4096×4096 is 16.7M cells; it is only ever a divisor.
- **Constraint:** absolute widgets are exempt as **both** subject and obstacle — they never enter `siblings`.
- **Constraint:** collision is scoped per grid level. A sub-grid child collides only with its own siblings
  inside its parent's coordinate space.
- **Validation:** `pnpm test`

### Step 5: layout schema and defensive parse
- **Test:** `client/layout/schema/layout.node-test.ts` — the worked example above parses clean; each of
  these yields an **issue and still renders**: overlapping non-absolute pair, out-of-bounds rect, zero-size
  widget, fractional coordinate, unknown widget `type`, sub-grid past depth 4, layout scene with no matching
  `stage.scenes` key. These are **rejected outright**: a `privateData` key anywhere, a pixel value in an
  absolute placement, `cols`/`rows` outside 8..4096.
- **Implement:** `client/layout/schema/{placement,widget,layout}.ts`
- **Code:**
```ts
const Percent = z.custom<`${number}%`>(v => typeof v === "string" && /^-?\d+(\.\d+)?%$/.test(v))

export const Placement = z.discriminatedUnion("kind", [
    z.object({kind: z.literal("grid"), col: z.int().nonnegative(), row: z.int().nonnegative(),
              w: z.int().positive(), h: z.int().positive()}),
    z.object({kind: z.literal("absolute"), left: Percent, top: Percent,
              width: Percent, height: Percent, z: z.int().optional()}),
])

export const GameLayout = z.object({
    grid: GridSize, style: StyleSchema.optional(), script: z.string().optional(),
    scenes: z.record(z.string(), z.object({
        style: StyleSchema.optional(), script: z.string().optional(),
        widgets: z.record(z.string(), WidgetSchema),      // MAP, never an array
    })),
})

// never throws — bad data degrades to issues, because a blank board is a worse
// failure than a wrong one, and this parses untrusted wire data on every keyframe
export function parseLayout(raw: unknown): {layout?: GameLayout; issues: LayoutIssue[]}
```
- **Constraint:** absolute placement uses `Percent`, so **pixel values are unrepresentable** rather than
  merely validated away — the spec's "no pixel positioning" becomes a type-level guarantee.
- **Constraint:** `privateData` is reserved anywhere in the layout. `SanitizeDelta` strips it in transit, so
  accepting it would cause silent, undebuggable data loss.
- **Constraint:** sub-grid depth cap is 4, threaded as a counter through the recursive schema via `z.lazy`.
  Past the cap, render an error placeholder — an unbounded recursive payload is a stack-overflow surface.
- **Constraint:** a layout scene entry with no matching `stage.scenes` key (or the reverse) is an issue, not
  an error; the two are edited independently.
- **Validation:** `pnpm test`

### Step 6: widget registry and field descriptors
- **Test:** `client/layout/registry/registry.node-test.ts` — **every key in each widget's config schema has
  a matching field descriptor and vice versa**; registering a duplicate `type` throws; `defaults` satisfy
  the widget's own schema. The bidirectional check is what keeps Plan B's auto-drawn panel honest.
- **Implement:** `client/layout/registry/{fields,registry}.ts`
- **Code:**
```ts
export type FieldDescriptor = FieldBase & (
    | {kind: "text"; multiline?: boolean}
    | {kind: "number"; min?: number; max?: number; step?: number}
    | {kind: "color"} | {kind: "boolean"} | {kind: "date"}
    | {kind: "select"; options: Option[]} | {kind: "multiselect"; options: Option[]}
    | {kind: "uri"; accept: "image" | "video"})

export interface WidgetDefinition<C> {
    type: string
    schema: z.ZodType<C>               // runtime validation of wire data
    fields: FieldDescriptor[]           // drives Plan B's config panel
    defaults: C
    Component: React.ComponentType<WidgetProps<C>>
    api(ctx: WidgetContext): Record<string, (...args: unknown[]) => unknown>   // callable from Lua
}
```
- **Constraint:** descriptors are hand-written. Do **not** derive them by introspecting zod — v4 moved
  `._def` → `._zod.def` and zod's own library-author guidance forbids reaching into internals. The
  bidirectional registry test is the mechanism that keeps schema and descriptors in sync instead.
- **Validation:** `pnpm test`

### Step 7: renderer — StyledBox, board, scene, widget host
- **Depends on:** Steps 3, 4, 5
- **Test:** no React renderer exists; the geometry, style and schema logic this consumes is fully covered by
  Steps 3-5. Do not add jest for this.
- **Implement:** `client/components/board/{styled-box,board-view,scene-view,widget-host}.tsx`;
  route `client/app/board/index.tsx`.
- **Code:**
```tsx
// RN has no background-image property, so layers are painted under children
export function StyledBox({style, children}: {style?: Style; children?: ReactNode}) {
    const {viewStyle, layers} = compileStyle(style)
    return (
        <View style={viewStyle}>
            {layers.map((l, i) => l.kind === "gradient"
                ? <LinearGradient key={i} colors={l.colors} locations={l.locations}
                                  start={l.start} end={l.end} style={StyleSheet.absoluteFill}/>
                : <Image key={i} source={{uri: l.uri}} contentFit={l.resizeMode}
                         style={StyleSheet.absoluteFill}/>)}
            {children}
        </View>
    )
}

export const WidgetHost = React.memo(function WidgetHost({id, widget, grid}: Props) {
    const box = widget.placement.kind === "grid"
        ? toPercentBox(widget.placement, grid) : absoluteBox(widget.placement)
    return <StyledBox style={widget.style} /* position:absolute + box */>{renderWidget(widget)}</StyledBox>
}, (a, b) => a.widget === b.widget)   // identity compare; deltas replace only changed subtrees
```
- **Constraint:** widgets are `React.memo` and keyed by widget id. Every delta produces fresh object
  identity for the whole game, so unkeyed children would re-reconcile the entire tree on every message.
- **Constraint:** the board container is `position: relative`; all widgets are absolutely positioned with
  `%` boxes. There is no flex layout inside the grid.
- **Constraint:** render `issues` from `parseLayout` into a dev-only overlay, not to the player.
- **Visual — requires human verification:** gradient rendering, border radii, layer ordering, `%` box
  alignment at both 8×8 and 4096×4096.
- **Validation:** `pnpm run typecheck && pnpm run lint`

### Step 8: widgets — text, image, sub-grid
- **Depends on:** Steps 6, 7
- **Test:** `client/layout/registry/widgets.node-test.ts` — each widget's `defaults` parse against its own
  schema; the sub-grid's recursive schema accepts nesting to depth 4 and issues at 5; text config rejects an
  unknown `align`.
- **Implement:** `client/components/board/widgets/{text,image,subgrid}/index.tsx` + their definitions.
- **Code:**
```tsx
export const TextWidget: WidgetDefinition<TextConfig> = {
    type: "text",
    schema: z.object({text: z.string(), fontSize: z.number().positive().optional(),
        color: z.string().optional(), align: z.enum(["left","center","right"]).optional()}).strict(),
    fields: [{key: "text", label: "Text", kind: "text", multiline: true},
             {key: "fontSize", label: "Size", kind: "number", min: 8, max: 512},
             {key: "color", label: "Colour", kind: "color"},
             {key: "align", label: "Align", kind: "select", options: ALIGN_OPTIONS}],
    defaults: {text: ""},
    Component: TextWidgetView,
    api: ctx => ({setText: (t: string) => ctx.override({text: String(t)})}),
}
```
- **Constraint:** sub-grid containers set `overflow: "hidden"` by default. RN's default is `visible`, so a
  `%`-positioned absolute child would otherwise silently escape its parent's bounds.
- **Constraint:** image widget uses `expo-image` (png/jpg/gif/webp/avif/svg on all three targets) with its
  built-in error placeholder. A broken or slow URI shows the placeholder — never a blank box, never a throw.
- **Constraint:** no video widget in this plan (see Non-goals). The `uri` field kind already accepts
  `"video"` so adding one later needs no schema change.
- **Visual — requires human verification:** text metrics across platforms, image `contentFit` modes,
  sub-grid clipping.
- **Validation:** `pnpm test && pnpm run typecheck`

### Step 9: Lua runtime — load, run, guard
- **Depends on:** Step 2
- **Test:** `client/layout/lua/runtime.node-test.ts` — a syntax error is returned as a value, not thrown; a
  runtime error surfaces the Lua message and line; `io`/`os`/`require`/`load` are `nil`; `while true do end`
  is aborted by the instruction budget within the budget's order of magnitude; a second script runs normally
  after a first one errored (no poisoned state).
- **Implement:** `client/layout/lua/runtime.ts`
- **Code:**
```ts
export function runChunk(L: lua_State, src: string, name: string): LuaResult {
    if (lauxlib.luaL_loadbuffer(L, to_luastring(src), null, to_luastring(name)) !== lua.LUA_OK)
        return {ok: false, error: to_jsstring(lua.lua_tostring(L, -1))}
    if (lua.lua_pcall(L, 0, 0, 0) !== lua.LUA_OK)
        return {ok: false, error: to_jsstring(lua.lua_tostring(L, -1))}
    return {ok: true}
}
```
- **Constraint:** a script error must never break rendering. Surface it in a dev console overlay and keep the
  last good presentation — mirroring the Go router's `recover()`-per-handler stance.
- **Constraint:** the instruction budget is **best-effort, not a guarantee**. Count hooks are per-coroutine
  and cannot interrupt inside a host function. Document that plainly; do not imply a hard limit.
- **Constraint:** always drain the Lua stack on both success and error paths. A leaked stack slot per event
  is an unbounded leak in a long session.
- **Validation:** `pnpm test`

### Step 10: host API, override layer, event bubbling
- **Depends on:** Step 9
- **Test:** `client/layout/lua/host-api.node-test.ts` — `widget("x").setText(...)` writes to the override map
  and **not** to game state; a keyframe clears all overrides; a delta does not; bubbling runs
  widget→scene→game and stops when a handler returns `false`; a handler registered for a removed widget is
  dropped; re-entrancy (a host callback invoked from inside a Lua handler) does not start a second dispatch.
- **Implement:** `client/layout/lua/{host-api,overrides,events}.ts`
- **Code:**
```ts
// Lua writes presentation only. Server state is reached solely through indri.send.
export interface HostApi {
    send(action: string, payload: Record<string, unknown>): void
    state(): Readonly<Game>                              // deep-frozen snapshot
    board:  {setStyle(s: Style): void}
    scene:  {setStyle(s: Style): void}
    widget(id: string): {setStyle(s: Style): void; setConfig(patch: object): void}
    on(event: LuaEvent, fn: LuaFunction): void
    log(...args: unknown[]): void
}
type LuaEvent = "stateChanged" | "sceneChanged" | "widgetPress" | "message"

const effective = mergeOverrides(serverLayout, overrides)   // server always wins for what it owns
```
- **Constraint:** overrides are cleared on **keyframe only**. A keyframe is a resync, so stale local
  presentation must not survive it; a delta is incremental and must not wipe Lua's work.
- **Constraint:** one long-lived Lua state per session with registered callbacks. Do **not** re-evaluate
  scripts on every delta — it would discard script-local state and burn the instruction budget on the
  priming path every tick, which is also what makes the budget meaningless.
- **Constraint:** a script changed by a delta re-instantiates that scope's chunk and discards its prior
  globals. Widget-local Lua state does not survive remove-then-re-add of the same id; document, don't fix.
- **Constraint:** `state()` returns a frozen snapshot. Handing Lua a live reference to the game object would
  let a script mutate authoritative state through the back door.
- **Validation:** `pnpm test`

### Step 11: Lua ↔ WebSocket bridge
- **Depends on:** Step 10
- **Test:** `client/layout/lua/bridge.node-test.ts` — with a fake socket, `indri.send("move", {...})` emits
  `{action: "move", ...}`; an inbound message dispatches to registered handlers; an unknown action is
  ignored rather than thrown; a send before the socket opens is dropped with a warning, matching
  `MessageHandler.send`'s existing behaviour.
- **Implement:** `client/layout/lua/bridge.ts`; register one parser in the existing `parsers` array.
- **Code:**
```ts
// reuses MessageHandler's existing extension point — no change to its internals
new MessageHandler(url, userDispatch, gameDispatch, listDispatch, [
    {name: "lua_event", action: "luaEvent", parser: d => bridge.dispatchInbound(d)},
])
```
- **Constraint:** the framework reserves no action names. Games name their own actions, exactly as
  tic-tac-toe's `move` does. Do not add a Lua-specific namespace to the protocol.
- **Constraint:** `indri.send` payloads are JSON-serialised at the boundary; reject cyclic or
  deeper-than-8-level tables rather than letting `JSON.stringify` throw inside a Lua callback.
- **Validation:** `pnpm test`

### Step 12: migrate tic-tac-toe to a layout
- **Depends on:** Steps 8, 11
- **Test:** `client/layout/fixtures/tictactoe.node-test.ts` — the shipped layout parses with **zero** issues;
  its 9 sub-grid rects tile 3×3 without overlap; every widget id referenced by the scene script exists.
- **Implement:** add `data.layout` to `config.json` and `example/tictactoe/config.json`; strip the hardcoded
  grid from `client/app/index.tsx` and render `<BoardView/>`.
- **Code:**
```lua
-- scene script: Lua IS the binding layer between game state and widget presentation
indri.on("stateChanged", function(g)
  local b = g.stage.scenes.board.data.board
  for r = 0, 2 do for c = 0, 2 do
    widget(string.format("c%d%d", r, c)).setConfig({ text = b[r+1][c+1] })
  end end
end)

indri.on("widgetPress", function(id)
  indri.send("move", { move = string.sub(id,2,2) .. "," .. string.sub(id,3,3) })
end)
```
- **Constraint:** the Go `move` handler is **unchanged** — it still writes `data.board`. The layout reads
  that state; it does not replace it. Game logic stays server-side.
- **Constraint:** Lua is 1-indexed and the board paths are 0-indexed. The `r+1`/`c+1` above is deliberate;
  get it wrong and the board silently transposes.
- **Constraint:** if Step 2 failed on native, fall back to a declarative `$bind` on the `text` config key and
  mark Lua web-only. State explicitly which path was taken.
- **Validation:** `pnpm test`; then play a full game across two browser windows and one device, confirming
  both players see each move.

### Step 13: measure delta replay cost
- **Test:** `client/services/game-state-parser.bench.node-test.ts` — replay time for a realistic layout
  (~200 widgets) at 1/10/100/500 accumulated deltas. Asserts only that it completes; the numbers are the
  deliverable.
- **Implement:** benchmark only. **Do not optimise in this step.**
- **Code:**
```ts
const p = new GameStateParser<Game>()
p.set(gameWithLayout(200), new Date(0))
for (const n of [1, 10, 100, 500]) { /* push n deltas, time the final reapply, log ms */ }
```
- **Constraint:** `reapply()` is O(state × retained deltas), and deltas are pruned only on keyframe, which
  arrives only on create/join/refresh/reconnect. Report measured numbers and decide afterwards — folding
  settled deltas into the base is a follow-up, deliberately out of scope here.
- **Validation:** `pnpm test`, then record the figures in this file.

## Acceptance Criteria

- [ ] A layout at `game.data.layout` renders on web **and** a native device, with the widget set selected by
      `stage.currentScene`.
- [ ] Grid accepts 8×8 through 4096×4096 with no cell materialisation at any size.
- [ ] Non-absolute widgets are reported as an issue when they overlap; absolute widgets position in `%` only
      and may overlap freely.
- [ ] Text, image and sub-grid widgets render, each with background colour, background image, linear
      gradient, and borders.
- [ ] Malformed layout data (overlap, out-of-bounds, zero-size, unknown type, excess depth) degrades to a
      dev-visible issue and still renders — never a crash, never a blank board.
- [ ] Lua at game/scene/widget level runs sandboxed (`io`/`os`/`require`/`load` all `nil`), survives errors
      without breaking render, and reaches the server only through `indri.send`.
- [ ] A Lua-driven interaction completes the loop: press → `indri.send` → server → delta → every client.
- [ ] Tic-tac-toe is playable end-to-end with zero hardcoded board markup in `app/index.tsx`.
- [ ] `pnpm test` runs every `*.node-test.ts` and passes.

## Checklist (non-TDD cleanup)

- [ ] `pnpm run lint` and `pnpm run typecheck` clean
- [ ] Step 2's fengari/Hermes result written down — it is the one finding nobody can cheaply rederive
- [ ] Step 13's benchmark numbers recorded in this file
- [ ] `docs/ARCHITECTURE.md` gains a layout-engine section; `CLAUDE.md` notes `game.data.layout` as the
      canonical location and `privateData` as reserved
- [ ] `client/app/board/spike.tsx` deleted
- [ ] `client/services/players.ts` is a 0-byte placeholder — delete it or fill it, don't leave it

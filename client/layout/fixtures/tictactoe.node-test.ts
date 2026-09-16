/**
 * TDD tests for the tic-tac-toe data-driven layout fixture.
 *
 * Covered:
 *   1. parseLayout(fixture) returns zero issues
 *   2. The 9 cell rects tile 3x3 without overlap
 *   3. Every widget id referenced by the Lua script exists in the layout
 *   4. The Lua 0-indexed access pattern b[r][c] is correct for fengari-interop
 *      JS proxy arrays (fengari-interop __index does u[k] with the raw Lua
 *      number, so b[0] = array[0], b[1] = array[1] — 0-based, NOT 1-based)
 *   5. stateChanged: widget text is set from board state via Lua
 *   6. widgetPress: sends the correct move action
 */
import {describe, it} from "node:test"
import assert from "node:assert/strict"
import {createRequire} from "node:module"

import {parseLayout} from "../schema/layout.ts"
import {collides} from "../grid/collision.ts"
import type {GridRect} from "../grid/coords.ts"
import {LuaSession} from "../lua/host-api.ts"

import fixture from "./tictactoe.json" with {type: "json"}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _require = createRequire(import.meta.url)
// widget() is a sandbox global that the host-api exposes — we wire it
// manually in tests by installing a custom host api on the LuaSession.

// All 9 expected widget IDs (cRC where R=row 0-2, C=col 0-2)
const EXPECTED_WIDGET_IDS = [
    "c00", "c01", "c02",
    "c10", "c11", "c12",
    "c20", "c21", "c22",
]

// ---------------------------------------------------------------------------
// 1. parseLayout
// ---------------------------------------------------------------------------

describe("tictactoe fixture: parseLayout", () => {
    it("parses with zero issues", () => {
        const {layout, issues} = parseLayout(fixture)
        assert.ok(layout !== undefined, "layout must be returned")
        assert.deepEqual(issues, [], `expected zero issues, got: ${JSON.stringify(issues)}`)
    })

    it("has a 12x12 grid", () => {
        const {layout} = parseLayout(fixture)
        assert.equal(layout!.grid.cols, 12)
        assert.equal(layout!.grid.rows, 12)
    })

    it("has a 'board' scene", () => {
        const {layout} = parseLayout(fixture)
        assert.ok("board" in layout!.scenes, "should have a 'board' scene")
    })

    it("has exactly 9 widgets", () => {
        const {layout} = parseLayout(fixture)
        const widgetIds = Object.keys(layout!.scenes.board.widgets)
        assert.equal(widgetIds.length, 9, `expected 9 widgets, got ${widgetIds.length}: ${widgetIds.join(", ")}`)
    })
})

// ---------------------------------------------------------------------------
// 2. Non-overlapping tile layout
// ---------------------------------------------------------------------------

describe("tictactoe fixture: grid layout", () => {
    it("all 9 cell rects are 4x4 each", () => {
        const {layout} = parseLayout(fixture)
        const widgets = layout!.scenes.board.widgets
        for (const id of EXPECTED_WIDGET_IDS) {
            const w = widgets[id] as {placement: {kind: string; col: number; row: number; w: number; h: number}}
            assert.equal(w.placement.kind, "grid", `${id} must be a grid widget`)
            assert.equal(w.placement.w, 4, `${id}.w must be 4`)
            assert.equal(w.placement.h, 4, `${id}.h must be 4`)
        }
    })

    it("no two cell rects overlap (tile without gaps)", () => {
        const {layout} = parseLayout(fixture)
        const widgets = layout!.scenes.board.widgets
        const rects: Array<{id: string; rect: GridRect}> = EXPECTED_WIDGET_IDS.map(id => {
            const w = widgets[id] as {placement: {kind: string; col: number; row: number; w: number; h: number}}
            return {id, rect: {col: w.placement.col, row: w.placement.row, w: w.placement.w, h: w.placement.h}}
        })

        for (let i = 0; i < rects.length; i++) {
            for (let j = i + 1; j < rects.length; j++) {
                const a = rects[i]
                const b = rects[j]
                assert.equal(
                    collides(a.rect, b.rect),
                    false,
                    `widgets "${a.id}" and "${b.id}" must not overlap`,
                )
            }
        }
    })

    it("cells cover the full 12x12 grid with no gaps (union area = 144)", () => {
        const {layout} = parseLayout(fixture)
        const widgets = layout!.scenes.board.widgets
        let totalArea = 0
        for (const id of EXPECTED_WIDGET_IDS) {
            const w = widgets[id] as {placement: {kind: string; col: number; row: number; w: number; h: number}}
            totalArea += w.placement.w * w.placement.h
        }
        assert.equal(totalArea, 144, `9 cells of 4x4 should cover the full 12x12 grid (area=144), got ${totalArea}`)
    })
})

// ---------------------------------------------------------------------------
// 3. Widget ID completeness (all IDs referenced by script exist in layout)
// ---------------------------------------------------------------------------

describe("tictactoe fixture: widget IDs", () => {
    it("all 9 expected widget IDs exist in the layout", () => {
        const {layout} = parseLayout(fixture)
        const widgetIds = new Set(Object.keys(layout!.scenes.board.widgets))
        for (const id of EXPECTED_WIDGET_IDS) {
            assert.ok(widgetIds.has(id), `widget "${id}" (referenced by Lua script) must exist in layout`)
        }
    })

    it("widget IDs follow cRC pattern where R and C are 0-2", () => {
        const {layout} = parseLayout(fixture)
        const widgetIds = Object.keys(layout!.scenes.board.widgets)
        for (const id of widgetIds) {
            assert.match(id, /^c[0-2][0-2]$/, `widget id "${id}" must match cRC pattern with R,C in 0-2`)
        }
    })
})

// ---------------------------------------------------------------------------
// 4. Lua 0-indexed access via fengari-interop JS proxy
//
// fengari-interop __index does: u[k] where k = lua_tonumber(L, idx)
// This means Lua b[0] → JS array[0] (first element), b[1] → JS array[1], etc.
// The correct Lua loop is: for r=0,2 do for c=0,2 do b[r][c] end end
// NOT: b[r+1][c+1] (that would produce board[1][1] for r=0,c=0 — wrong)
// ---------------------------------------------------------------------------

describe("tictactoe fixture: Lua script board access pattern", () => {
    it("Lua script uses 0-based array access (b[r][c]) matching JS array layout", () => {
        // Verify by inspecting the script text in the fixture
        const scene = fixture.scenes.board
        assert.ok(scene.script, "scene must have a script")
        // Must use b[r][c], not b[r+1][c+1]
        assert.ok(
            scene.script.includes("b[r][c]"),
            `Lua script must use b[r][c] (0-based JS proxy access), got script: ${scene.script}`,
        )
        assert.ok(
            !scene.script.includes("b[r+1][c+1]"),
            "Lua script must NOT use b[r+1][c+1] — fengari-interop JS proxy arrays are 0-indexed",
        )
    })

    it("widgetPress sends 'move' with cRC id digits as r,c string", () => {
        // The Lua script for widgetPress uses string.sub(id,2,2) and string.sub(id,3,3)
        // For id="c01": string.sub("c01",2,2) = "0", string.sub("c01",3,3) = "1"
        // So the move message is "0,1" — matching the server's expected r,c format.
        const scene = fixture.scenes.board
        assert.ok(scene.script.includes('string.sub(id,2,2)'), "script must extract row digit from widget id")
        assert.ok(scene.script.includes('string.sub(id,3,3)'), "script must extract col digit from widget id")
        assert.ok(scene.script.includes('"move"'), 'script must call indri.send with action "move"')
    })
})

// ---------------------------------------------------------------------------
// 5. LuaSession integration: stateChanged sets widget text from board state
// ---------------------------------------------------------------------------

describe("tictactoe Lua script: stateChanged updates widget text", () => {
    it("sets all 9 widget texts from board state on stateChanged", () => {
        const session = new LuaSession()
        session.installHostApi(() => {})

        // Install the widget() global — it's a HostApi method not exposed in Lua
        // by default. We patch it in by registering a global "widget" function via
        // the session's Lua state using the session's own widget() API.
        // The host-api registers widget() under the "indri" namespace; the scene
        // script expects a bare global "widget(id)". Install it:
        const {lua: luaMod, to_luastring} = _require("fengari") as typeof import("fengari")
        const {lauxlib} = _require("fengari") as typeof import("fengari")

        // Expose widget() as a global in Lua: widget(id) calls session.widget(id)
        // We do this via a thin Lua shim that delegates to indri.*
        // Actually, the scene script calls bare widget(id), so we need to install
        // it as a Lua global. Use runScript to define it in terms of indri internals.
        // The simplest approach: run a Lua setup chunk that aliases widget = indri.widget
        // — but indri.widget is NOT exposed. Instead we set up a JS-backed global.
        // We install widget via the standard Lua C API:
        const L = session.L

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const luaExt = luaMod as any
        luaExt.lua_pushcfunction(L, (innerL: import("fengari").lua_State) => {
            const raw = luaMod.lua_tostring(innerL, 1)
            const {to_jsstring} = _require("fengari") as typeof import("fengari")
            const id = raw ? to_jsstring(raw) : ""
            // Return a table with setConfig and setStyle methods
            // We need to push a Lua table with methods that forward to session.widget(id)
            luaExt.lua_newtable(innerL)
            const tblIdx = luaMod.lua_gettop(innerL)

            luaExt.lua_pushcfunction(innerL, (L2: import("fengari").lua_State) => {
                // setConfig(patch) — patch is a Lua table passed as arg 1.
                // Call is: widget("c00").setConfig({ text = "X" }) — dot-syntax, no implicit self.
                if (luaMod.lua_type(L2, 1) === luaExt.LUA_TTABLE) {
                    // Extract text key from the Lua table
                    luaExt.lua_pushstring(L2, to_luastring("text"))
                    luaExt.lua_gettable(L2, 1)
                    const rawText = luaMod.lua_tostring(L2, -1)
                    const text = rawText ? to_jsstring(rawText) : ""
                    luaMod.lua_pop(L2, 1)
                    session.widget(id).setConfig({text})
                }
                return 0
            })
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setConfig"))

            luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => {
                // setStyle — no-op for this test
                return 0
            })
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setStyle"))

            return 1
        })
        luaMod.lua_setglobal(L, to_luastring("widget"))

        // Run the scene script
        const r = session.runScript(fixture.scenes.board.script, "=tictactoe")
        assert.ok(r.ok, `scene script failed: ${r.ok ? "" : r.error}`)

        // Build a game state with a known board: X at (0,0), O at (1,1)
        const game = {
            stage: {
                scenes: {
                    board: {
                        data: {
                            board: [
                                ["X", "", ""],
                                ["", "O", ""],
                                ["", "", ""],
                            ],
                        },
                    },
                },
            },
        }

        session.onStateChange(game, "board")

        // Check widget overrides: c00 = "X", c11 = "O", others = ""
        assert.equal(session.overrides.widgets["c00"]?.config?.["text"], "X", "c00 must show X")
        assert.equal(session.overrides.widgets["c11"]?.config?.["text"], "O", "c11 must show O")
        assert.equal(session.overrides.widgets["c01"]?.config?.["text"], "", "c01 must be empty")
    })
})

// ---------------------------------------------------------------------------
// 6. LuaSession integration: widgetPress sends the correct move action
// ---------------------------------------------------------------------------

describe("tictactoe Lua script: widgetPress sends move action", () => {
    it("pressing c12 sends move action with '1,2'", () => {
        const session = new LuaSession()

        const sent: Array<{action: string; payload: Record<string, unknown>}> = []
        session.installHostApi((action, payload) => {
            sent.push({action, payload})
        })

        // Install bare widget() global (no-op setConfig/setStyle — not needed here)
        const {lua: luaMod, to_luastring, to_jsstring} = _require("fengari") as typeof import("fengari")
        const L = session.L
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const luaExt = luaMod as any

        luaExt.lua_pushcfunction(L, (innerL: import("fengari").lua_State) => {
            luaExt.lua_newtable(innerL)
            const tblIdx = luaMod.lua_gettop(innerL)
            luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => 0)
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setConfig"))
            luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => 0)
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setStyle"))
            return 1
        })
        luaMod.lua_setglobal(L, to_luastring("widget"))

        const r = session.runScript(fixture.scenes.board.script, "=tictactoe")
        assert.ok(r.ok, `scene script failed: ${r.ok ? "" : r.error}`)

        session.onKeyframe({}, "board")
        session.emit("widgetPress", "c12")

        assert.equal(sent.length, 1, `expected 1 send, got ${sent.length}: ${JSON.stringify(sent)}`)
        assert.equal(sent[0].action, "move")
        assert.equal(sent[0].payload["move"], "1,2", `expected move='1,2', got: ${JSON.stringify(sent[0].payload)}`)
    })

    it("pressing c00 sends move action with '0,0'", () => {
        const session = new LuaSession()

        const sent: Array<{action: string; payload: Record<string, unknown>}> = []
        session.installHostApi((action, payload) => {
            sent.push({action, payload})
        })

        const {lua: luaMod, to_luastring} = _require("fengari") as typeof import("fengari")
        const L = session.L
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const luaExt = luaMod as any

        luaExt.lua_pushcfunction(L, (innerL: import("fengari").lua_State) => {
            luaExt.lua_newtable(innerL)
            const tblIdx = luaMod.lua_gettop(innerL)
            luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => 0)
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setConfig"))
            luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => 0)
            luaExt.lua_setfield(innerL, tblIdx, to_luastring("setStyle"))
            return 1
        })
        luaMod.lua_setglobal(L, to_luastring("widget"))

        const r = session.runScript(fixture.scenes.board.script, "=tictactoe")
        assert.ok(r.ok, `scene script failed: ${r.ok ? "" : r.error}`)

        session.onKeyframe({}, "board")
        session.emit("widgetPress", "c00")

        assert.equal(sent.length, 1, `expected 1 send, got ${sent.length}`)
        assert.equal(sent[0].action, "move")
        assert.equal(sent[0].payload["move"], "0,0")
    })
})

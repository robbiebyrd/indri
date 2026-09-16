import {createRequire} from "node:module"
import {lauxlib, lua, to_jsstring, to_luastring} from "fengari"

import type {lua_State} from "fengari"

const _require = createRequire(import.meta.url)
const fengariInterop = _require("fengari-interop") as {
    push(L: lua_State, v: unknown): void
    tojs(L: lua_State, idx: number): unknown
    luaopen_js(L: lua_State): number
}

import {createRuntimeState, runChunk} from "./runtime.ts"
import type {LuaResult} from "./runtime.ts"
import {emptyOverrides} from "./overrides.ts"
import type {OverrideMap} from "./overrides.ts"
import {EventRegistry} from "./events.ts"
import type {LuaEvent, LuaHandler} from "./events.ts"
import type {Style} from "../schema/style.ts"

export type {LuaResult, LuaEvent, LuaHandler}

// ---------------------------------------------------------------------------
// Deep freeze
// ---------------------------------------------------------------------------

function deepFreeze(obj: unknown): Readonly<unknown> {
    if (obj === null || typeof obj !== "object") return obj as Readonly<unknown>
    if (Object.isFrozen(obj)) return obj as Readonly<unknown>
    Object.freeze(obj)
    for (const key of Object.keys(obj)) {
        deepFreeze((obj as Record<string, unknown>)[key])
    }
    return obj as Readonly<unknown>
}

// ---------------------------------------------------------------------------
// Fengari extended API
//
// fengari.d.ts declares only the surface used by state.ts / state.node-test.ts.
// We need a few more functions here. We access them through a typed cast so
// TypeScript doesn't complain about undeclared properties.
// ---------------------------------------------------------------------------

type FengariLuaExt = {
    lua_newtable(L: lua_State): void
    lua_setfield(L: lua_State, idx: number, k: Uint8Array): void
    lua_pushcfunction(L: lua_State, fn: (L: lua_State) => number): void
    lua_pushstring(L: lua_State, s: Uint8Array): void
    lua_pushnumber(L: lua_State, n: number): void
    lua_tojsstring(L: lua_State, idx: number): string
    lua_tonumber(L: lua_State, idx: number): number
    lua_toboolean(L: lua_State, idx: number): boolean
    lua_isnil(L: lua_State, idx: number): boolean
    lua_next(L: lua_State, idx: number): number
    LUA_TNIL: number
    LUA_TBOOLEAN: number
    LUA_TNUMBER: number
    LUA_TSTRING: number
    LUA_TTABLE: number
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const luaExt = lua as unknown as FengariLuaExt

// ---------------------------------------------------------------------------
// Lua table → plain JS object (avoids fengari-interop proxy's ownKeys issue)
// ---------------------------------------------------------------------------

function luaValueToJs(L: lua_State, idx: number): unknown {
    const t = lua.lua_type(L, idx)
    if (t === luaExt.LUA_TBOOLEAN) return luaExt.lua_toboolean(L, idx)
    if (t === luaExt.LUA_TNUMBER)  return luaExt.lua_tonumber(L, idx)
    if (t === luaExt.LUA_TSTRING)  { const r = lua.lua_tostring(L, idx); return r ? to_jsstring(r) : "" }
    if (t === luaExt.LUA_TTABLE)   return luaTableToJs(L, idx)
    return null
}

function luaTableToJs(L: lua_State, tableIdx: number): Record<string, unknown> {
    const top = lua.lua_gettop(L)
    const absIdx = tableIdx > 0 ? tableIdx : top + tableIdx + 1
    const result: Record<string, unknown> = {}
    lua.lua_pushnil(L)
    while (luaExt.lua_next(L, absIdx) !== 0) {
        const keyType = lua.lua_type(L, -2)
        let k: string | undefined
        if (keyType === luaExt.LUA_TSTRING) { const r = lua.lua_tostring(L, -2); k = r ? to_jsstring(r) : undefined }
        else if (keyType === luaExt.LUA_TNUMBER) k = String(luaExt.lua_tonumber(L, -2))
        if (k !== undefined) result[k] = luaValueToJs(L, -1)
        lua.lua_pop(L, 1)
    }
    return result
}

// ---------------------------------------------------------------------------
// HostApi interface
// ---------------------------------------------------------------------------

export interface HostApi {
    send(action: string, payload: Record<string, unknown>): void
    state(): Readonly<unknown>
    board: {setStyle(s: Style): void}
    scene: {setStyle(s: Style): void}
    widget(id: string): {setStyle(s: Style): void; setConfig(patch: Record<string, unknown>): void}
    on(event: LuaEvent, fn: LuaHandler): void
    log(...args: unknown[]): void
}

// ---------------------------------------------------------------------------
// LuaSession
// ---------------------------------------------------------------------------

export class LuaSession implements HostApi {
    readonly L: lua_State
    overrides: OverrideMap
    currentGame: unknown
    private currentSceneId: string
    private dispatch_in_progress: boolean
    readonly events: EventRegistry

    constructor() {
        this.L = createRuntimeState()
        this.overrides = emptyOverrides()
        this.currentGame = undefined
        this.currentSceneId = ""
        this.dispatch_in_progress = false
        this.events = new EventRegistry()
    }

    // HostApi: send — wired by installHostApi
    send(_action: string, _payload: Record<string, unknown>): void {
        // no-op until installHostApi is called
    }

    // HostApi: state
    state(): Readonly<unknown> {
        return deepFreeze(
            // Return a plain JSON clone so mutations to the frozen object cannot
            // reach currentGame through shared references.
            JSON.parse(JSON.stringify(this.currentGame ?? null)) as unknown,
        )
    }

    // HostApi: board
    readonly board = {
        setStyle: (s: Style) => {
            this.overrides.board = {style: {...this.overrides.board?.style, ...s}}
        },
    }

    // HostApi: scene
    readonly scene = {
        setStyle: (s: Style) => {
            this.overrides.scene = {style: {...this.overrides.scene?.style, ...s}}
        },
    }

    // HostApi: widget
    widget(id: string): {setStyle(s: Style): void; setConfig(patch: Record<string, unknown>): void} {
        return {
            setStyle: (s: Style) => {
                const existing = this.overrides.widgets[id] ?? {}
                this.overrides.widgets[id] = {
                    ...existing,
                    style: {...existing.style, ...s},
                }
            },
            setConfig: (patch: Record<string, unknown>) => {
                const existing = this.overrides.widgets[id] ?? {}
                this.overrides.widgets[id] = {
                    ...existing,
                    config: {...existing.config, ...patch},
                }
            },
        }
    }

    // HostApi: on — registers at the "game" scope
    on(event: LuaEvent, fn: LuaHandler): void {
        this.events.on("game", event, fn)
    }

    // HostApi: log
    log(...args: unknown[]): void {
        console.log(...args)
    }

    // -----------------------------------------------------------------------
    // Lifecycle
    // -----------------------------------------------------------------------

    /**
     * Called by the provider when a keyframe arrives.
     * Clears all overrides (full resync).
     */
    onKeyframe(game: unknown, sceneId: string): void {
        this.overrides = emptyOverrides()
        this.currentGame = game
        this.currentSceneId = sceneId
    }

    /**
     * Called by the provider when a delta is applied.
     * Does NOT clear overrides — incremental update.
     */
    onStateChange(game: unknown, sceneId: string): void {
        this.currentGame = game
        this.currentSceneId = sceneId
        this.emit("stateChanged", null)
    }

    /**
     * Load and run a Lua source chunk in the context of this session.
     * Never throws; errors surface as {ok:false}.
     * Captures and returns the first Lua return value as a string if present.
     */
    runScript(src: string, name: string): LuaResult {
        const L = this.L
        const loadStatus = lauxlib.luaL_loadbuffer(L, to_luastring(src), null, to_luastring(name))
        if (loadStatus !== lua.LUA_OK) {
            const error = to_jsstring(lua.lua_tostring(L, -1))
            lua.lua_pop(L, 1)
            return {ok: false, error}
        }
        // Request 1 return value so callers can inspect results (e.g. tests reading Lua globals).
        const callStatus = lua.lua_pcall(L, 0, 1, 0)
        if (callStatus !== lua.LUA_OK) {
            const error = to_jsstring(lua.lua_tostring(L, -1))
            lua.lua_pop(L, 1)
            return {ok: false, error}
        }
        const raw = lua.lua_tostring(L, -1)
        const value = raw !== null ? to_jsstring(raw) : undefined
        lua.lua_pop(L, 1)
        return {ok: true, value}
    }

    /**
     * Emit an event with bubbling.
     *
     * Re-entrancy guard: if called while already dispatching, the call is
     * silently dropped to prevent infinite recursion.
     */
    emit(event: LuaEvent, widgetId?: string | null, ...args: unknown[]): void {
        if (this.dispatch_in_progress) return
        this.dispatch_in_progress = true
        try {
            this.events.emit(event, this.currentSceneId, widgetId ?? null, ...args)
        } finally {
            this.dispatch_in_progress = false
        }
    }

    /**
     * Register the `indri` global table in the Lua state.
     * Must be called once after construction and before running scripts.
     *
     * @param sendFn  Called when Lua invokes `indri.send(action, payload)`.
     */
    installHostApi(sendFn: (action: string, payload: Record<string, unknown>) => void): void {
        const L = this.L

        // Initialise JS interop so fengariInterop.push works in this state.
        lauxlib.luaL_requiref(L, to_luastring("js"), fengariInterop.luaopen_js, 0)
        lua.lua_pop(L, 1)

        // Build the `indri` table
        luaExt.lua_newtable(L) // stack: [tbl]
        const tblIdx = lua.lua_gettop(L)

        // indri.send(action, payload)
        const self = this
        luaExt.lua_pushcfunction(L, (innerL) => {
            try {
                const actionRaw = lua.lua_tostring(innerL, 1)
                const action = actionRaw ? to_jsstring(actionRaw) : ""
                const payload = lua.lua_type(innerL, 2) === luaExt.LUA_TTABLE
                    ? luaTableToJs(innerL, 2)
                    : {}
                sendFn(action, payload)
            } catch (e) {
                console.warn("[LuaSession] indri.send dropped:", e)
            }
            return 0
        })
        luaExt.lua_setfield(L, tblIdx, to_luastring("send"))

        // indri.state()
        luaExt.lua_pushcfunction(L, (innerL) => {
            const snap = self.state()
            fengariInterop.push(innerL, snap)
            return 1
        })
        luaExt.lua_setfield(L, tblIdx, to_luastring("state"))

        // indri.on(event, fn)
        luaExt.lua_pushcfunction(L, (innerL) => {
            try {
                const event = luaExt.lua_tojsstring(innerL, 1) as LuaEvent
                const fn = fengariInterop.tojs(innerL, 2) as LuaHandler
                self.events.on("game", event, fn)
            } catch (_) {
                // ignore
            }
            return 0
        })
        luaExt.lua_setfield(L, tblIdx, to_luastring("on"))

        // indri.log(...)
        luaExt.lua_pushcfunction(L, (innerL) => {
            const n = lua.lua_gettop(innerL)
            const parts: unknown[] = []
            for (let i = 1; i <= n; i++) {
                parts.push(tojs(innerL, i))
            }
            console.log(...parts)
            return 0
        })
        luaExt.lua_setfield(L, tblIdx, to_luastring("log"))

        // Set _G.indri = tbl
        lua.lua_setglobal(L, to_luastring("indri"))
        // stack is now empty again
    }
}

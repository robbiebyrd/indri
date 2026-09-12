/**
 * The `indri` global: everything a script is allowed to touch.
 *
 * LUA DRIVES BEHAVIOUR; THE SERVER OWNS STATE. That one sentence explains every
 * decision in this file:
 *
 * - There is no way to write game state. Presentation writes land in the local
 *   override layer (`./overrides.ts`); the only thing that reaches the server
 *   is an action posted through `send`, which the server may accept or refuse
 *   like any other client message.
 * - `state()` hands out a DEEP-FROZEN CLONE on the JS side and a read-only
 *   proxy on the Lua side. A live reference would let a script mutate the
 *   authoritative object through the back door, and the reducer would then
 *   disagree with the server with nothing to show why.
 * - LUA IS ASYMMETRIC. Scripts send actions outbound; nothing inbound is
 *   delivered to them. A script learns about the world by observing the
 *   reduced state through `stateChanged` — see `./events.ts` for why.
 *
 * IMPLEMENTED WITH PLAIN `lua_pushcfunction`, NOT `fengari-interop`. Interop's
 * value is handing JS objects to Lua as proxies, and that is exactly what this
 * module must not do: an interop-wrapped object exposes its JS prototype chain,
 * so `obj.constructor.constructor` reaches the JS `Function` constructor and
 * walks straight out of the sandbox `./state.ts` spends fifty lines building.
 * Interop also ships no type declarations, so it would not have saved any. Host
 * functions here take and return marshalled values only; no JS object is ever
 * visible to a script.
 *
 * One long-lived Lua state holds every registered callback. Scripts are NOT
 * re-evaluated per delta: that would discard script-local state and spend the
 * whole instruction budget re-running priming code every tick, which is also
 * what would make the budget meaningless. `instantiate` is the deliberate
 * re-run, for when a delta changed the script text itself.
 */

import {lauxlib, lua, to_jsstring, to_luastring} from "fengari"

import {StyleSchema} from "../schema/style.ts"
import {BOARD_SCOPE, EventBus, LUA_EVENTS, isLuaEvent, scopeKey} from "./events.ts"
import {OverrideLayer} from "./overrides.ts"
import {LuaScriptError} from "./runtime.ts"

import type {LuaCFunction, lua_State} from "fengari"
import type {Game} from "../../models/models.ts"
import type {Style} from "../schema/style.ts"
import type {DispatchResult, Invoke, LuaEvent, Scope} from "./events.ts"
import type {LuaResult, LuaRuntime} from "./runtime.ts"

export type {LuaEvent, Scope} from "./events.ts"

/**
 * A Lua function pinned in the registry. Opaque by design — `ref` is a
 * `luaL_ref` key, and holding it is what stops the GC collecting a callback
 * whose only other reference was the script's local variable.
 */
export interface LuaFunction {
    readonly ref: number
}

export interface StyleTarget {
    setStyle(style: Style): void
}

export interface WidgetTarget extends StyleTarget {
    setConfig(patch: Readonly<Record<string, unknown>>): void
}

/**
 * The surface a script sees. Mirrored one-for-one into the Lua `indri` table,
 * so this interface is the contract in both languages.
 */
export interface HostApi {
    send(action: string, payload: Record<string, unknown>): void
    /** A deep-frozen snapshot. Never the live game object. */
    state(): Readonly<Game>
    readonly board: StyleTarget
    readonly scene: StyleTarget
    widget(id: string): WidgetTarget
    on(event: LuaEvent, fn: LuaFunction): void
    log(...args: unknown[]): void
}

export interface HostDeps {
    /**
     * The only outbound path. Injected rather than imported so this module
     * never depends on the websocket; story 024 wires it to `MessageHandler`.
     */
    send: (action: string, payload: Record<string, unknown>) => void

    /**
     * The scene a call resolves against when the caller is not itself inside a
     * scene or widget chunk — normally `stage.currentScene`. Required, because
     * silently defaulting to "the first scene" would let a board script paint
     * the wrong board.
     */
    activeSceneId: () => string | undefined

    /** Where `indri.log` goes. Defaults to `console.log`. */
    log?: (...args: unknown[]) => void
}

/** What one `emit`/`broadcast` did, plus any script errors it contained. */
export interface EmitOutcome extends DispatchResult {
    /** One entry per handler that failed. The chain continued regardless. */
    readonly errors: readonly string[]
}

/**
 * Depth cap when marshalling Lua -> JS.
 *
 * This also handles cycles: a table that refers to itself hits the cap instead
 * of hanging, so nothing has to track table identity, and `JSON.stringify`
 * never gets the chance to throw from inside a Lua callback.
 */
export const MAX_PAYLOAD_DEPTH = 8

/**
 * Depth cap when marshalling JS -> Lua. Much looser than the payload cap: this
 * side carries authoritative game state, and truncating it would silently lie
 * to a script rather than reject a bad input.
 */
const MAX_STATE_DEPTH = 32

/**
 * Stack slots one level of marshalling reserves before it pushes anything.
 *
 * LUA DOES NOT GROW THE STACK FOR YOU. A host function is entered with
 * `LUA_MINSTACK` (20) slots reserved, and fengari's `api_check` throws a bare
 * `Error("stack overflow")` the moment a push goes past them — no Lua traceback,
 * no chunk name, nothing pointing at the marshaller. Marshalling recurses, so
 * every level has to ask for its own room; a real game state is several levels
 * deep and exceeds 20 without this.
 *
 * Eight is the deepest any single level goes: a table, a key, and the four slots
 * `wrapReadOnly` needs to build a proxy over it, with margin.
 */
const MARSHAL_SLOTS = 8

const INDEX = to_luastring("__index")
const NEWINDEX = to_luastring("__newindex")
const LEN = to_luastring("__len")
const PAIRS = to_luastring("__pairs")
const METATABLE = to_luastring("__metatable")
/** Marks a read-only proxy so it can be unwrapped when handed back to the host. */
const READ_ONLY = to_luastring("__indriReadOnly")

export class LuaHost {
    readonly overrides = new OverrideLayer()
    readonly api: HostApi

    private readonly rt: LuaRuntime
    private readonly deps: HostDeps
    private readonly bus = new EventBus<LuaFunction>()

    /**
     * Whose chunk is running. A stack, not a field: a handler registered by a
     * widget script may call `indri.scene.setStyle`, and a nested dispatch is
     * refused rather than impossible, so the value has to unwind reliably.
     */
    private readonly scopes: Scope[] = []

    private snapshot: Readonly<Game> = Object.freeze({})
    /** Registry key of the Lua view of `snapshot`, built on demand and cached. */
    private stateRef: number | null = null
    private installed = false

    constructor(rt: LuaRuntime, deps: HostDeps) {
        this.rt = rt
        this.deps = deps

        this.api = {
            send: (action, payload) => this.deps.send(action, payload),
            state: () => this.snapshot,
            board: {setStyle: (style) => this.overrides.setBoardStyle(style)},
            scene: {
                setStyle: (style) => this.inScene("indri.scene.setStyle", (id) => {
                    this.overrides.setSceneStyle(id, style)
                }),
            },
            widget: (id) => ({
                setStyle: (style) => this.inScene(`indri.widget("${id}").setStyle`, (scene) => {
                    this.overrides.setWidgetStyle(scene, id, style)
                }),
                setConfig: (patch) => this.inScene(`indri.widget("${id}").setConfig`, (scene) => {
                    this.overrides.setWidgetConfig(scene, id, patch)
                }),
            }),
            on: (event, fn) => this.bus.register(this.currentScope(), event, fn),
            log: (...args) => (this.deps.log ?? console.log)(...args),
        }
    }

    /** Install the `indri` global. Idempotent; call before any `instantiate`. */
    install(): LuaResult {
        if (this.installed) return {ok: true, value: undefined}

        const result = this.rt.guard((L) => {
            lua.lua_newtable(L)
            setFunction(L, "send", this.luaSend)
            setFunction(L, "state", this.luaState)
            setFunction(L, "widget", this.luaWidget)
            setFunction(L, "on", this.luaOn)
            setFunction(L, "log", this.luaLog)

            lua.lua_newtable(L)
            setFunction(L, "setStyle", this.luaBoardSetStyle)
            lua.lua_setfield(L, -2, to_luastring("board"))

            lua.lua_newtable(L)
            setFunction(L, "setStyle", this.luaSceneSetStyle)
            lua.lua_setfield(L, -2, to_luastring("scene"))

            lua.lua_setglobal(L, to_luastring("indri"))
        })

        if (result.ok) this.installed = true
        return result
    }

    /**
     * (Re-)run one scope's chunk.
     *
     * THIS IS THE ENTRY POINT FOR "A DELTA CHANGED THIS SCRIPT". It drops the
     * scope's existing handlers and gives the chunk a fresh `_ENV`, so every
     * global the previous version set is gone. Reads still fall through to
     * `_G`, so `indri`, `string`, `table` and `math` stay visible.
     *
     * A consequence worth knowing rather than fixing: widget-local Lua state
     * does not survive remove-then-re-add of the same widget id.
     */
    instantiate(scope: Scope, src: string): LuaResult {
        this.remove(scope)

        return this.rt.guard((L) => {
            // The leading `=` is the same convention `LuaRuntime.run` uses: it
            // tells Lua the chunk name is already a description, so errors read
            // `scene:board:2: ...` instead of `[string "scene:board"]:2: ...`.
            const name = to_luastring(`=${scopeKey(scope)}`)
            if (lauxlib.luaL_loadbuffer(L, to_luastring(src), null, name) !== lua.LUA_OK) {
                throw new LuaScriptError(readTop(L))
            }

            lua.lua_newtable(L) // the chunk's own globals
            lua.lua_newtable(L) // its metatable
            lua.lua_pushglobaltable(L)
            lua.lua_setfield(L, -2, INDEX)
            lua.lua_setmetatable(L, -2)
            // Upvalue 1 of a main chunk is `_ENV`. A chunk with no upvalue at
            // all is possible (fengari returns null), and then the table we
            // just built is ours to drop.
            if (lua.lua_setupvalue(L, -2, 1) === null) lua.lua_pop(L, 1)

            this.scopes.push(scope)
            try {
                if (lua.lua_pcall(L, 0, 0, 0) !== lua.LUA_OK) throw new LuaScriptError(readTop(L))
            } finally {
                this.scopes.pop()
            }
        })
    }

    /** Forget a scope's handlers — its widget or scene is gone. */
    remove(scope: Scope): void {
        this.release(this.bus.removeScope(scope))
    }

    /**
     * Keep handlers for the listed scopes and drop the rest. This is how a
     * handler registered for a widget a delta removed stops being reachable:
     * pass the scopes the current layout still contains.
     */
    retain(scopes: Iterable<Scope>): void {
        this.release(this.bus.retain(scopes))
    }

    /**
     * Publish new authoritative state and fire `stateChanged`.
     *
     * `kind` is not cosmetic. A KEYFRAME IS A RESYNC, so the override layer is
     * cleared: whatever a script painted was computed from state that no longer
     * applies. A DELTA IS INCREMENTAL and leaves the overrides alone, or every
     * tick would wipe the script's work.
     */
    applyGameState(game: Game, kind: "keyframe" | "delta"): EmitOutcome {
        if (kind === "keyframe") this.overrides.clearForKeyframe()
        this.setSnapshot(game)
        return this.broadcast("stateChanged", this.snapshot)
    }

    /** Deliver a targeted event, bubbling widget -> scene -> board. */
    emit(event: LuaEvent, target: Scope, arg?: unknown): EmitOutcome {
        const errors: string[] = []
        return {...this.bus.dispatch(event, target, this.invoker(arg, errors)), errors}
    }

    /** Deliver to every registered scope; a `false` return does not stop it. */
    broadcast(event: LuaEvent, arg?: unknown): EmitOutcome {
        const errors: string[] = []
        return {...this.bus.broadcast(event, this.invoker(arg, errors)), errors}
    }

    /** Release every registry reference. Does not dispose the runtime. */
    dispose(): void {
        this.release(this.bus.clear())
        this.dropStateRef()
    }

    // ---- internals -------------------------------------------------------

    private currentScope(): Scope {
        return this.scopes[this.scopes.length - 1] ?? BOARD_SCOPE
    }

    private inScene(what: string, apply: (sceneId: string) => void): void {
        const scope = this.currentScope()
        const sceneId = scope.kind === "board" ? this.deps.activeSceneId() : scope.sceneId
        if (sceneId === undefined) {
            this.api.log(`indri: ${what} was ignored — no scene is active`)
            return
        }
        apply(sceneId)
    }

    private setSnapshot(game: Game): void {
        // Cloned through JSON first: it detaches the snapshot from the live
        // object, matches what `GameStateParser` already does, and normalises
        // away anything the Lua marshaller could not represent.
        this.snapshot = deepFreeze(JSON.parse(JSON.stringify(game)) as Game)
        this.dropStateRef()
    }

    private dropStateRef(): void {
        const ref = this.stateRef
        if (ref === null) return
        this.stateRef = null
        this.rt.guard((L) => {
            lauxlib.luaL_unref(L, lua.LUA_REGISTRYINDEX, ref)
        })
    }

    private release(fns: readonly LuaFunction[]): void {
        if (fns.length === 0) return
        this.rt.guard((L) => {
            for (const fn of fns) lauxlib.luaL_unref(L, lua.LUA_REGISTRYINDEX, fn.ref)
        })
    }

    /**
     * Each handler runs in its OWN top-level `guard`, so it gets its own
     * instruction budget and its own error containment: one broken or greedy
     * script cannot silence the handlers above it in the chain.
     */
    private invoker(arg: unknown, errors: string[]): Invoke<LuaFunction> {
        return (fn, scope) => {
            this.scopes.push(scope)
            let result: LuaResult<boolean>
            try {
                result = this.rt.guard((L) => {
                    lua.lua_rawgeti(L, lua.LUA_REGISTRYINDEX, fn.ref)
                    this.pushArg(L, arg)
                    if (lua.lua_pcall(L, 1, 1, 0) !== lua.LUA_OK) {
                        throw new LuaScriptError(readTop(L))
                    }
                    // ONLY a literal `false` stops the chain. A handler that
                    // falls off its end returns nil, and reading that as "stop"
                    // would make every handler accidentally exclusive.
                    return !(lua.lua_type(L, -1) === lua.LUA_TBOOLEAN && !lua.lua_toboolean(L, -1))
                })
            } finally {
                this.scopes.pop()
            }

            if (result.ok) return result.value
            errors.push(result.error)
            return true
        }
    }

    private pushArg(L: lua_State, arg: unknown): void {
        // An identity check, not a type check: the Lua view of the state is
        // expensive to build and is cached, and `applyGameState` broadcasts the
        // very object `state()` returns.
        if (arg === this.snapshot) this.pushState(L)
        else pushReadOnly(L, arg, 1)
    }

    private pushState(L: lua_State): void {
        if (this.stateRef !== null) {
            lua.lua_rawgeti(L, lua.LUA_REGISTRYINDEX, this.stateRef)
            return
        }
        pushReadOnly(L, this.snapshot, 1)
        lua.lua_pushvalue(L, -1)
        this.stateRef = lauxlib.luaL_ref(L, lua.LUA_REGISTRYINDEX)
    }

    private readStyle(L: lua_State, what: string, idx: number): Style {
        // Validated here because here is the untrusted boundary: after this the
        // override layer and the renderer may treat a style as well-formed.
        const parsed = StyleSchema.safeParse(toJs(L, idx, 1))
        if (parsed.success) return parsed.data

        const issue = parsed.error.issues[0]
        const at = issue === undefined || issue.path.length === 0 ? "style" : issue.path.join(".")
        raise(L, `${what}: ${at} — ${issue?.message ?? "is not a valid style"}`)
    }

    // ---- host functions --------------------------------------------------

    private readonly luaSend: LuaCFunction = (L) => {
        const action = lua.lua_tojsstring(L, 1)
        if (action === null || action === "") {
            raise(L, "indri.send(action, payload): action must be a non-empty string")
        }

        const payload = isAbsent(L, 2) ? {} : toJs(L, 2, 1)
        if (!isRecord(payload)) raise(L, `indri.send("${action}", payload): payload must be a table`)

        let failure: unknown
        try {
            this.deps.send(action, payload)
        } catch (e) {
            failure = e
        }
        if (failure !== undefined) raise(L, `indri.send("${action}") failed: ${describe(failure)}`)

        return 0
    }

    private readonly luaState: LuaCFunction = (L) => {
        this.pushState(L)
        return 1
    }

    private readonly luaOn: LuaCFunction = (L) => {
        const event = lua.lua_tojsstring(L, 1)
        if (event === null || !isLuaEvent(event)) {
            raise(
                L,
                `indri.on(event, fn): "${event ?? "nil"}" is not an event; ` +
                `expected one of ${LUA_EVENTS.join(", ")}`,
            )
        }
        if (lua.lua_type(L, 2) !== lua.LUA_TFUNCTION) {
            raise(L, "indri.on(event, fn): fn must be a function")
        }

        lua.lua_pushvalue(L, 2)
        this.bus.register(this.currentScope(), event, {
            ref: lauxlib.luaL_ref(L, lua.LUA_REGISTRYINDEX),
        })
        return 0
    }

    private readonly luaLog: LuaCFunction = (L) => {
        const parts: string[] = []
        for (let i = 1, n = lua.lua_gettop(L); i <= n; i++) {
            parts.push(to_jsstring(lauxlib.luaL_tolstring(L, i)))
            lua.lua_pop(L, 1)
        }
        this.api.log(...parts)
        return 0
    }

    private readonly luaBoardSetStyle: LuaCFunction = (L) => {
        this.overrides.setBoardStyle(this.readStyle(L, "indri.board.setStyle", 1))
        return 0
    }

    private readonly luaSceneSetStyle: LuaCFunction = (L) => {
        const style = this.readStyle(L, "indri.scene.setStyle", 1)
        this.inScene("indri.scene.setStyle", (sceneId) => {
            this.overrides.setSceneStyle(sceneId, style)
        })
        return 0
    }

    private readonly luaWidget: LuaCFunction = (L) => {
        const id = lua.lua_tojsstring(L, 1)
        if (id === null || id === "") raise(L, "indri.widget(id): id must be a non-empty string")

        lua.lua_newtable(L)

        setFunction(L, "setStyle", (inner) => {
            const what = `indri.widget("${id}").setStyle`
            const style = this.readStyle(inner, what, 1)
            this.inScene(what, (sceneId) => this.overrides.setWidgetStyle(sceneId, id, style))
            return 0
        })

        setFunction(L, "setConfig", (inner) => {
            const what = `indri.widget("${id}").setConfig`
            const patch = toJs(inner, 1, 1)
            if (!isRecord(patch)) raise(inner, `${what}: patch must be a table`)
            this.inScene(what, (sceneId) => this.overrides.setWidgetConfig(sceneId, id, patch))
            return 0
        })

        return 1
    }
}

// ---- marshalling ---------------------------------------------------------

/**
 * Push a JS value as a Lua value. Objects and arrays become READ-ONLY VIEWS.
 *
 * The view is a proxy — an empty table whose `__index` is the real data —
 * rather than the data table with a `__newindex` on it, because `__newindex`
 * is skipped for a key that already exists. Assigning to `state.code` on such a
 * table would succeed silently, which is the exact failure this must prevent.
 * `__metatable` is set so a script cannot swap the protection out, and
 * `./state.ts` has already removed `rawset`, which would walk around it.
 */
function pushReadOnly(L: lua_State, value: unknown, depth: number): void {
    reserve(L)

    switch (typeof value) {
        case "boolean":
            lua.lua_pushboolean(L, value)
            return
        case "number":
            // Integer vs float is visible in Lua: `tostring(2)` is "2" but
            // `tostring(2.0)` is "2.0", and a score rendered as "3.0" is a bug
            // the script author cannot fix.
            if (Number.isInteger(value)) lua.lua_pushinteger(L, value)
            else lua.lua_pushnumber(L, value)
            return
        case "string":
            lua.lua_pushstring(L, to_luastring(value))
            return
        case "object":
            break
        default:
            // undefined, and the function/symbol/bigint that JSON cannot carry.
            lua.lua_pushnil(L)
            return
    }

    if (value === null || depth > MAX_STATE_DEPTH) {
        lua.lua_pushnil(L)
        return
    }

    lua.lua_newtable(L)
    if (Array.isArray(value)) {
        for (let i = 0; i < value.length; i++) {
            lua.lua_pushinteger(L, i + 1) // Lua arrays are 1-based
            pushReadOnly(L, value[i], depth + 1)
            lua.lua_rawset(L, -3)
        }
    } else {
        for (const [key, child] of Object.entries(value)) {
            if (child === undefined) continue // a nil value would delete the key
            lua.lua_pushstring(L, to_luastring(key))
            pushReadOnly(L, child, depth + 1)
            lua.lua_rawset(L, -3)
        }
    }
    wrapReadOnly(L)
}

/** Replace the table on top of the stack with a read-only proxy over it. */
function wrapReadOnly(L: lua_State): void {
    lua.lua_newtable(L) // proxy
    lua.lua_newtable(L) // metatable

    lua.lua_pushvalue(L, -3)
    lua.lua_setfield(L, -2, INDEX)
    lua.lua_pushcfunction(L, refuseWrite)
    lua.lua_setfield(L, -2, NEWINDEX)
    // `#t` and `pairs(t)` would otherwise see the empty proxy, not the data.
    lua.lua_pushcfunction(L, proxyLen)
    lua.lua_setfield(L, -2, LEN)
    lua.lua_pushcfunction(L, proxyPairs)
    lua.lua_setfield(L, -2, PAIRS)
    lua.lua_pushboolean(L, true)
    lua.lua_setfield(L, -2, READ_ONLY)
    lua.lua_pushboolean(L, false)
    lua.lua_setfield(L, -2, METATABLE)

    lua.lua_setmetatable(L, -2)
    lua.lua_remove(L, -2) // drop the data table; the proxy holds it
}

const refuseWrite: LuaCFunction = (L) => {
    const key = lua.lua_type(L, 2) === lua.LUA_TSTRING ? lua.lua_tojsstring(L, 2) : null
    raise(
        L,
        `cannot assign to ${key === null ? "a field" : `"${key}"`}: game state is read-only. ` +
        "Lua drives behaviour, the server owns state — use indri.send to ask for a change.",
    )
}

const proxyLen: LuaCFunction = (L) => {
    pushBacking(L, 1)
    lua.lua_pushinteger(L, lua.lua_rawlen(L, -1))
    return 1
}

/** `pairs(proxy)` iterates the backing table, since the proxy itself is empty. */
const proxyPairs: LuaCFunction = (L) => {
    lua.lua_pushcfunction(L, luaNext)
    pushBacking(L, 1)
    lua.lua_pushnil(L)
    return 3
}

const luaNext: LuaCFunction = (L) => {
    lua.lua_settop(L, 2)
    if (lua.lua_next(L, 1) !== 0) return 2
    lua.lua_pushnil(L)
    return 1
}

/**
 * Push the data table behind a read-only proxy. `__metatable` hides the
 * metatable from Lua, but not from here — `lua_getmetatable` ignores it.
 */
function pushBacking(L: lua_State, idx: number): void {
    if (!lua.lua_getmetatable(L, idx)) raise(L, "read-only view lost its metatable")
    lua.lua_getfield(L, -1, INDEX)
    lua.lua_remove(L, -2)
}

/** True when the value at `idx` is a read-only proxy this module made. */
function isReadOnlyProxy(L: lua_State, idx: number): boolean {
    if (!lua.lua_getmetatable(L, idx)) return false
    lua.lua_getfield(L, -1, READ_ONLY)
    const marked = lua.lua_toboolean(L, -1)
    lua.lua_pop(L, 2)
    return marked
}

/**
 * Marshal a Lua value at `idx` into JS. Raises a Lua error for anything that
 * cannot cross — a function, userdata, or a table nested past
 * `MAX_PAYLOAD_DEPTH` — rather than letting a bad value reach `JSON.stringify`
 * and throw from inside a callback.
 */
function toJs(L: lua_State, idx: number, depth: number): unknown {
    switch (lua.lua_type(L, idx)) {
        case lua.LUA_TNONE:
        case lua.LUA_TNIL:
            return undefined
        case lua.LUA_TBOOLEAN:
            return lua.lua_toboolean(L, idx)
        case lua.LUA_TNUMBER:
            return lua.lua_tonumber(L, idx)
        case lua.LUA_TSTRING:
            return lua.lua_tojsstring(L, idx) ?? ""
        case lua.LUA_TTABLE:
            return tableToJs(L, idx, depth)
        default:
            raise(L, "only nil, booleans, numbers, strings and tables can be passed to the host")
    }
}

function tableToJs(L: lua_State, idx: number, depth: number): unknown {
    if (depth > MAX_PAYLOAD_DEPTH) {
        raise(
            L,
            `table is nested more than ${MAX_PAYLOAD_DEPTH} levels deep; ` +
            "a table that refers to itself reaches this limit too",
        )
    }

    // Restored on the way out, but DELIBERATELY NOT IN A `finally`: `luaL_error`
    // pushes its message and then throws, so a finally here would pop the very
    // message the enclosing pcall is about to read. The failure path is the
    // enclosing `LuaRuntime.guard`'s job — it restores the stack unconditionally.
    const base = lua.lua_gettop(L)
    reserve(L)

    let table = lua.lua_absindex(L, idx)
    // A read-only view stores nothing of its own, so iterating it directly
    // would silently yield an empty table.
    if (isReadOnlyProxy(L, table)) {
        pushBacking(L, table)
        table = lua.lua_gettop(L)
    }

    const keys: (string | number)[] = []
    const values: unknown[] = []

    lua.lua_pushnil(L)
    while (lua.lua_next(L, table) !== 0) {
        // Read the key WITHOUT converting it: `lua_tojsstring` rewrites a
        // number key in place, and `lua_next` would then lose its place.
        const keyType = lua.lua_type(L, -2)
        if (keyType === lua.LUA_TSTRING) keys.push(lua.lua_tojsstring(L, -2) ?? "")
        else if (keyType === lua.LUA_TNUMBER) keys.push(lua.lua_tonumber(L, -2))
        else {
            lua.lua_pop(L, 1)
            continue
        }
        values.push(toJs(L, -1, depth + 1))
        lua.lua_pop(L, 1)
    }

    lua.lua_settop(L, base)
    return keys.length > 0 && isDenseArray(keys)
        ? toArray(keys as number[], values)
        : toObject(keys, values)
}

/** Exactly the keys 1..n, which is the only shape that round-trips as a JSON array. */
function isDenseArray(keys: readonly (string | number)[]): boolean {
    const seen = new Set<number>()
    for (const key of keys) {
        if (typeof key !== "number" || !Number.isInteger(key) || key < 1 || key > keys.length) {
            return false
        }
        seen.add(key)
    }
    return seen.size === keys.length
}

function toArray(keys: readonly number[], values: readonly unknown[]): unknown[] {
    const out = new Array<unknown>(keys.length)
    keys.forEach((key, i) => {
        out[key - 1] = values[i]
    })
    return out
}

function toObject(
    keys: readonly (string | number)[],
    values: readonly unknown[],
): Record<string, unknown> {
    const out: Record<string, unknown> = {}
    keys.forEach((key, i) => {
        out[String(key)] = values[i]
    })
    return out
}

// ---- small helpers -------------------------------------------------------

/**
 * Make room for one more level of marshalling.
 *
 * Thrown rather than raised through `luaL_error`: raising needs stack room to
 * push its message, which is precisely what is missing here. The throw unwinds
 * to the enclosing `LuaRuntime.guard`, which restores the stack and reports it
 * like any other script failure. Unreachable in practice — `LUAI_MAXSTACK` is a
 * million slots and the depth caps bound this at a few hundred — but a silently
 * ignored failure here is the bug this function exists to prevent.
 */
function reserve(L: lua_State): void {
    if (!lua.lua_checkstack(L, MARSHAL_SLOTS)) {
        throw new LuaScriptError(`cannot reserve ${MARSHAL_SLOTS} more Lua stack slots`)
    }
}

/** Push a host function and store it under `name` in the table below it. */
function setFunction(L: lua_State, name: string, fn: LuaCFunction): void {
    lua.lua_pushcfunction(L, fn)
    lua.lua_setfield(L, -2, to_luastring(name))
}

/**
 * Raise a Lua error carrying `message`.
 *
 * `%` is doubled because `luaL_error` runs its argument through the format
 * engine, and a message built from author-supplied ids can contain one.
 */
function raise(L: lua_State, message: string): never {
    return lauxlib.luaL_error(L, to_luastring(message.replace(/%/g, "%%")))
}

function isAbsent(L: lua_State, idx: number): boolean {
    const t = lua.lua_type(L, idx)
    return t === lua.LUA_TNONE || t === lua.LUA_TNIL
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value)
}

/** Read the error object Lua left on top of the stack. */
function readTop(L: lua_State): string {
    return lua.lua_tojsstring(L, -1) ?? "lua error with no string representation"
}

function describe(e: unknown): string {
    return e instanceof Error ? `${e.name}: ${e.message}` : String(e)
}

/**
 * Freeze in place. The caller always hands over a fresh JSON clone, so nothing
 * shared is affected; `isFrozen` also makes the walk safe against a cycle.
 */
function deepFreeze<T>(value: T): T {
    if (value === null || typeof value !== "object" || Object.isFrozen(value)) return value
    Object.freeze(value)
    for (const child of Object.values(value)) deepFreeze(child)
    return value
}

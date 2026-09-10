/**
 * The Lua runtime: one long-lived sandboxed state, plus the guards that every
 * entry into Lua passes through.
 *
 * Three properties matter here, and all three are enforced in `guard`:
 *
 * 1. ERRORS ARE VALUES. Nothing in this module throws at its caller. A syntax
 *    error, a Lua `error()`, an instruction-budget abort and a JS exception
 *    raised inside a host function all come back as `{ok: false, error}`, and
 *    are additionally reported to `onError`. This mirrors the Go router's
 *    recover()-per-handler stance: a broken script must not take the renderer
 *    down with it.
 *
 * 2. THE LUA STACK IS ALWAYS DRAINED. Every entry records `lua_gettop` on the
 *    way in and restores it in a `finally`, on the success path and on both
 *    error paths. Lua gives no signal when a slot is leaked, and one leaked
 *    slot per event is an unbounded leak over a long session, so the restore is
 *    unconditional rather than per-path.
 *
 * 3. THE INSTRUCTION BUDGET IS BEST-EFFORT, NOT A GUARANTEE. See
 *    `LuaRuntimeOptions.instructionBudget`.
 *
 * The runtime deliberately knows nothing about the host API, events or the
 * websocket. `guard` is the seam those build on.
 */

import {lauxlib, lua, to_luastring} from "fengari"

import {createSandboxedState} from "./state.ts"

import type {lua_State} from "fengari"

export type LuaResult<T = void> =
    | {readonly ok: true; readonly value: T}
    | {readonly ok: false; readonly error: string}

export interface LuaRuntimeOptions {
    /**
     * Ceiling on Lua VM instructions executed per top-level call, enforced by a
     * `LUA_MASKCOUNT` debug hook and reset at the start of each top-level call.
     *
     * BEST-EFFORT, NOT A GUARANTEE. It bounds VM instructions, which is not the
     * same as bounding time or memory:
     *
     * - A count hook cannot interrupt a host function. Anything implemented in
     *   JS — a `string`/`table` library call, or a function this runtime hands
     *   to Lua later — runs to completion no matter how long it takes. A single
     *   `string.rep(s, 1e9)` is one instruction.
     * - Count hooks are per-coroutine. The `coroutine` library is not opened,
     *   so no script can currently create one, but a thread created by host
     *   code would not inherit this budget.
     *
     * Treat it as a guard against an accidental runaway loop, not as a defence
     * against a hostile script.
     */
    instructionBudget?: number

    /**
     * Called once per failure, with the same string the failing call returns.
     * Wire it to a dev overlay or a log; it is never called on success.
     */
    onError?: (err: string) => void
}

export const DEFAULT_INSTRUCTION_BUDGET = 200_000

/**
 * How many times the count hook fires across one full budget. The hook is
 * cheap, but it is JS called from the VM's inner loop, so this trades abort
 * precision against overhead: the budget can overshoot by up to one interval.
 */
const HOOK_FIRINGS_PER_BUDGET = 200

const DISPOSED_ERROR = "lua runtime is disposed"

/**
 * Carries a Lua-side message out of the operation and into `guard`'s catch, so
 * that Lua failures and JS failures share one exit path — and therefore one
 * stack restore and one `onError` call.
 *
 * Exported for other code building on `guard`: a Lua message thrown as one of
 * these is reported verbatim, whereas any other error is prefixed with its JS
 * type, which would bury `board:3: attempt to index a nil value` under
 * `Error:`.
 */
export class LuaScriptError extends Error {}

export class LuaRuntime {
    private L: lua_State | null
    private readonly budget: number
    private readonly hookInterval: number
    private readonly onError: ((err: string) => void) | undefined

    /** VM instructions left in the current top-level call. */
    private remaining: number

    /** `guard` nesting depth. Only the outermost entry resets the budget. */
    private depth = 0

    constructor(opts: LuaRuntimeOptions = {}) {
        const budget = opts.instructionBudget ?? DEFAULT_INSTRUCTION_BUDGET
        if (!Number.isFinite(budget) || budget < 1) {
            throw new RangeError(`instructionBudget must be a finite number >= 1, got ${budget}`)
        }

        this.budget = Math.floor(budget)
        this.hookInterval = Math.max(1, Math.floor(this.budget / HOOK_FIRINGS_PER_BUDGET))
        this.remaining = this.budget
        this.onError = opts.onError

        const L = createSandboxedState()
        // Installed once and left in place. fengari's hook counter lives on the
        // state and keeps decrementing across separate pcalls, which is exactly
        // why `remaining` has to be reset per top-level call rather than being
        // read back out of the hook.
        lua.lua_sethook(L, this.hook, lua.LUA_MASKCOUNT, this.hookInterval)
        this.L = L
    }

    /**
     * Load and run a chunk. `chunkName` is a logical id (a scene or widget id),
     * not a file, and it is what error messages are prefixed with.
     */
    run(src: string, chunkName: string): LuaResult {
        return this.guard((L) => {
            const name = to_luastring(luaChunkName(chunkName))
            if (lauxlib.luaL_loadbuffer(L, to_luastring(src), null, name) !== lua.LUA_OK) {
                throw new LuaScriptError(readTop(L))
            }
            if (lua.lua_pcall(L, 0, 0, 0) !== lua.LUA_OK) {
                throw new LuaScriptError(readTop(L))
            }
        })
    }

    /**
     * Run `op` against the raw Lua state under this runtime's guards: the
     * instruction budget is reset (outermost entry only), the stack is restored
     * to its entry depth, and both Lua and JS errors are captured as values.
     *
     * This is the seam the host API and event dispatch are built on — they need
     * the state, and they need exactly these guarantees. `op` may leave values
     * on the stack; they are discarded on the way out, so anything it wants to
     * keep must be returned as a JS value.
     */
    guard<T>(op: (L: lua_State) => T): LuaResult<T> {
        const L = this.L
        if (L === null) return this.fail(DISPOSED_ERROR)

        const base = lua.lua_gettop(L)
        if (this.depth === 0) this.remaining = this.budget
        this.depth++

        try {
            return {ok: true, value: op(L)}
        } catch (e) {
            return this.fail(describeError(e))
        } finally {
            this.depth--
            // `op` is arbitrary host code and may have disposed the runtime.
            // `lua_close` frees the stack, so restoring it would throw a
            // TypeError straight out of this `finally` — past every guarantee
            // this method makes.
            if (this.L !== null) lua.lua_settop(L, base)
        }
    }

    /** Release the state. Idempotent; every later call fails as a value. */
    dispose(): void {
        const L = this.L
        if (L === null) return

        this.L = null
        lua.lua_sethook(L, null, 0, 0) // drop the hook's reference to this runtime
        lua.lua_close(L)
    }

    /**
     * Arrow property, so it stays bound when fengari calls it. fengari invokes
     * hooks as `(L, ar)`; the debug record is not used.
     */
    private readonly hook = (L: lua_State): void => {
        this.remaining -= this.hookInterval
        if (this.remaining > 0) return

        // Raised from inside the hook, this aborts the running chunk with
        // LUA_ERRRUN, so the enclosing pcall reports it like any other error.
        lauxlib.luaL_error(L, to_luastring(`instruction budget of ${this.budget} exceeded`))
    }

    private fail(error: string): {ok: false; error: string} {
        try {
            this.onError?.(error)
        } catch (e) {
            // A reporter that throws must not convert a handled script error
            // into an unhandled one — that is the failure mode this whole class
            // exists to prevent.
            console.warn(`lua onError handler threw while reporting ${JSON.stringify(error)}:`, e)
        }

        return {ok: false, error}
    }
}

/**
 * Lua reads a leading `=` as "this name is already a description, show it
 * verbatim". Without it a plain name is treated as source text and errors read
 * `[string "board"]:1: ...` instead of `board:1: ...`. `@` (a filename) and a
 * name already starting with `=` are passed through untouched.
 */
function luaChunkName(name: string): string {
    return name.startsWith("=") || name.startsWith("@") ? name : `=${name}`
}

/**
 * Read the error object Lua left on top of the stack. `lua_tostring` returns
 * null for a value with no string form, which `error({code = 1})` produces, so
 * the null case is reachable from an ordinary script.
 */
function readTop(L: lua_State): string {
    return lua.lua_tojsstring(L, -1) ?? "lua error with no string representation"
}

function describeError(e: unknown): string {
    if (e instanceof LuaScriptError) return e.message
    if (e instanceof Error) return `${e.name}: ${e.message}`
    return String(e)
}

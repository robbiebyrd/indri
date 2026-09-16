import {lauxlib, lua, to_jsstring, to_luastring} from "fengari"
import type {lua_State} from "fengari"

import {createSandboxedState} from "./state.ts"

export type LuaResult = {ok: true; value?: string} | {ok: false; error: string}

/**
 * Maximum Lua VM instructions before the budget hook fires.
 *
 * IMPORTANT: This is best-effort, NOT a hard limit.
 * Count hooks are per-coroutine and cannot interrupt inside a host (JS) function
 * call. A script that spends all its time in host calls will not be interrupted
 * by this budget. Use it to catch runaway pure-Lua loops, not adversarial code.
 */
const INSTRUCTION_BUDGET = 1_000_000

/**
 * Install a best-effort instruction budget on an existing Lua state.
 *
 * The hook fires every INSTRUCTION_BUDGET VM instructions and aborts the running
 * chunk via luaL_error. Because count hooks are per-coroutine and do not fire
 * during host calls, this provides protection against runaway Lua loops, not a
 * strict execution-time cap.
 */
function installBudgetHook(L: lua_State): void {
    lua.lua_sethook(
        L,
        (hookL) => {
            lauxlib.luaL_error(hookL, to_luastring("instruction budget exceeded"))
        },
        lua.LUA_MASKCOUNT,
        INSTRUCTION_BUDGET,
    )
}

/**
 * Create a sandboxed Lua state with the instruction budget hook pre-installed.
 *
 * The returned state is long-lived — one state per widget script scope. Callers
 * run chunks against it via runChunk. Session lifecycle is managed by the caller
 * (Story 020).
 */
export function createRuntimeState(): lua_State {
    const L = createSandboxedState()
    installBudgetHook(L)
    return L
}

/**
 * Load and execute a Lua chunk in an existing state.
 *
 * Returns {ok: true} on success, or {ok: false, error: "..."} on any failure
 * (syntax error, runtime error, budget exceeded). Never throws.
 *
 * Stack contract: the Lua stack is always fully drained on both success and
 * failure paths. A stack slot leak per call would be a memory leak over a long
 * session.
 *
 * Note: runChunk does not return Lua values — it just executes the chunk. The
 * runtime layer (Story 020) manages state reads and callbacks.
 */
export function runChunk(L: lua_State, src: string, name: string): LuaResult {
    // Load the chunk. luaL_loadbuffer sets the chunk name correctly, which
    // Lua includes in error messages as "name:line: message".
    const loadStatus = lauxlib.luaL_loadbuffer(
        L,
        to_luastring(src),
        null,
        to_luastring(name),
    )
    if (loadStatus !== lua.LUA_OK) {
        const error = to_jsstring(lua.lua_tostring(L, -1))
        lua.lua_pop(L, 1) // pop error message left by luaL_loadbuffer
        return {ok: false, error}
    }

    // Execute the chunk. 0 args, 0 results, no message handler.
    // Discarding return values keeps the stack clean without needing to know
    // how many values the chunk returned.
    const callStatus = lua.lua_pcall(L, 0, 0, 0)
    if (callStatus !== lua.LUA_OK) {
        const error = to_jsstring(lua.lua_tostring(L, -1))
        lua.lua_pop(L, 1) // pop error message left by lua_pcall
        return {ok: false, error}
    }

    return {ok: true}
}

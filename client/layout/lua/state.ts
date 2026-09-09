import {lauxlib, lua, lualib, to_luastring} from "fengari"

import type {LuaOpenFn, lua_State} from "fengari"

/**
 * The only standard libraries a game script may use.
 *
 * `io`, `os`, `package` and `debug` are never opened, so a script cannot see
 * them. Note they ARE in the bundle: fengari's `lstrlib` and `ltablib` both
 * require `lualib`, which unconditionally pulls in loslib, loadlib and ldblib.
 * There is no import arrangement that keeps them out while still providing
 * `string` and `table` — verified by comparing module graphs. The sandbox is
 * therefore "never opened", not "never shipped".
 */
const SAFE_LIBS: ReadonlyArray<readonly [string, LuaOpenFn]> = [
    ["_G", lualib.luaopen_base],
    ["string", lualib.luaopen_string],
    ["table", lualib.luaopen_table],
    ["math", lualib.luaopen_math],
]

/**
 * Globals that `luaopen_base` installs and that we remove by hand. Selective
 * `luaL_requiref` is NOT a sandbox on its own — base brings these with it.
 *
 * `load`/`loadstring`/`dofile`/`loadfile` matter most: a script able to compile
 * new chunks at runtime bypasses every restriction placed on the chunk we
 * loaded, which is the classic way out of a Lua sandbox.
 *
 * The `raw*` family is denied because it reads and writes tables while ignoring
 * metatables. The host API hands scripts a read-only view of game state
 * protected by a metatable, and `rawset` would walk straight through it.
 *
 * `collectgarbage` goes because fengari delegates to the JS GC, which makes the
 * function misleading rather than useful.
 */
const DENIED_BASE_GLOBALS: readonly string[] = [
    "load",
    "loadstring",
    "dofile",
    "loadfile",
    "require",
    "rawset",
    "rawget",
    "rawequal",
    "rawlen",
    "collectgarbage",
]

/**
 * Create a Lua state with a deny-by-default standard library.
 *
 * Deliberately does NOT call `luaL_openlibs`, which would expose io, os,
 * package and debug to scripts.
 */
export function createSandboxedState(): lua_State {
    const L = lauxlib.luaL_newstate()

    for (const [name, openf] of SAFE_LIBS) {
        lauxlib.luaL_requiref(L, to_luastring(name), openf, 1)
        lua.lua_pop(L, 1) // luaL_requiref leaves the module on the stack
    }

    for (const name of DENIED_BASE_GLOBALS) {
        lua.lua_pushnil(L)
        lua.lua_setglobal(L, to_luastring(name))
    }

    return L
}

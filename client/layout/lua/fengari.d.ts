// fengari ships no type declarations, so this is a hand-written surface
// covering only what the layout engine uses.

declare module "fengari" {
    /** Opaque handle to a Lua interpreter state. */
    export type lua_State = {readonly __brand: unique symbol}

    /** Debug record handed to a hook. Opaque — we never read it. */
    export type lua_Debug = {readonly __brand: unique symbol}

    export type LuaOpenFn = (L: lua_State) => number

    export function to_luastring(s: string, cache?: boolean): Uint8Array
    export function to_jsstring(a: Uint8Array): string

    export const lua: {
        readonly LUA_OK: 0
        readonly LUA_ERRRUN: number
        readonly LUA_ERRSYNTAX: number
        readonly LUA_ERRMEM: number

        readonly LUA_MASKCALL: number
        readonly LUA_MASKRET: number
        readonly LUA_MASKLINE: number
        readonly LUA_MASKCOUNT: number

        lua_pop(L: lua_State, n: number): void
        lua_settop(L: lua_State, n: number): void
        lua_gettop(L: lua_State): number
        lua_pushnil(L: lua_State): void
        lua_setglobal(L: lua_State, name: Uint8Array): void
        lua_getglobal(L: lua_State, name: Uint8Array): number
        lua_pcall(L: lua_State, nargs: number, nresults: number, errfunc: number): number
        lua_tostring(L: lua_State, idx: number): Uint8Array
        lua_type(L: lua_State, idx: number): number

        /**
         * Install a debug hook. Verified against fengari 0.1.5: the callback is
         * invoked as (L, ar), and raising from it via luaL_error aborts the
         * running chunk with LUA_ERRRUN.
         */
        lua_sethook(
            L: lua_State,
            f: ((L: lua_State, ar: lua_Debug) => void) | null,
            mask: number,
            count: number,
        ): void
    }

    export const lauxlib: {
        luaL_newstate(): lua_State
        luaL_requiref(L: lua_State, modname: Uint8Array, openf: LuaOpenFn, glb: number): void
        luaL_loadbuffer(
            L: lua_State,
            buff: Uint8Array,
            name: Uint8Array | null,
            chunkname: Uint8Array,
        ): number
        luaL_error(L: lua_State, fmt: Uint8Array): never
    }

    export const lualib: {
        readonly luaopen_base: LuaOpenFn
        readonly luaopen_string: LuaOpenFn
        readonly luaopen_table: LuaOpenFn
        readonly luaopen_math: LuaOpenFn
        readonly luaopen_coroutine: LuaOpenFn
        // luaopen_io / luaopen_os / luaopen_package / luaopen_debug exist too.
        // They are deliberately not declared: nothing in this codebase may open
        // them, and leaving them off the type keeps that honest.
    }
}

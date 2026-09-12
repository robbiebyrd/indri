// fengari ships no type declarations, so this is a hand-written surface
// covering only what the layout engine uses.

declare module "fengari" {
    /** Opaque handle to a Lua interpreter state. */
    export type lua_State = {readonly __brand: unique symbol}

    /** Debug record handed to a hook. Opaque — we never read it. */
    export type lua_Debug = {readonly __brand: unique symbol}

    export type LuaOpenFn = (L: lua_State) => number

    /**
     * A host function callable from Lua. Returns the number of values it left
     * on the stack. It may raise a Lua error via `lauxlib.luaL_error`; any
     * other JS throw escapes the enclosing `lua_pcall` entirely, so host
     * functions must not leak one.
     */
    export type LuaCFunction = (L: lua_State) => number

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

        readonly LUA_TNONE: number
        readonly LUA_TNIL: number
        readonly LUA_TBOOLEAN: number
        readonly LUA_TNUMBER: number
        readonly LUA_TSTRING: number
        readonly LUA_TTABLE: number
        readonly LUA_TFUNCTION: number

        readonly LUA_REGISTRYINDEX: number

        lua_pop(L: lua_State, n: number): void
        lua_settop(L: lua_State, n: number): void
        lua_gettop(L: lua_State): number
        lua_absindex(L: lua_State, idx: number): number
        /** False when the stack cannot grow. Lua never grows it on its own. */
        lua_checkstack(L: lua_State, n: number): boolean
        lua_remove(L: lua_State, idx: number): void
        lua_pushvalue(L: lua_State, idx: number): void
        lua_pushnil(L: lua_State): void
        lua_pushboolean(L: lua_State, b: boolean): void
        lua_pushnumber(L: lua_State, n: number): void
        lua_pushinteger(L: lua_State, n: number): void
        lua_pushstring(L: lua_State, s: Uint8Array): void
        lua_pushcfunction(L: lua_State, fn: LuaCFunction): void
        lua_pushglobaltable(L: lua_State): void
        lua_newtable(L: lua_State): void
        lua_setglobal(L: lua_State, name: Uint8Array): void
        lua_getglobal(L: lua_State, name: Uint8Array): number
        lua_setfield(L: lua_State, idx: number, k: Uint8Array): void
        lua_getfield(L: lua_State, idx: number, k: Uint8Array): number
        lua_rawgeti(L: lua_State, idx: number, n: number): number
        lua_rawset(L: lua_State, idx: number): void
        lua_rawlen(L: lua_State, idx: number): number
        lua_setmetatable(L: lua_State, idx: number): void
        /**
         * BOOLEAN, not the 0/1 the C API returns — verified against fengari
         * 0.1.5's `lapi.js`. Pushes the metatable only when it returns true.
         */
        lua_getmetatable(L: lua_State, idx: number): boolean
        /** 1 when a key/value pair was pushed, 0 when the table is exhausted. */
        lua_next(L: lua_State, idx: number): number
        lua_pcall(L: lua_State, nargs: number, nresults: number, errfunc: number): number
        lua_type(L: lua_State, idx: number): number
        lua_toboolean(L: lua_State, idx: number): boolean
        lua_tonumber(L: lua_State, idx: number): number

        /**
         * Sets upvalue `n` of the function at `funcindex`, popping the value.
         * Returns the upvalue's name, or null when it does not exist. Upvalue 1
         * of a main chunk is `_ENV`, which is how a chunk gets its own globals.
         */
        lua_setupvalue(L: lua_State, funcindex: number, n: number): Uint8Array | null

        /**
         * Frees the state's stack. Nothing may touch the state afterwards.
         */
        lua_close(L: lua_State): void

        /**
         * Both return null when the value has no string form — `lua_tolstring`
         * converts only strings and numbers, so a table error object (from
         * `error({...})`) yields null rather than a description.
         */
        lua_tostring(L: lua_State, idx: number): Uint8Array | null
        lua_tojsstring(L: lua_State, idx: number): string | null

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
        readonly LUA_NOREF: number
        readonly LUA_REFNIL: number

        luaL_newstate(): lua_State
        luaL_requiref(L: lua_State, modname: Uint8Array, openf: LuaOpenFn, glb: number): void
        luaL_loadbuffer(
            L: lua_State,
            buff: Uint8Array,
            name: Uint8Array | null,
            chunkname: Uint8Array,
        ): number
        luaL_error(L: lua_State, fmt: Uint8Array): never

        /** Pops the value at the top and stores it in table `t`, returning its key. */
        luaL_ref(L: lua_State, t: number): number
        luaL_unref(L: lua_State, t: number, ref: number): void

        /**
         * The value at `idx` as a string, converting via `__tostring` when it
         * has one. Pushes the result, which the caller must pop.
         */
        luaL_tolstring(L: lua_State, idx: number): Uint8Array
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

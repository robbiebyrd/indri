// Hand-written declarations for the subset of fengari-interop used by host-api.ts.
// The package ships no TypeScript types of its own.

declare module "fengari-interop" {
    import type {lua_State} from "fengari"

    /**
     * Push a JS value onto the Lua stack, choosing the most natural
     * Lua type (number → number, string → string, object → userdata, …).
     */
    export function push(L: lua_State, v: unknown): void

    /**
     * Convert the Lua value at the given stack index to a JS value.
     * Tables and functions become wrapped proxy objects.
     */
    export function tojs(L: lua_State, idx: number): unknown

    /**
     * Open the `js` library into the given state. Must be called once per
     * state before push/tojs are used.
     */
    export function luaopen_js(L: lua_State): number
}

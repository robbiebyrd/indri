// Spike coverage for the Lua sandbox. Story 019 owns the real runtime; this
// file only proves the state is built the way the sandbox depends on.
import test from "node:test";
import assert from "node:assert/strict";

import {lauxlib, lua, to_jsstring, to_luastring} from "fengari";
import {createSandboxedState} from "./state.ts";

import type {lua_State} from "fengari";

type EvalResult = {ok: true; value: string} | {ok: false; error: string};

/** Minimal load+call used only by this spike. Story 019 replaces it. */
function evalLua(L: lua_State, src: string): EvalResult {
    if (lauxlib.luaL_loadbuffer(L, to_luastring(src), null, to_luastring("=test")) !== lua.LUA_OK) {
        const error = to_jsstring(lua.lua_tostring(L, -1));
        lua.lua_pop(L, 1);
        return {ok: false, error};
    }
    if (lua.lua_pcall(L, 0, 1, 0) !== lua.LUA_OK) {
        const error = to_jsstring(lua.lua_tostring(L, -1));
        lua.lua_pop(L, 1);
        return {ok: false, error};
    }
    const value = to_jsstring(lua.lua_tostring(L, -1));
    lua.lua_pop(L, 1);
    return {ok: true, value};
}

function expectValue(L: lua_State, src: string): string {
    const r = evalLua(L, src);
    assert.ok(r.ok, `expected ${src} to succeed, got: ${r.ok ? "" : r.error}`);
    return r.value;
}

test("a chunk evaluates with only the safe libraries open", () => {
    const L = createSandboxedState();
    assert.equal(expectValue(L, "return 2 + 2"), "4");
    assert.equal(expectValue(L, "return string.rep('ab', 3)"), "ababab");
    assert.equal(expectValue(L, "return table.concat({'a','b'}, '-')"), "a-b");
    assert.equal(expectValue(L, "return math.max(3, 7)"), "7");
});

test("dangerous globals are absent from the sandbox", () => {
    const L = createSandboxedState();
    // io/os/package/debug are never imported, so their libraries are not in the
    // bundle at all. load/loadstring/dofile/loadfile/require come from
    // luaopen_base and are nil'd explicitly.
    for (const name of [
        // never opened
        "io", "os", "package", "debug",
        // installed by luaopen_base, nil'd explicitly
        "load", "loadstring", "dofile", "loadfile", "require", "collectgarbage",
        // bypass metatables, so they would defeat the read-only state view
        "rawset", "rawget", "rawequal", "rawlen",
    ]) {
        assert.equal(expectValue(L, `return tostring(${name})`), "nil", `${name} must not be reachable`);
    }
});

test("a syntax error is returned as a value, not thrown", () => {
    const L = createSandboxedState();
    const r = evalLua(L, "return (");
    assert.equal(r.ok, false);
    assert.match(r.ok ? "" : r.error, /unexpected symbol/);
});

test("a runtime error is returned as a value and carries the Lua message", () => {
    const L = createSandboxedState();
    const r = evalLua(L, "error('boom')");
    assert.equal(r.ok, false);
    assert.match(r.ok ? "" : r.error, /boom/);
});

test("the state still works after a chunk errored", () => {
    const L = createSandboxedState();
    assert.equal(evalLua(L, "error('boom')").ok, false);
    assert.equal(expectValue(L, "return 1 + 1"), "2", "no poisoned state after an error");
});

test("a count hook aborts a runaway loop", () => {
    // Records the verified fengari 0.1.5 hook contract: lua_sethook(L, fn,
    // LUA_MASKCOUNT, n) invokes fn as (L, ar), and raising from it aborts the
    // chunk. Story 019 turns this into a real instruction budget.
    const L = createSandboxedState();
    let fired = 0;
    lua.lua_sethook(L, (hookL) => {
        fired++;
        if (fired > 3) lauxlib.luaL_error(hookL, to_luastring("instruction budget exceeded"));
    }, lua.LUA_MASKCOUNT, 200);

    const r = evalLua(L, "while true do end");
    assert.equal(r.ok, false, "an infinite loop must not hang the test");
    assert.match(r.ok ? "" : r.error, /instruction budget exceeded/);
    assert.ok(fired > 3, "the count hook fired");
});

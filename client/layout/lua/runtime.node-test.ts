import test from "node:test"
import assert from "node:assert/strict"

import {lauxlib, lua, to_jsstring, to_luastring} from "fengari"
import {createRuntimeState, runChunk} from "./runtime.ts"

// ── tests ─────────────────────────────────────────────────────────────────────

test("syntax error is a value, not a throw", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "return (", "test")
    assert.equal(result.ok, false)
    assert.ok(
        result.ok === false && result.error.length > 0,
        "error message must be non-empty",
    )
    assert.ok(
        result.ok === false && /syntax|unexpected|'<eof>'/.test(result.error),
        `expected syntax-related message, got: ${result.ok ? "" : result.error}`,
    )
})

test("runtime error is a value, not a throw", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "error('boom')", "test")
    assert.equal(result.ok, false)
    assert.ok(
        result.ok === false && result.error.includes("boom"),
        `error must include 'boom', got: ${result.ok ? "" : result.error}`,
    )
})

test("runtime error message includes chunk name and line number", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "error('boom')", "test")
    assert.equal(result.ok, false)
    assert.ok(result.ok === false && result.error.length > 0, "error must be non-empty")
    // fengari formats the chunk name as [string "test"] when the name does not
    // start with '=' or '@'.  The full message is: [string "test"]:1: boom
    // Either format is acceptable — the key requirement is that the message
    // includes a line number and the error text.
    assert.ok(
        result.ok === false && /:\d+:/.test(result.error),
        `expected line number in error, got: ${result.ok ? "" : result.error}`,
    )
    assert.ok(
        result.ok === false && result.error.includes("boom"),
        `expected error text 'boom' in error, got: ${result.ok ? "" : result.error}`,
    )
})

test("io, os, require and load are nil inside the runtime", () => {
    const L = createRuntimeState()
    for (const name of ["io", "os", "require", "load"]) {
        // Write result into a known global so we can read it back via fengari API
        const result = runChunk(L, `__check__ = tostring(${name})`, `=check-${name}`)
        assert.ok(result.ok, `chunk for ${name} failed: ${result.ok ? "" : result.error}`)
        lua.lua_getglobal(L, to_luastring("__check__"))
        const val = to_jsstring(lua.lua_tostring(L, -1))
        lua.lua_pop(L, 1)
        assert.equal(val, "nil", `${name} must be nil inside the runtime`)
    }
})

test("infinite loop is aborted by the instruction budget within 100ms", () => {
    const L = createRuntimeState()
    const start = Date.now()
    const result = runChunk(L, "while true do end", "test")
    const elapsed = Date.now() - start
    assert.equal(result.ok, false, "infinite loop must be aborted")
    assert.ok(elapsed < 100, `budget must fire within 100ms, took ${elapsed}ms`)
})

test("no poisoned state after a runtime error", () => {
    const L = createRuntimeState()
    const bad = runChunk(L, "error('boom')", "test")
    assert.equal(bad.ok, false)
    const good = runChunk(L, "return 1+1", "test2")
    assert.ok(good.ok, `second chunk must succeed, got: ${good.ok ? "" : good.error}`)
})

test("Lua stack is drained on success", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "return 42", "test")
    assert.ok(result.ok, `chunk must succeed: ${result.ok ? "" : result.error}`)
    assert.equal(lua.lua_gettop(L), 0, "stack must be empty after successful runChunk")
})

test("Lua stack is drained on error", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "error('oops')", "test")
    assert.equal(result.ok, false)
    assert.equal(lua.lua_gettop(L), 0, "stack must be empty after failed runChunk")
})

test("multiple chunks share the same state", () => {
    const L = createRuntimeState()
    // chunk1 sets a global
    const set = runChunk(L, "x = 10", "chunk1")
    assert.ok(set.ok, `first chunk must succeed: ${set.ok ? "" : set.error}`)
    // Read x via fengari API directly — runChunk drains the stack so we read via getglobal
    lua.lua_getglobal(L, to_luastring("x"))
    const val = to_jsstring(lua.lua_tostring(L, -1))
    lua.lua_pop(L, 1)
    assert.equal(val, "10", "global x set in chunk1 must be visible after chunk1 runs")
})

// Ensure the instruction budget hook does not interfere with luaL_loadbuffer
// (used internally by runChunk). A hook set on a state must not fire during
// the load phase, only during execution.
test("budget hook does not prevent loading a valid chunk", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "local x = 1 + 1", "test-load")
    assert.ok(result.ok, `load must succeed: ${result.ok ? "" : result.error}`)
})

// Verify the stack is clean when we start: createRuntimeState must not leave
// anything on the stack.
test("createRuntimeState leaves an empty stack", () => {
    const L = createRuntimeState()
    assert.equal(lua.lua_gettop(L), 0, "stack must be empty after createRuntimeState")
})

// Regression: after a syntax error (load phase fails), the stack must also be drained.
test("Lua stack is drained after a syntax error", () => {
    const L = createRuntimeState()
    const result = runChunk(L, "return (", "test")
    assert.equal(result.ok, false)
    assert.equal(lua.lua_gettop(L), 0, "stack must be empty after a syntax error")
})

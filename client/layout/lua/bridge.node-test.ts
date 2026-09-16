import test from "node:test"
import assert from "node:assert/strict"

import {LuaSession} from "./host-api.ts"
import {attachBridge, toPlainJson} from "./bridge.ts"

// ---------------------------------------------------------------------------
// 1. indri.send emits the correct wire shape
// ---------------------------------------------------------------------------

test("indri.send emits correct wire shape", () => {
    const session = new LuaSession()
    const captured: object[] = []
    attachBridge(session, (msg) => captured.push(msg))

    const r = session.runScript(`indri.send("move", {r=1, c=2})`, "=test")
    assert.ok(r.ok, `runScript failed: ${r.ok ? "" : r.error}`)

    assert.equal(captured.length, 1)
    const msg = captured[0] as Record<string, unknown>
    assert.equal(msg["action"], "move")
    assert.equal(msg["r"], 1)
    assert.equal(msg["c"], 2)
})

// ---------------------------------------------------------------------------
// 2. Send before socket is open is dropped with a warning
// ---------------------------------------------------------------------------

test("send before socket opens is dropped with a warning, not thrown into Lua", () => {
    const session = new LuaSession()
    const warnings: unknown[] = []
    const origWarn = console.warn
    console.warn = (...args: unknown[]) => warnings.push(args)

    attachBridge(session, (_msg) => {
        throw new Error("socket not open")
    })

    let threw = false
    try {
        // Lua executes indri.send; the sendFn throws; the bridge must catch it
        const r = session.runScript(`indri.send("x", {})`, "=test")
        // runScript itself should succeed (no Lua error propagated)
        assert.ok(r.ok, `unexpected Lua error: ${r.ok ? "" : r.error}`)
    } catch {
        threw = true
    }

    console.warn = origWarn

    assert.equal(threw, false, "the error must not propagate out of Lua")
    assert.ok(warnings.length > 0, "a warning should be logged when sendFn throws")
})

// ---------------------------------------------------------------------------
// 3. No inbound message dispatch to Lua
// ---------------------------------------------------------------------------

test("LuaSession has no parsers field — no inbound message dispatch to Lua", () => {
    const session = new LuaSession()
    assert.equal(
        (session as unknown as Record<string, unknown>)["parsers"],
        undefined,
        "LuaSession must not carry a parsers registry",
    )
})

// ---------------------------------------------------------------------------
// 4. stateChanged fires exactly once per state change
// ---------------------------------------------------------------------------

test("stateChanged fires exactly once per onStateChange call", () => {
    const session = new LuaSession()
    attachBridge(session, () => {/* no send needed */})

    const setup = session.runScript(`
_fireCount = 0
indri.on("stateChanged", function()
  _fireCount = _fireCount + 1
end)
`, "=setup")
    assert.ok(setup.ok, `setup failed: ${setup.ok ? "" : setup.error}`)

    session.onStateChange({x: 1}, "s1")
    session.onStateChange({x: 2}, "s1")

    const check = session.runScript(`return tostring(_fireCount)`, "=check")
    assert.ok(check.ok, `check failed: ${check.ok ? "" : check.error}`)
    assert.equal(check.value, "2", "handler must fire exactly twice")
})

// ---------------------------------------------------------------------------
// 5. stateChanged receives the already-reduced game state
// ---------------------------------------------------------------------------

test("stateChanged fires from the already-reduced game state", () => {
    const session = new LuaSession()
    attachBridge(session, () => {/* no send needed */})

    // Set an initial keyframe
    session.onKeyframe({score: 0}, "s1")

    // Register a handler that reads the current state via indri.state()
    const setup = session.runScript(`
_lastScore = nil
indri.on("stateChanged", function()
  local g = indri.state()
  _lastScore = g.score
end)
`, "=setup")
    assert.ok(setup.ok, `setup failed: ${setup.ok ? "" : setup.error}`)

    // Apply a state change; the handler should see score=5
    session.onStateChange({score: 5}, "s1")

    const check = session.runScript(`return tostring(_lastScore)`, "=check")
    assert.ok(check.ok, `check failed: ${check.ok ? "" : check.error}`)
    // fengari represents JS Numbers as Lua floats; tostring(5) yields "5.0"
    assert.ok(
        check.value === "5" || check.value === "5.0",
        `stateChanged handler must see the new score, got: ${check.value}`,
    )
})

// ---------------------------------------------------------------------------
// 6. Framework reserves no action names
// ---------------------------------------------------------------------------

test("any action name works without error", () => {
    const session = new LuaSession()
    const captured: object[] = []
    attachBridge(session, (msg) => captured.push(msg))

    const r = session.runScript(`indri.send("my_custom_action", {})`, "=test")
    assert.ok(r.ok, `runScript failed: ${r.ok ? "" : r.error}`)
    assert.equal(captured.length, 1)
    assert.equal((captured[0] as Record<string, unknown>)["action"], "my_custom_action")
})

// ---------------------------------------------------------------------------
// 7. Cyclic payload rejected
// ---------------------------------------------------------------------------

test("toPlainJson rejects cyclic objects", () => {
    const obj: Record<string, unknown> = {a: 1}
    obj["self"] = obj  // cyclic

    let threw = false
    let result: unknown = "NOT_SET"
    try {
        result = toPlainJson(obj)
    } catch {
        threw = true
    }
    // Either it throws OR returns null — both are acceptable
    assert.ok(threw || result === null, "cyclic object must throw or return null")
})

// ---------------------------------------------------------------------------
// 8. Deeply nested payload rejected
// ---------------------------------------------------------------------------

test("toPlainJson rejects objects nested deeper than 8 levels", () => {
    // Build a 9-level deep object
    let deep: unknown = {value: "leaf"}
    for (let i = 0; i < 9; i++) {
        deep = {nested: deep}
    }

    let threw = false
    let result: unknown = "NOT_SET"
    try {
        result = toPlainJson(deep)
    } catch {
        threw = true
    }
    assert.ok(threw || result === null, "deeply nested object must throw or return null")
})

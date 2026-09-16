import test from "node:test"
import assert from "node:assert/strict"

import {LuaSession} from "./host-api.ts"

// ---------------------------------------------------------------------------
// Test 1: Override writes to override map, not game state
// ---------------------------------------------------------------------------
test("widget setStyle writes to override map, not game state", () => {
    const session = new LuaSession()
    const game = {x: 1}
    session.onKeyframe(game, "scene1")

    session.widget("x").setStyle({backgroundColor: "#f00"})

    assert.equal(session.overrides.widgets["x"]?.style?.backgroundColor, "#f00")
    // original game object is unchanged
    assert.deepEqual(session.currentGame, {x: 1})
})

// ---------------------------------------------------------------------------
// Test 2: Keyframe clears all overrides
// ---------------------------------------------------------------------------
test("onKeyframe clears all overrides", () => {
    const session = new LuaSession()
    const game = {x: 1}
    session.onKeyframe(game, "scene1")
    session.widget("w").setStyle({backgroundColor: "#0f0"})
    assert.ok(Object.keys(session.overrides.widgets).length > 0, "override should exist before keyframe")

    session.onKeyframe(game, "scene1")

    assert.equal(Object.keys(session.overrides.widgets).length, 0, "widgets should be empty after keyframe")
    assert.equal(session.overrides.board, undefined)
    assert.equal(session.overrides.scene, undefined)
})

// ---------------------------------------------------------------------------
// Test 3: Delta does NOT clear overrides
// ---------------------------------------------------------------------------
test("onStateChange does not clear overrides", () => {
    const session = new LuaSession()
    const game = {x: 1}
    session.onKeyframe(game, "scene1")
    session.widget("w").setStyle({backgroundColor: "#00f"})

    session.onStateChange(game, "scene1")

    assert.equal(session.overrides.widgets["w"]?.style?.backgroundColor, "#00f",
        "override must survive a delta update")
})

// ---------------------------------------------------------------------------
// Test 4: Bubbling widget → scene → game
// ---------------------------------------------------------------------------
test("event bubbling fires widget then scene then game", () => {
    const session = new LuaSession()
    session.onKeyframe({}, "scene1")

    const order: string[] = []
    session.events.on("btn", "widgetPress", () => { order.push("widget") })
    session.events.on("scene", "widgetPress", () => { order.push("scene") })
    session.events.on("game", "widgetPress", () => { order.push("game") })

    session.emit("widgetPress", "btn")

    assert.deepEqual(order, ["widget", "scene", "game"])
})

// ---------------------------------------------------------------------------
// Test 5: Bubbling stops when handler returns false
// ---------------------------------------------------------------------------
test("bubbling stops when a handler returns false", () => {
    const session = new LuaSession()
    session.onKeyframe({}, "scene1")

    const order: string[] = []
    session.events.on("btn", "widgetPress", () => { order.push("widget"); return false })
    session.events.on("scene", "widgetPress", () => { order.push("scene") })
    session.events.on("game", "widgetPress", () => { order.push("game") })

    session.emit("widgetPress", "btn")

    assert.deepEqual(order, ["widget"], "scene and game must not fire when widget returns false")
})

// ---------------------------------------------------------------------------
// Test 6: Handler for removed widget is dropped
// ---------------------------------------------------------------------------
test("dropWidget removes all handlers for that widget", () => {
    const session = new LuaSession()
    session.onKeyframe({}, "scene1")

    let called = false
    session.events.on("w1", "stateChanged", () => { called = true })
    session.events.dropWidget("w1")

    session.emit("stateChanged", "w1")

    assert.equal(called, false, "dropped widget handler must not fire")
})

// ---------------------------------------------------------------------------
// Test 7: Re-entrancy — a host callback from inside a Lua handler does not
//         start a second dispatch
// ---------------------------------------------------------------------------
test("re-entrancy: nested emit inside a handler does not recurse infinitely", () => {
    const session = new LuaSession()
    session.onKeyframe({}, "scene1")

    let outerCount = 0
    let innerCount = 0

    session.events.on("game", "stateChanged", () => {
        outerCount++
        // Calling emit while already dispatching — must be a no-op
        session.emit("widgetPress", null)
    })
    session.events.on("game", "widgetPress", () => {
        innerCount++
    })

    session.emit("stateChanged", null)

    assert.equal(outerCount, 1, "outer handler fires once")
    assert.equal(innerCount, 0, "inner emit is suppressed during outer dispatch")
})

// ---------------------------------------------------------------------------
// Test 8: Scripts are NOT re-evaluated on delta
// ---------------------------------------------------------------------------
test("onStateChange fires stateChanged but does not re-run the script chunk", () => {
    const session = new LuaSession()
    session.installHostApi(() => {})
    session.onKeyframe({counter: 0}, "scene1")

    // Run a chunk that increments a global counter
    const r = session.runScript("_counter = (_counter or 0) + 1", "=test")
    assert.ok(r.ok, `chunk failed: ${r.ok ? "" : r.error}`)

    // Fire two deltas
    session.onStateChange({counter: 1}, "scene1")
    session.onStateChange({counter: 2}, "scene1")

    // The chunk ran exactly once; stateChanged callbacks fire but don't re-run the chunk
    const r2 = session.runScript("return _counter", "=check")
    assert.ok(r2.ok)
    // Lua tostring returns strings; "1" confirms the chunk ran exactly once
    assert.equal(r2.ok ? r2.value : null, "1", "counter stays 1 — chunk ran once, not on each delta")
})

// ---------------------------------------------------------------------------
// Test 9: state() returns a frozen snapshot
// ---------------------------------------------------------------------------
test("state() returns a deep-frozen snapshot", () => {
    const session = new LuaSession()
    const game = {x: 1, nested: {y: 2}}
    session.onKeyframe(game, "s")

    const snap = session.state()

    assert.ok(Object.isFrozen(snap), "snapshot must be frozen")
    // Mutating it should not change currentGame
    try { (snap as {x: number}).x = 99 } catch (_) { /* strict mode may throw */ }
    assert.equal((session.currentGame as {x: number}).x, 1, "currentGame must be unaffected by mutation attempt")
})

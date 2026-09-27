import test from "node:test"
import assert from "node:assert/strict"

import type {PlacedWidget} from "../grid/coords.ts"
import type {EditContext, Sender} from "./ops.ts"
import {
    moveWidget,
    addWidget,
    removeWidget,
    setWidgetConfig,
    setStyle,
    setGrid,
    setScript,
} from "./ops.ts"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSender(): Sender & {sent: object[]} {
    const sent: object[] = []
    return {
        send(msg: object) { sent.push(msg) },
        sent,
    }
}

function makeCtx(overrides: Partial<EditContext> = {}): EditContext {
    return {
        gameCode: "GAME1",
        sceneId: "scene1",
        siblings: [],
        grid: {cols: 10, rows: 10},
        ...overrides,
    }
}

function makeGridWidget(id: string, col: number, row: number, w: number, h: number): PlacedWidget {
    return {id, placement: {kind: "grid", col, row, w, h}}
}

function makeAbsoluteWidget(id: string): PlacedWidget {
    return {id, placement: {kind: "absolute"}}
}

// ---------------------------------------------------------------------------
// moveWidget
// ---------------------------------------------------------------------------

test("moveWidget: emits setPlacement with correct wire shape", () => {
    const ws = makeSender()
    const ctx = makeCtx()
    const result = moveWidget(ws, ctx, "w1", {col: 0, row: 0, w: 2, h: 2})
    assert.equal(result, true)
    assert.equal(ws.sent.length, 1)
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setPlacement",
        code: "GAME1",
        sceneId: "scene1",
        widgetId: "w1",
        placement: {kind: "grid", col: 0, row: 0, w: 2, h: 2},
    })
})

test("moveWidget: returns false and sends nothing when canPlace fails", () => {
    const ws = makeSender()
    const siblings: PlacedWidget[] = [makeGridWidget("w2", 0, 0, 3, 3)]
    const ctx = makeCtx({siblings})
    // w1 at (0,0,2,2) overlaps w2 at (0,0,3,3)
    const result = moveWidget(ws, ctx, "w1", {col: 0, row: 0, w: 2, h: 2})
    assert.equal(result, false)
    assert.equal(ws.sent.length, 0)
})

test("moveWidget: sends when target position overlaps only an absolute widget", () => {
    const ws = makeSender()
    // Absolute sibling at same position — should NOT block a grid move
    const siblings: PlacedWidget[] = [makeAbsoluteWidget("abs1")]
    const ctx = makeCtx({siblings})
    const result = moveWidget(ws, ctx, "w1", {col: 0, row: 0, w: 2, h: 2})
    assert.equal(result, true)
    assert.equal(ws.sent.length, 1)
})

test("moveWidget: widget moving to its own position does not self-collide", () => {
    const ws = makeSender()
    const siblings: PlacedWidget[] = [makeGridWidget("w1", 0, 0, 2, 2)]
    const ctx = makeCtx({siblings})
    // Moving w1 to exactly its current position
    const result = moveWidget(ws, ctx, "w1", {col: 0, row: 0, w: 2, h: 2})
    assert.equal(result, true)
    assert.equal(ws.sent.length, 1)
})

// ---------------------------------------------------------------------------
// addWidget
// ---------------------------------------------------------------------------

test("addWidget: emits addWidget with correct wire shape", () => {
    const ws = makeSender()
    const ctx = makeCtx()
    const widget = {type: "text", placement: {kind: "grid", col: 0, row: 0, w: 2, h: 2}}
    addWidget(ws, ctx, "w1", widget)
    assert.equal(ws.sent.length, 1)
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "addWidget",
        code: "GAME1",
        sceneId: "scene1",
        widgetId: "w1",
        widget,
    })
})

// ---------------------------------------------------------------------------
// removeWidget
// ---------------------------------------------------------------------------

test("removeWidget: emits removeWidget with correct wire shape", () => {
    const ws = makeSender()
    const ctx = makeCtx()
    removeWidget(ws, ctx, "w1")
    assert.equal(ws.sent.length, 1)
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "removeWidget",
        code: "GAME1",
        sceneId: "scene1",
        widgetId: "w1",
    })
})

// ---------------------------------------------------------------------------
// setWidgetConfig
// ---------------------------------------------------------------------------

test("setWidgetConfig: emits setWidgetConfig with correct wire shape", () => {
    const ws = makeSender()
    const ctx = makeCtx()
    setWidgetConfig(ws, ctx, "w1", {color: "red", fontSize: 14})
    assert.equal(ws.sent.length, 1)
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setWidgetConfig",
        code: "GAME1",
        sceneId: "scene1",
        widgetId: "w1",
        config: {color: "red", fontSize: 14},
    })
})

// ---------------------------------------------------------------------------
// setStyle
// ---------------------------------------------------------------------------

test("setStyle: board scope emits correct wire shape", () => {
    const ws = makeSender()
    setStyle(ws, "GAME1", {scope: "board"}, {background: "#fff"})
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setStyle",
        code: "GAME1",
        scope: "board",
        style: {background: "#fff"},
    })
})

test("setStyle: scene scope includes sceneId", () => {
    const ws = makeSender()
    setStyle(ws, "GAME1", {scope: "scene", sceneId: "s1"}, {color: "blue"})
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setStyle",
        code: "GAME1",
        scope: "scene",
        sceneId: "s1",
        style: {color: "blue"},
    })
})

test("setStyle: widget scope includes sceneId and widgetId", () => {
    const ws = makeSender()
    setStyle(ws, "GAME1", {scope: "widget", sceneId: "s1", widgetId: "w1"}, {opacity: 0.5})
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setStyle",
        code: "GAME1",
        scope: "widget",
        sceneId: "s1",
        widgetId: "w1",
        style: {opacity: 0.5},
    })
})

// ---------------------------------------------------------------------------
// setGrid
// ---------------------------------------------------------------------------

test("setGrid: emits setGrid with correct wire shape", () => {
    const ws = makeSender()
    setGrid(ws, "GAME1", {cols: 12, rows: 8})
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setGrid",
        code: "GAME1",
        grid: {cols: 12, rows: 8},
    })
})

// ---------------------------------------------------------------------------
// setScript
// ---------------------------------------------------------------------------

test("setScript: board scope emits correct wire shape", () => {
    const ws = makeSender()
    setScript(ws, "GAME1", {scope: "board"}, "return true")
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setScript",
        code: "GAME1",
        scope: "board",
        source: "return true",
    })
})

test("setScript: scene scope includes sceneId", () => {
    const ws = makeSender()
    setScript(ws, "GAME1", {scope: "scene", sceneId: "s1"}, "return false")
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setScript",
        code: "GAME1",
        scope: "scene",
        sceneId: "s1",
        source: "return false",
    })
})

test("setScript: widget scope includes sceneId and widgetId", () => {
    const ws = makeSender()
    setScript(ws, "GAME1", {scope: "widget", sceneId: "s1", widgetId: "w1"}, "return nil")
    assert.deepEqual(ws.sent[0], {
        action: "layout",
        op: "setScript",
        code: "GAME1",
        scope: "widget",
        sceneId: "s1",
        widgetId: "w1",
        source: "return nil",
    })
})

// ---------------------------------------------------------------------------
// Sender.send drop-before-open (exercised via fake that mimics MessageHandler)
// ---------------------------------------------------------------------------

test("no message is sent when the sender drops it (simulates pre-open socket)", () => {
    // Simulate a sender whose socket is not yet open (like MessageHandler before OPEN state).
    const droppingSender: Sender & {sent: object[]} = {
        sent: [],
        send(_msg: object) {
            // Drop silently — mirrors MessageHandler.send() when readyState !== OPEN
        },
    }
    const ctx = makeCtx()
    const result = moveWidget(droppingSender, ctx, "w1", {col: 0, row: 0, w: 2, h: 2})
    // moveWidget returns true (canPlace passed) but the underlying sender dropped it
    assert.equal(result, true)
    assert.equal(droppingSender.sent.length, 0)
})

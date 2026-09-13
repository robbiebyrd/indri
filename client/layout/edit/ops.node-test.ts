import test from "node:test";
import assert from "node:assert/strict";

import {
    addWidget,
    newWidgetId,
    removeWidget,
    setAbsolutePlacement,
    setGrid,
    setScript,
    setStyle,
    setPlacement,
    setWidgetConfig,
} from "./ops.ts";

import type {EditContext, LayoutSocket} from "./ops.ts";
import type {PlacedWidget} from "../grid/collision.ts";
import type {Widget} from "../schema/widget.ts";

/** Captures what would go on the wire. */
function fakeSocket(): LayoutSocket & {sent: Record<string, unknown>[]} {
    const sent: Record<string, unknown>[] = [];
    return {sent, send: (m) => void sent.push(m as Record<string, unknown>)};
}

const OCCUPIED: PlacedWidget[] = [{id: "taken", rect: {col: 0, row: 0, w: 3, h: 3}}];

function ctx(siblings: PlacedWidget[] = OCCUPIED): EditContext {
    return {gameCode: "ABCD", sceneId: "board", grid: {cols: 12, rows: 12}, siblings};
}

const widget: Widget = {
    type: "text",
    placement: {kind: "grid", col: 5, row: 5, w: 1, h: 1},
    config: {text: "hi"},
};

// --- wire shape -------------------------------------------------------------

// Field names and op values mirror internal/handlers/actions/layout/op.go, and
// its decoder rejects unknown keys PER OP — so an extra or misspelled field is
// a rejected edit, not something ignored. These assertions are deepStrictEqual
// for that reason.
test("every builder emits exactly the wire shape the Go decoder expects", () => {
    const ws = fakeSocket();
    const c = ctx();

    addWidget(ws, c, "score", widget);
    removeWidget(ws, c, "score");
    setWidgetConfig(ws, c, "score", {text: "X"});
    setGrid(ws, c, {cols: 8, rows: 8});
    setStyle(ws, c, "board", {backgroundColor: "#000"});
    setStyle(ws, c, "scene", {padding: 4});
    setStyle(ws, c, "widget", {opacity: 0.5}, "score");
    setScript(ws, c, "scene", "-- lua");

    assert.deepStrictEqual(ws.sent[0], {
        action: "layout", op: "addWidget", code: "ABCD",
        sceneId: "board", widgetId: "score", widget,
    });
    assert.deepStrictEqual(ws.sent[1], {
        action: "layout", op: "removeWidget", code: "ABCD", sceneId: "board", widgetId: "score",
    });
    assert.deepStrictEqual(ws.sent[2], {
        action: "layout", op: "setWidgetConfig", code: "ABCD",
        sceneId: "board", widgetId: "score", config: {text: "X"},
    });
    assert.deepStrictEqual(ws.sent[3], {
        action: "layout", op: "setGrid", code: "ABCD", grid: {cols: 8, rows: 8},
    });

    // board carries NEITHER id: the Go decoder rejects a board-scoped op that
    // includes an address, so sending one would fail every board edit.
    assert.deepStrictEqual(ws.sent[4], {
        action: "layout", op: "setStyle", code: "ABCD", scope: "board", style: {backgroundColor: "#000"},
    });
    assert.deepStrictEqual(ws.sent[5], {
        action: "layout", op: "setStyle", code: "ABCD", scope: "scene",
        style: {padding: 4}, sceneId: "board",
    });
    assert.deepStrictEqual(ws.sent[6], {
        action: "layout", op: "setStyle", code: "ABCD", scope: "widget",
        style: {opacity: 0.5}, sceneId: "board", widgetId: "score",
    });
    assert.deepStrictEqual(ws.sent[7], {
        action: "layout", op: "setScript", code: "ABCD", scope: "scene",
        source: "-- lua", sceneId: "board",
    });
});

test("an empty script source is sent, because it is how a script is cleared", () => {
    const ws = fakeSocket();
    setScript(ws, ctx(), "widget", "", "score");
    assert.equal((ws.sent[0] as {source: string}).source, "");
});

test("a widget-scoped op without a widgetId is a programming error, not a silent send", () => {
    const ws = fakeSocket();
    assert.throws(() => setStyle(ws, ctx(), "widget", {opacity: 1}));
    assert.equal(ws.sent.length, 0);
});

// --- local collision check --------------------------------------------------

test("a move that collides is not sent", () => {
    const ws = fakeSocket();
    // Overlaps "taken" at (0,0,3,3).
    assert.equal(setPlacement(ws, ctx(), "mover", {col: 1, row: 1, w: 2, h: 2}), false);
    assert.equal(ws.sent.length, 0, "a doomed round trip must be avoided");
});

test("a move to a free slot is sent as a whole placement", () => {
    const ws = fakeSocket();
    assert.equal(setPlacement(ws, ctx(), "mover", {col: 6, row: 6, w: 2, h: 2}), true);
    assert.deepStrictEqual(ws.sent[0], {
        action: "layout", op: "setPlacement", code: "ABCD", sceneId: "board", widgetId: "mover",
        placement: {kind: "grid", col: 6, row: 6, w: 2, h: 2},
    });
});

test("a widget does not collide with its own current position", () => {
    const ws = fakeSocket();
    const c = ctx([{id: "mover", rect: {col: 0, row: 0, w: 3, h: 3}}]);
    // Overlaps where "mover" currently is, which is exactly what a drag does.
    assert.equal(setPlacement(ws, c, "mover", {col: 1, row: 1, w: 3, h: 3}), true);
});

test("an out-of-bounds move is not sent", () => {
    const ws = fakeSocket();
    assert.equal(setPlacement(ws, ctx([]), "mover", {col: 11, row: 11, w: 4, h: 4}), false);
    assert.equal(ws.sent.length, 0);
});

// Absolute widgets are exempt from collision as BOTH subject and obstacle. The
// exemption is expressed by the caller omitting them from `siblings`, so a move
// onto the space one occupies is legal and must still be sent.
test("a move onto an absolute widget is sent, because absolutes never block", () => {
    const ws = fakeSocket();
    // An absolute widget visually covering (0,0)-(3,3) contributes no sibling.
    assert.equal(setPlacement(ws, ctx([]), "mover", {col: 0, row: 0, w: 3, h: 3}), true);
    assert.equal(ws.sent.length, 1);
});

test("an absolute placement always sends and is passed through unchanged", () => {
    const ws = fakeSocket();
    const placement = {kind: "absolute", left: "10%", top: "20%", width: "30%", height: "40%"} as const;
    setAbsolutePlacement(ws, ctx(), "badge", placement);
    assert.deepStrictEqual(ws.sent[0], {
        action: "layout", op: "setPlacement", code: "ABCD",
        sceneId: "board", widgetId: "badge", placement,
    });
});

// --- no optimistic application ---------------------------------------------

test("builders only send; they never mutate the context they were given", () => {
    const c = ctx();
    const before = JSON.stringify(c);
    const ws = fakeSocket();

    addWidget(ws, c, "a", widget);
    setPlacement(ws, c, "mover", {col: 6, row: 6, w: 1, h: 1});
    removeWidget(ws, c, "a");

    assert.equal(JSON.stringify(c), before, "the delta is the only confirmation an edit happened");
});

// --- id generation ----------------------------------------------------------

test("a generated widget id never collides with one already present", () => {
    assert.equal(newWidgetId("text", []), "text1");
    assert.equal(newWidgetId("text", ["text1", "text2"]), "text3");
    assert.equal(newWidgetId("subgrid", ["subgrid1"]), "subgrid2");
});

test("a generated id is short and safe for a delta path", () => {
    // Ids appear in dotted delta paths, so they stay alphanumeric and short
    // rather than being UUIDs.
    const id = newWidgetId("Some Weird/Type!", []);
    assert.match(id, /^[a-z0-9]+$/);
    assert.ok(id.length <= 12, `id ${id} is too long for a readable delta path`);
});

// Positioning, overlap permission and z-order are independent decisions. A
// widget that allows overlap is exempt as BOTH subject and obstacle — the
// asymmetric case is what makes "turn the flag on and the resize goes through"
// actually true.
test("a widget that allows overlap is not blocked by the local check", () => {
    const ws = fakeSocket();
    // Would collide with "taken" at (0,0,3,3).
    const sent = setPlacement(ws, ctx(), "mover", {col: 1, row: 1, w: 3, h: 3, overlap: true});
    assert.equal(sent, true, "the flag must let the move through");
    assert.deepStrictEqual((ws.sent[0] as {placement: unknown}).placement, {
        kind: "grid", col: 1, row: 1, w: 3, h: 3, overlap: true,
    });
});

test("a widget that allows overlap does not block another", () => {
    const ws = fakeSocket();
    // The exemption is expressed by the caller leaving it out of `siblings`,
    // so an overlapping neighbour simply is not there to collide with.
    assert.equal(setPlacement(ws, ctx([]), "mover", {col: 0, row: 0, w: 3, h: 3}), true);
});

test("z rides along with a grid placement", () => {
    const ws = fakeSocket();
    setPlacement(ws, ctx(), "mover", {col: 6, row: 6, w: 1, h: 1, z: 5});
    assert.deepStrictEqual((ws.sent[0] as {placement: unknown}).placement, {
        kind: "grid", col: 6, row: 6, w: 1, h: 1, z: 5,
    });
});

/**
 * Placement of a newly created widget, exercised in bare Node.
 *
 * The three `definition.ts` modules are imported, never the `.tsx` components
 * beside them (see `components/board/widgets/widgets.node-test.ts`), and
 * `place.ts` itself pulls in no react-native — which is what lets the palette's
 * entire decision-making run under `node --experimental-strip-types`.
 */

import assert from "node:assert/strict";
import test from "node:test";

import {canPlace} from "../grid/collision.ts";
import {Placement} from "../schema/placement.ts";
import {WidgetSchema} from "../schema/widget.ts";
import {imageDefinition} from "../../components/board/widgets/image/definition.ts";
import {subGridDefinition} from "../../components/board/widgets/subgrid/definition.ts";
import {textDefinition} from "../../components/board/widgets/text/definition.ts";
import {
    createWidget,
    firstFree,
    MAX_SEARCH_SPAN,
    newWidget,
    NEW_WIDGET_SIZE,
    placementFor,
} from "./place.ts";

import type {PlacedWidget} from "../grid/collision.ts";
import type {GridRect, GridSize} from "../grid/coords.ts";
import type {WidgetDefinition} from "../registry/registry.ts";
import type {Widget} from "../schema/widget.ts";
import type {EditContext, LayoutSocket} from "./ops.ts";

/** Captures what would go on the wire. */
function fakeSocket(): LayoutSocket & {sent: Record<string, unknown>[]} {
    const sent: Record<string, unknown>[] = [];
    return {sent, send: (m) => void sent.push(m as Record<string, unknown>)};
}

function ctx(siblings: PlacedWidget[], grid: GridSize = {cols: 12, rows: 12}): EditContext {
    return {gameCode: "ABCD", sceneId: "board", grid, siblings};
}

function placed(rects: GridRect[]): PlacedWidget[] {
    return rects.map((rect, i) => ({id: `w${i}`, rect}));
}

// The renderer-free halves of exactly the definitions `components/board/widgets`
// registers. The cast is how the erased registry type is reached from a
// concrete `WidgetDefinition<C>`; `registerWidget` does the same thing.
const DEFINITIONS: WidgetDefinition[] = [
    textDefinition as WidgetDefinition,
    imageDefinition as WidgetDefinition,
    subGridDefinition as WidgetDefinition,
];

// --- first fit ---------------------------------------------------------------

test("firstFree takes the first free rect scanning row-major", () => {
    // Row 0 is full, and row 1 is occupied up to column 3. Scanning row-major,
    // the first 2x1 that fits is therefore (col 3, row 1) — a column-major scan
    // would answer (col 3, row 1) too only by accident, so row 0 is deliberately
    // blocked whole to make the row step observable.
    const sibs = placed([{col: 0, row: 0, w: 12, h: 1}, {col: 0, row: 1, w: 3, h: 1}]);

    assert.deepStrictEqual(firstFree({w: 2, h: 1}, sibs, {cols: 12, rows: 12}), {
        col: 3, row: 1, w: 2, h: 1,
    });
});

test("firstFree starts at the origin when nothing is placed", () => {
    assert.deepStrictEqual(firstFree({w: 3, h: 2}, [], {cols: 12, rows: 12}), {
        col: 0, row: 0, w: 3, h: 2,
    });
});

test("firstFree steps past a widget it would overlap rather than around its origin", () => {
    // A 3x3 at the origin blocks columns 0-2 of rows 0-2, so a 2x2 cannot start
    // at column 1 or 2 either: the first fit is column 3, not column 1.
    const sibs = placed([{col: 0, row: 0, w: 3, h: 3}]);

    assert.deepStrictEqual(firstFree({w: 2, h: 2}, sibs, {cols: 12, rows: 12}), {
        col: 3, row: 0, w: 2, h: 2,
    });
});

test("a full grid yields no placement rather than an overlapping one", () => {
    const sibs = placed([{col: 0, row: 0, w: 8, h: 8}]);

    assert.equal(firstFree({w: 1, h: 1}, sibs, {cols: 8, rows: 8}), undefined);
});

test("a widget too large for the grid yields no placement", () => {
    assert.equal(firstFree({w: 9, h: 1}, [], {cols: 8, rows: 8}), undefined);
});

// --- the bounded search window ----------------------------------------------

// A naive row-major scan of a maximal grid is 4096 x 4096 = 16.7 MILLION
// candidate positions, each O(siblings). This is the test that says the bound
// exists: the siblings below block every row of the window but the last, which
// is the worst case the bound permits — a full 64 x 64 sweep of origins.
test("a 4096x4096 grid is answered promptly, from inside the bounded window", () => {
    const sibs = placed([{col: 0, row: 0, w: 4096, h: 63}]);
    const grid = {cols: 4096, rows: 4096};

    const started = performance.now();
    const rect = firstFree(NEW_WIDGET_SIZE, sibs, grid);
    const elapsed = performance.now() - started;

    assert.deepStrictEqual(rect, {col: 0, row: 63, ...NEW_WIDGET_SIZE});
    assert.ok(
        elapsed < 100,
        `a bounded first-fit search took ${elapsed}ms; the 64x64 origin cap is not being applied`,
    );
});

test("free space beyond the window is not searched for, it is reported as none", () => {
    // 256x256 with the whole 64x64 origin window occupied. Column 64 onwards is
    // wide open, and finding it is exactly what the bound gives up: the caller
    // falls back to absolute placement instead of sweeping 65,536 positions.
    const grid = {cols: 256, rows: 256};
    const sibs = placed([{col: 0, row: 0, w: MAX_SEARCH_SPAN, h: MAX_SEARCH_SPAN}]);

    assert.ok(
        canPlace({col: MAX_SEARCH_SPAN, row: 0, w: 1, h: 1}, "", sibs, grid),
        "the fixture is wrong: there must be free space outside the window",
    );
    assert.equal(firstFree({w: 1, h: 1}, sibs, grid), undefined);
});

test("a widget wider than the window still places, because only the origin is bounded", () => {
    // The cap is on candidate ORIGINS, not on the rect. A 100-cell-wide widget
    // has an origin inside the window even though it extends well past it.
    const rect = firstFree({w: 100, h: 100}, [], {cols: 256, rows: 256});

    assert.deepStrictEqual(rect, {col: 0, row: 0, w: 100, h: 100});
});

// --- firstFree agrees with the collision module ------------------------------

/** Deterministic pseudo-randomness, so a failure is reproducible. */
function lcg(seed: number): () => number {
    let state = seed >>> 0;

    return () => {
        state = (Math.imul(state, 1664525) + 1013904223) >>> 0;

        return state / 0x100000000;
    };
}

function randomLayout(rand: () => number, g: GridSize, count: number): PlacedWidget[] {
    const rects: GridRect[] = [];
    for (let i = 0; i < count; i++) {
        const w = 1 + Math.floor(rand() * 3);
        const h = 1 + Math.floor(rand() * 3);
        rects.push({
            col: Math.floor(rand() * (g.cols - w + 1)),
            row: Math.floor(rand() * (g.rows - h + 1)),
            w,
            h,
        });
    }

    return placed(rects);
}

// firstFree does its own loop bounds and then asks canPlace, so the two could
// disagree — an off-by-one in the loop would produce a rect that collides or
// hangs off the edge, which the server would reject on arrival.
test("firstFree never returns a rect canPlace would reject", () => {
    const rand = lcg(20260911);

    for (const g of [{cols: 8, rows: 8}, {cols: 12, rows: 12}, {cols: 37, rows: 21}]) {
        for (let trial = 0; trial < 200; trial++) {
            const sibs = randomLayout(rand, g, 1 + Math.floor(rand() * 12));
            const size = {w: 1 + Math.floor(rand() * 4), h: 1 + Math.floor(rand() * 4)};
            const rect = firstFree(size, sibs, g);
            if (rect === undefined) continue;

            assert.deepStrictEqual({w: rect.w, h: rect.h}, size, "the size must come back intact");
            assert.ok(
                canPlace(rect, "", sibs, g),
                `firstFree returned ${JSON.stringify(rect)} that canPlace rejects`,
            );
        }
    }
});

// Inside the window the answer must be the EXACT first fit, not merely a legal
// one: "no placement" has to mean no origin fits, or a full grid and a merely
// awkward one would be indistinguishable.
test("on a grid inside the window firstFree matches an exhaustive row-major scan", () => {
    const rand = lcg(7654321);
    const g = {cols: 12, rows: 12};

    for (let trial = 0; trial < 300; trial++) {
        const sibs = randomLayout(rand, g, 1 + Math.floor(rand() * 14));
        const size = {w: 1 + Math.floor(rand() * 3), h: 1 + Math.floor(rand() * 3)};

        let expected: GridRect | undefined;
        for (let row = 0; expected === undefined && row + size.h <= g.rows; row++) {
            for (let col = 0; col + size.w <= g.cols; col++) {
                const rect = {col, row, ...size};
                if (canPlace(rect, "", sibs, g)) {
                    expected = rect;
                    break;
                }
            }
        }

        assert.deepStrictEqual(firstFree(size, sibs, g), expected);
    }
});

// --- the absolute fallback ---------------------------------------------------

test("with no free slot the placement falls back to absolute at the origin", () => {
    const full = placed([{col: 0, row: 0, w: 8, h: 8}]);
    const placement = placementFor({w: 2, h: 2}, full, {cols: 8, rows: 8});

    // Percentages, never pixels — and parsed, because the wire schema is the
    // authority on what an absolute placement may contain.
    assert.deepStrictEqual(Placement.parse(placement), {
        kind: "absolute", left: "0%", top: "0%", width: "25%", height: "25%",
    });
});

test("a free slot is a grid placement, so the new widget joins the grid", () => {
    assert.deepStrictEqual(placementFor({w: 2, h: 2}, [], {cols: 8, rows: 8}), {
        kind: "grid", col: 0, row: 0, w: 2, h: 2,
    });
});

// --- what the palette emits --------------------------------------------------

// The payload is built from the registry's own `defaults`, so a widget type
// whose defaults drifted from its schema would create widgets the server
// accepts and the renderer then refuses to draw.
test("every registered widget's defaults produce a payload its own schema accepts", () => {
    for (const def of DEFINITIONS) {
        const ws = fakeSocket();
        const id = createWidget(ws, ctx([]), def, []);

        assert.equal(ws.sent.length, 1, `${def.type}: exactly one op per palette selection`);
        const sent = ws.sent[0] as {action: string; op: string; widgetId: string; widget: Widget};

        assert.equal(sent.action, "layout");
        assert.equal(sent.op, "addWidget");
        assert.equal(sent.widgetId, id);

        // Structure first: `WidgetSchema` is strict, so an extra key here would
        // be a rejected edit rather than something ignored.
        const widget = WidgetSchema.parse(sent.widget);
        assert.equal(widget.type, def.type);
        assert.deepStrictEqual(widget.placement, {kind: "grid", col: 0, row: 0, ...NEW_WIDGET_SIZE});

        // Then meaning: the widget's OWN schema on the config that was sent.
        const config = def.schema.safeParse(widget.config);
        assert.ok(
            config.success,
            `${def.type}: emitted config rejected by its own schema: ${JSON.stringify(config.error?.issues)}`,
        );
        assert.deepStrictEqual(widget.config, def.defaults);
    }
});

test("the emitted config is a copy, so the registry's defaults cannot be edited through it", () => {
    const def = DEFINITIONS[0];
    const widget = newWidget(def, ctx([]));

    assert.notEqual(widget.config, def.defaults, "sending the registry's own object invites mutation");
});

// `ctx.siblings` omits absolutely placed widgets by design, so it is NOT the id
// list — generating an id from it alone would collide with an absolute widget,
// and the server rejects `addWidget` on an existing id rather than replacing.
test("a new id avoids every id at the level, including ones with no grid rect", () => {
    const ws = fakeSocket();
    const taken = ["text1", "text2"];

    assert.equal(createWidget(ws, ctx([]), DEFINITIONS[0], taken), "text3");
});

test("a palette selection with nowhere to go still creates the widget, absolutely placed", () => {
    const ws = fakeSocket();
    const full = placed([{col: 0, row: 0, w: 8, h: 8}]);

    createWidget(ws, ctx(full, {cols: 8, rows: 8}), DEFINITIONS[0], []);

    const sent = ws.sent[0] as {widget: Widget};
    assert.equal(WidgetSchema.parse(sent.widget).placement.kind, "absolute");
});

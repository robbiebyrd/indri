import assert from "node:assert/strict";
import {test} from "node:test";

import {placementBox, placementZIndex} from "./placement.ts";

import type {GridSize} from "../../layout/grid/coords.ts";

const GRID: GridSize = {cols: 12, rows: 12};

test("a grid placement becomes a percentage box in the parent's coordinate space", () => {
    const box = placementBox({kind: "grid", col: 3, row: 6, w: 6, h: 3}, GRID);

    assert.deepEqual(box, {left: "25%", top: "50%", width: "50%", height: "25%"});
});

test("a grid placement at the origin of a 4096 grid keeps full precision", () => {
    const box = placementBox({kind: "grid", col: 0, row: 0, w: 1, h: 1}, {cols: 4096, rows: 4096});

    assert.deepEqual(box, {
        left: "0%",
        top: "0%",
        width: `${(1 / 4096) * 100}%`,
        height: `${(1 / 4096) * 100}%`,
    });
});

// Absolute values arrive already expressed as percentages. Re-deriving them
// would only invent a rounding difference, so the box must be the same strings.
test("an absolute placement passes its percentage strings straight through", () => {
    const box = placementBox(
        {kind: "absolute", left: "80%", top: "4.5%", width: "16%", height: "-2%"},
        GRID,
    );

    assert.deepEqual(box, {left: "80%", top: "4.5%", width: "16%", height: "-2%"});
});

// The grid is a divisor and nothing else, so the same rect in a different grid
// must produce a different box without any other input changing.
test("the same rect resolves differently in a different grid", () => {
    const rect = {kind: "grid", col: 1, row: 1, w: 1, h: 1} as const;

    assert.notDeepEqual(placementBox(rect, {cols: 8, rows: 8}), placementBox(rect, GRID));
});

// `parseLayout` clamps rects so this is unreachable through the normal path,
// but a degenerate grid divides by zero and produces "Infinity%", which RN
// ignores silently — leaving the widget stretched across its parent with no
// clue why. The guard turns that into a visible zero-size box instead.
test("a percentage that RN could not parse collapses to 0 rather than reaching RN", () => {
    const box = placementBox({kind: "grid", col: 0, row: 0, w: 1, h: 1}, {cols: 0, rows: 0});

    assert.deepEqual(box, {left: 0, top: 0, width: 0, height: 0});
});

test("only absolute placement carries an explicit stacking order", () => {
    assert.equal(placementZIndex({kind: "grid", col: 0, row: 0, w: 1, h: 1}), undefined);
    assert.equal(
        placementZIndex({kind: "absolute", left: "0%", top: "0%", width: "1%", height: "1%"}),
        undefined,
    );
    assert.equal(
        placementZIndex({
            kind: "absolute", left: "0%", top: "0%", width: "1%", height: "1%", z: 10,
        }),
        10,
    );
});

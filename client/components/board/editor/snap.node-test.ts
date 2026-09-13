import assert from "node:assert/strict";
import test from "node:test";

import {
    MAX_GRID_LINE_CELLS,
    MIN_ABSOLUTE_SPAN,
    cellSize,
    lineIndexes,
    moveAbsolute,
    rectToPixels,
    resizeAbsolute,
    resizeAbsoluteOrigin,
    showGridLines,
    snapMove,
    snapResize,
    snapResizeOrigin,
} from "./snap.ts";

import type {CellSize} from "./snap.ts";
import type {GridRect, GridSize} from "../../../layout/grid/coords.ts";

const GRID: GridSize = {cols: 12, rows: 12};

/** A 1200x600 board on a 12x12 grid: 100px wide cells, 50px tall ones. */
const CELL: CellSize = {width: 100, height: 50};

const RECT: GridRect = {col: 4, row: 4, w: 2, h: 2};

test("a cell is the board divided by the grid, and the two axes are independent", () => {
    assert.deepEqual(cellSize({width: 1200, height: 600}, GRID), {width: 100, height: 50});
});

test("a rect becomes a pixel box in the measured board's coordinates", () => {
    assert.deepEqual(rectToPixels(RECT, CELL), {left: 400, top: 200, width: 200, height: 100});
});

// The snap threshold IS the half cell. Anything less must not move the widget,
// or a tap with a shaky finger would re-place it.
test("a drag shorter than half a cell snaps back to where it started", () => {
    assert.deepEqual(snapMove(RECT, 49, 24, CELL, GRID), RECT);
});

test("a drag past the half cell snaps to the next cell, on each axis independently", () => {
    assert.deepEqual(snapMove(RECT, 150, -26, CELL, GRID), {col: 6, row: 3, w: 2, h: 2});
});

test("a move never changes the size", () => {
    const moved = snapMove(RECT, 500, 300, CELL, GRID);

    assert.equal(moved.w, RECT.w);
    assert.equal(moved.h, RECT.h);
});

// Clamping rather than rejecting: dragging past an edge parks the widget
// against it. Rejecting there would make the whole border feel broken.
test("a move dragged past the top left clamps to the origin", () => {
    assert.deepEqual(snapMove(RECT, -9999, -9999, CELL, GRID), {col: 0, row: 0, w: 2, h: 2});
});

test("a move dragged past the bottom right clamps so the whole rect stays inside", () => {
    assert.deepEqual(snapMove(RECT, 9999, 9999, CELL, GRID), {col: 10, row: 10, w: 2, h: 2});
});

// A widget bigger than the grid can exist: parseLayout clamps geometry, but a
// smaller grid can arrive by delta afterwards. The origin must stay valid.
test("a rect wider than the grid still clamps to a non-negative origin", () => {
    const huge: GridRect = {col: 0, row: 0, w: 20, h: 20};

    assert.deepEqual(snapMove(huge, 9999, 9999, CELL, GRID), {col: 0, row: 0, w: 20, h: 20});
});

test("a resize moves the far corner and leaves the origin alone", () => {
    const resized = snapResize(RECT, 250, 120, CELL, GRID);

    assert.deepEqual(resized, {col: 4, row: 4, w: 5, h: 4});
});

test("a resize dragged inwards stops at one cell rather than collapsing", () => {
    assert.deepEqual(snapResize(RECT, -9999, -9999, CELL, GRID), {col: 4, row: 4, w: 1, h: 1});
});

test("a resize dragged outwards stops at the far edge of the grid", () => {
    assert.deepEqual(snapResize(RECT, 9999, 9999, CELL, GRID), {col: 4, row: 4, w: 8, h: 8});
});

test("a resize on a rect whose origin is already at the last cell keeps a one cell span", () => {
    const corner: GridRect = {col: 11, row: 11, w: 1, h: 1};

    assert.deepEqual(snapResize(corner, 9999, 9999, CELL, GRID), corner);
});

// The board is measured by onLayout, so the first render has no size at all.
// A zero cell would divide to Infinity and put NaN into a placement.
test("an unmeasured board produces no movement instead of NaN", () => {
    const unmeasured = cellSize({width: 0, height: 0}, GRID);

    assert.deepEqual(snapMove(RECT, 120, 120, unmeasured, GRID), RECT);
    assert.deepEqual(snapResize(RECT, 120, 120, unmeasured, GRID), RECT);
});

test("a non-finite gesture delta produces no movement", () => {
    assert.deepEqual(snapMove(RECT, Number.NaN, Number.POSITIVE_INFINITY, CELL, GRID), RECT);
    assert.deepEqual(snapResize(RECT, Number.NaN, Number.POSITIVE_INFINITY, CELL, GRID), RECT);
});

test("the grid-line overlay is drawn up to the cell ceiling and suppressed past it", () => {
    assert.equal(MAX_GRID_LINE_CELLS, 4096);
    assert.equal(showGridLines({cols: 64, rows: 64}), true);
    assert.equal(showGridLines({cols: 64, rows: 65}), false);
    assert.equal(showGridLines({cols: 4096, rows: 4096}), false);
});

test("only interior lines are drawn — the outer edges are the board's own border", () => {
    assert.deepEqual(lineIndexes(4), [1, 2, 3]);
    assert.deepEqual(lineIndexes(1), []);
});

// --- viewport-placed widgets ------------------------------------------------

const BOARD = {width: 400, height: 200};
const ABS = {
    kind: "absolute",
    left: "10%", top: "20%", width: "30%", height: "40%",
} as const;

test("moveAbsolute converts a pixel delta into a percentage of the board", () => {
    // 40px of 400 is 10%; 20px of 200 is 10%.
    const moved = moveAbsolute(ABS, 40, 20, BOARD);
    assert.equal(moved.left, "20%");
    assert.equal(moved.top, "30%");
    assert.equal(moved.width, "30%", "a move must not resize");
    assert.equal(moved.height, "40%");
});

test("moveAbsolute keeps a widget on the canvas", () => {
    // Dragged far right: clamped so its right edge stays at 100%, which is
    // what stops a widget being dragged out of reach entirely.
    const moved = moveAbsolute(ABS, 10_000, 10_000, BOARD);
    assert.equal(moved.left, "70%", "100% less its 30% width");
    assert.equal(moved.top, "60%", "100% less its 40% height");

    const back = moveAbsolute(ABS, -10_000, -10_000, BOARD);
    assert.equal(back.left, "0%");
    assert.equal(back.top, "0%");
});

test("resizeAbsolute floors the span and keeps it inside the board", () => {
    const grown = resizeAbsolute(ABS, 40, 0, BOARD);
    assert.equal(grown.width, "40%");
    assert.equal(grown.left, "10%", "a resize must not move the origin");

    const shrunk = resizeAbsolute(ABS, -10_000, -10_000, BOARD);
    assert.equal(shrunk.width, `${MIN_ABSOLUTE_SPAN}%`);
    assert.equal(shrunk.height, `${MIN_ABSOLUTE_SPAN}%`);

    const huge = resizeAbsolute(ABS, 10_000, 10_000, BOARD);
    assert.equal(huge.width, "90%", "100% less its 10% left offset");
    assert.equal(huge.height, "80%", "100% less its 20% top offset");
});

test("percentages are rounded, so a drag does not emit 17 decimal places", () => {
    const moved = moveAbsolute(ABS, 1, 1, BOARD);
    assert.match(moved.left, /^\d+(\.\d{1,2})?%$/);
    assert.match(moved.top, /^\d+(\.\d{1,2})?%$/);
});

test("overlap and z survive a move, because a drag must not silently reset them", () => {
    const withFlags = {...ABS, overlap: true, z: 3} as const;
    const moved = moveAbsolute(withFlags, 40, 0, BOARD);
    assert.equal(moved.overlap, true);
    assert.equal(moved.z, 3);
});

// --- resizing from the origin ------------------------------------------------

// Without this a widget can only grow right and down, so one pinned against
// the left edge could never be widened at all.
test("snapResizeOrigin moves the origin and keeps the far edge put", () => {
    const base = {col: 4, row: 4, w: 3, h: 3};
    const cell = {width: 10, height: 10};
    const grid = {cols: 12, rows: 12};

    // Two cells left and one up: the right/bottom edges must not move.
    const grown = snapResizeOrigin(base, -20, -10, cell, grid);
    assert.deepStrictEqual(grown, {col: 2, row: 3, w: 5, h: 4});
    assert.equal(grown.col + grown.w, base.col + base.w, "right edge fixed");
    assert.equal(grown.row + grown.h, base.row + base.h, "bottom edge fixed");
});

test("snapResizeOrigin floors the span at one cell rather than inverting", () => {
    const shrunk = snapResizeOrigin(
        {col: 0, row: 0, w: 3, h: 3}, 10_000, 10_000, {width: 10, height: 10}, {cols: 12, rows: 12},
    );
    assert.equal(shrunk.w, 1);
    assert.equal(shrunk.h, 1);
    assert.equal(shrunk.col + shrunk.w, 3, "the far edge still does not move");
});

test("snapResizeOrigin clamps at the board edge", () => {
    const base = {col: 2, row: 2, w: 2, h: 2};
    const out = snapResizeOrigin(base, -10_000, -10_000, {width: 10, height: 10}, {cols: 12, rows: 12});
    assert.equal(out.col, 0);
    assert.equal(out.row, 0);
    assert.equal(out.w, 4, "it grew to the edge, not past it");
});

test("resizeAbsoluteOrigin is the viewport equivalent", () => {
    const grown = resizeAbsoluteOrigin(ABS, -40, 0, BOARD);
    // 40px of 400 is 10%: left 10% -> 0%, width 30% -> 40%.
    assert.equal(grown.left, "0%");
    assert.equal(grown.width, "40%");
    assert.equal(
        pctNum(grown.left) + pctNum(grown.width),
        pctNum(ABS.left) + pctNum(ABS.width),
        "the right edge must not move",
    );
});

test("resizeAbsoluteOrigin floors the span", () => {
    const shrunk = resizeAbsoluteOrigin(ABS, 10_000, 10_000, BOARD);
    assert.equal(shrunk.width, `${MIN_ABSOLUTE_SPAN}%`);
    assert.equal(shrunk.height, `${MIN_ABSOLUTE_SPAN}%`);
});

function pctNum(v: string): number {
    return Number.parseFloat(v);
}

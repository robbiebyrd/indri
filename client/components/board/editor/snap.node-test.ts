import assert from "node:assert/strict";
import test from "node:test";

import {
    MAX_GRID_LINE_CELLS,
    cellSize,
    lineIndexes,
    rectToPixels,
    showGridLines,
    snapMove,
    snapResize,
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

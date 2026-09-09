import test from "node:test";
import assert from "node:assert/strict";

import {
    MAX_DIM,
    MIN_DIM,
    clampRect,
    isValidGridSize,
    isValidRect,
    isWithinBounds,
    toPercentBox,
} from "./coords.ts";
import {canPlace, collides} from "./collision.ts";

import type {GridRect, GridSize} from "./coords.ts";
import type {PlacedWidget} from "./collision.ts";

const SMALL: GridSize = {cols: 8, rows: 8};
const ODD: GridSize = {cols: 12, rows: 12};
const HUGE: GridSize = {cols: 4096, rows: 4096};

function rect(col: number, row: number, w: number, h: number): GridRect {
    return {col, row, w, h};
}

test("the dimension bounds are the ones the whole engine is built around", () => {
    assert.equal(MIN_DIM, 8);
    assert.equal(MAX_DIM, 4096);
});

test("toPercentBox is exact on an 8x8 grid", () => {
    assert.deepEqual(toPercentBox(rect(1, 7, 3, 1), SMALL), {
        left: "12.5%",
        top: "87.5%",
        width: "37.5%",
        height: "12.5%",
    });
});

test("toPercentBox is exact on a 12x12 grid, repeating decimals and all", () => {
    // 12 does not divide 100, so these strings carry full float expansion.
    // Asserting them exactly is the point: CSS consumes them verbatim, and a
    // rounding "fix" here would make adjacent widgets fail to meet.
    assert.deepEqual(toPercentBox(rect(1, 11, 7, 1), ODD), {
        left: "8.333333333333332%",
        top: "91.66666666666666%",
        width: "58.333333333333336%",
        height: "8.333333333333332%",
    });
    assert.equal(toPercentBox(rect(5, 0, 1, 1), ODD).left, "41.66666666666667%");
});

test("toPercentBox is exact on a 4096x4096 grid", () => {
    // 4096 is a power of two, so every one of these is exactly representable.
    assert.deepEqual(toPercentBox(rect(1, 4095, 3, 1), HUGE), {
        left: "0.0244140625%",
        top: "99.9755859375%",
        width: "0.0732421875%",
        height: "0.0244140625%",
    });
    assert.equal(toPercentBox(rect(2048, 0, 1, 1), HUGE).left, "50%");
});

test("a full-grid rect fills the box at every supported size", () => {
    for (const g of [SMALL, ODD, HUGE]) {
        assert.deepEqual(toPercentBox(rect(0, 0, g.cols, g.rows), g), {
            left: "0%",
            top: "0%",
            width: "100%",
            height: "100%",
        }, `${g.cols}x${g.rows}`);
    }
});

test("isValidGridSize accepts the inclusive 8..4096 range and nothing else", () => {
    assert.equal(isValidGridSize({cols: 8, rows: 8}), true);
    assert.equal(isValidGridSize({cols: 4096, rows: 4096}), true);
    assert.equal(isValidGridSize({cols: 8, rows: 4096}), true);

    assert.equal(isValidGridSize({cols: 7, rows: 8}), false);
    assert.equal(isValidGridSize({cols: 8, rows: 7}), false);
    assert.equal(isValidGridSize({cols: 4097, rows: 8}), false);
    assert.equal(isValidGridSize({cols: 8, rows: 4097}), false);
    assert.equal(isValidGridSize({cols: 0, rows: 0}), false);
    assert.equal(isValidGridSize({cols: -8, rows: 8}), false);
});

test("isValidGridSize rejects non-integer dimensions", () => {
    assert.equal(isValidGridSize({cols: 12.5, rows: 12}), false);
    assert.equal(isValidGridSize({cols: 12, rows: 12.5}), false);
    assert.equal(isValidGridSize({cols: Number.NaN, rows: 12}), false);
    assert.equal(isValidGridSize({cols: Number.POSITIVE_INFINITY, rows: 12}), false);
});

test("isValidRect requires whole cells, a non-negative origin and a positive span", () => {
    assert.equal(isValidRect(rect(0, 0, 1, 1)), true);
    assert.equal(isValidRect(rect(3, 4, 2, 5)), true);

    assert.equal(isValidRect(rect(0, 0, 0, 1)), false, "zero width");
    assert.equal(isValidRect(rect(0, 0, 1, 0)), false, "zero height");
    assert.equal(isValidRect(rect(0, 0, -1, 1)), false, "negative width");
    assert.equal(isValidRect(rect(0, 0, 1, -1)), false, "negative height");
    assert.equal(isValidRect(rect(0, 0, 1.5, 1)), false, "fractional width");
    assert.equal(isValidRect(rect(0, 0, 1, 1.5)), false, "fractional height");
    assert.equal(isValidRect(rect(-1, 0, 1, 1)), false, "negative col");
    assert.equal(isValidRect(rect(0, -1, 1, 1)), false, "negative row");
    assert.equal(isValidRect(rect(0.5, 0, 1, 1)), false, "fractional col");
    assert.equal(isValidRect(rect(0, 0.5, 1, 1)), false, "fractional row");
    assert.equal(isValidRect(rect(Number.NaN, 0, 1, 1)), false, "NaN col");
});

test("isWithinBounds treats the far edge as inclusive", () => {
    assert.equal(isWithinBounds(rect(0, 0, 8, 8), SMALL), true, "exactly fills the grid");
    assert.equal(isWithinBounds(rect(7, 7, 1, 1), SMALL), true, "last cell");
    assert.equal(isWithinBounds(rect(7, 0, 2, 1), SMALL), false, "one column past the edge");
    assert.equal(isWithinBounds(rect(0, 7, 1, 2), SMALL), false, "one row past the edge");
    assert.equal(isWithinBounds(rect(-1, 0, 1, 1), SMALL), false, "negative origin");
});

test("the AABB truth table", () => {
    const base = rect(2, 2, 3, 3); // covers cols 2..4, rows 2..4

    assert.equal(collides(base, rect(10, 10, 1, 1)), false, "disjoint");

    // Edge-touching is the case that separates this from a naive <= AABB: a
    // rect ending at col 5 must sit flush against one starting at col 5.
    assert.equal(collides(base, rect(5, 2, 3, 3)), false, "flush on the right edge");
    assert.equal(collides(base, rect(2, 5, 3, 3)), false, "flush on the bottom edge");
    assert.equal(collides(base, rect(0, 2, 2, 3)), false, "flush on the left edge");
    assert.equal(collides(base, rect(2, 0, 3, 2)), false, "flush on the top edge");

    assert.equal(collides(base, rect(5, 5, 3, 3)), false, "corner-touching only");

    assert.equal(collides(base, rect(4, 4, 3, 3)), true, "overlapping by one cell");
    assert.equal(collides(base, rect(3, 3, 1, 1)), true, "fully contained");
    assert.equal(collides(rect(3, 3, 1, 1), base), true, "fully containing");
    assert.equal(collides(base, rect(2, 2, 3, 3)), true, "identical rects");
    assert.equal(collides(base, base), true, "an identical rect by reference still overlaps");
});

test("a rect never collides with itself when placement uses its own id", () => {
    // collides() is pure geometry, so a rect does overlap its own coordinates.
    // Self-exemption lives in canPlace, keyed on id — that is what lets a drag
    // move a widget without colliding with the position it is leaving.
    const moving = rect(2, 2, 3, 3);
    const siblings: PlacedWidget[] = [{id: "a", rect: moving}];

    assert.equal(collides(moving, moving), true);
    assert.equal(canPlace(moving, "a", siblings, SMALL), true);
    assert.equal(canPlace(moving, "b", siblings, SMALL), false);
});

test("canPlace rejects a collision and accepts a free slot", () => {
    const siblings: PlacedWidget[] = [
        {id: "a", rect: rect(0, 0, 4, 4)},
        {id: "b", rect: rect(4, 0, 4, 4)},
    ];

    assert.equal(canPlace(rect(3, 3, 2, 2), "", siblings, SMALL), false, "overlaps a and b");
    assert.equal(canPlace(rect(0, 4, 8, 4), "", siblings, SMALL), true, "the free bottom half");
    assert.equal(canPlace(rect(0, 4, 4, 4), "", siblings, SMALL), true, "flush beneath a");
});

test("canPlace lets a widget move to a slot that overlaps only its old position", () => {
    const siblings: PlacedWidget[] = [
        {id: "mover", rect: rect(0, 0, 3, 3)},
        {id: "other", rect: rect(5, 5, 2, 2)},
    ];

    assert.equal(canPlace(rect(1, 1, 3, 3), "mover", siblings, SMALL), true);
    assert.equal(canPlace(rect(4, 4, 3, 3), "mover", siblings, SMALL), false, "hits 'other'");
});

test("canPlace rejects an out-of-bounds or invalid rect before looking at siblings", () => {
    assert.equal(canPlace(rect(7, 0, 2, 1), "", [], SMALL), false, "past the right edge");
    assert.equal(canPlace(rect(0, 7, 1, 2), "", [], SMALL), false, "past the bottom edge");
    assert.equal(canPlace(rect(-1, 0, 1, 1), "", [], SMALL), false, "negative origin");
    assert.equal(canPlace(rect(0, 0, 0, 1), "", [], SMALL), false, "zero span");
    assert.equal(canPlace(rect(0, 0, 1.5, 1), "", [], SMALL), false, "fractional span");
});

test("an absolute widget is exempt as both subject and obstacle by being absent from siblings", () => {
    // This encodes the contract with the caller rather than any branch in the
    // module: absolute widgets have no grid rect, so they are simply never
    // passed in. Anything sitting "under" one therefore places freely.
    const gridSiblings: PlacedWidget[] = [{id: "panel", rect: rect(0, 0, 2, 2)}];

    assert.equal(canPlace(rect(4, 4, 2, 2), "", gridSiblings, SMALL), true);
    assert.equal(canPlace(rect(0, 0, 2, 2), "", gridSiblings, SMALL), false,
        "a grid sibling still blocks — only absent widgets are exempt");
});

test("collision is scoped per grid level: the caller supplies one level's siblings", () => {
    // The same coordinates in a parent grid and in a sub-grid are unrelated
    // spaces, so the module never sees both sets at once.
    const parentSiblings: PlacedWidget[] = [{id: "board", rect: rect(0, 0, 4, 4)}];
    const subGridSiblings: PlacedWidget[] = [{id: "tile", rect: rect(0, 0, 1, 1)}];
    const subGrid: GridSize = {cols: 8, rows: 8};

    assert.equal(canPlace(rect(0, 0, 1, 1), "", parentSiblings, SMALL), false, "inside 'board'");
    assert.equal(canPlace(rect(1, 0, 1, 1), "", subGridSiblings, subGrid), true, "next to 'tile'");
});

test("clampRect pulls an out-of-bounds rect back inside without resizing it", () => {
    assert.deepEqual(clampRect(rect(7, 7, 3, 3), SMALL), rect(5, 5, 3, 3), "past both far edges");
    assert.deepEqual(clampRect(rect(-4, -4, 3, 3), SMALL), rect(0, 0, 3, 3), "past both near edges");
    assert.deepEqual(clampRect(rect(2, 2, 3, 3), SMALL), rect(2, 2, 3, 3), "already inside, untouched");
});

test("clampRect shrinks only a rect that cannot fit", () => {
    assert.deepEqual(clampRect(rect(0, 0, 20, 20), SMALL), rect(0, 0, 8, 8), "larger than the grid");
    assert.deepEqual(clampRect(rect(3, 0, 20, 4), SMALL), rect(0, 0, 8, 4), "wider than the grid only");
});

test("clampRect normalises fractional, zero and non-finite input into a valid rect", () => {
    // clampRect is the "keep this widget" path, so it must always return
    // something placeable — callers stop checking after clamping.
    for (const bad of [
        rect(1.7, 2.9, 2.4, 3.6),
        rect(0, 0, 0, 0),
        rect(-3, -3, -3, -3),
        rect(Number.NaN, Number.NaN, Number.NaN, Number.NaN),
        rect(Number.POSITIVE_INFINITY, 0, Number.POSITIVE_INFINITY, 1),
    ]) {
        const clamped = clampRect(bad, SMALL);
        assert.equal(isValidRect(clamped), true, `valid: ${JSON.stringify(bad)}`);
        assert.equal(isWithinBounds(clamped, SMALL), true, `in bounds: ${JSON.stringify(bad)}`);
    }
});

test("a 4096x4096 grid costs the same as an 8x8 one — the grid is only a divisor", () => {
    // Guards the "never materialise cells" rule. 4096x4096 is 16.7M cells; if
    // anything ever allocated per cell, this test would take seconds and tens
    // of megabytes instead of milliseconds and nothing.
    const siblings: PlacedWidget[] = [
        {id: "a", rect: rect(0, 0, 1024, 1024)},
        {id: "b", rect: rect(1024, 0, 1024, 1024)},
        {id: "c", rect: rect(2048, 2048, 2048, 2048)},
        {id: "d", rect: rect(0, 4095, 1, 1)},
        {id: "e", rect: rect(4095, 0, 1, 1)},
    ];

    const started = performance.now();
    for (let i = 0; i < 10_000; i++) {
        assert.equal(canPlace(rect(1024, 1024, 1024, 1024), "", siblings, HUGE), true);
        assert.equal(canPlace(rect(1023, 1023, 2, 2), "", siblings, HUGE), false);
        toPercentBox(rect(i % 4096, 0, 1, 1), HUGE);
    }
    const elapsed = performance.now() - started;

    // 30k operations against a 16.7M-cell space. Generous enough not to flake
    // on a loaded machine, tight enough that per-cell work cannot hide.
    assert.ok(elapsed < 1000, `expected well under 1s, took ${elapsed.toFixed(1)}ms`);
});

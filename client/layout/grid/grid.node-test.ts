import test from "node:test"
import assert from "node:assert/strict"

import {
    toPercentBox,
    validateGridSize,
    clampRect,
    MIN_DIM,
    MAX_DIM,
} from "./coords.ts"
import type {GridRect, GridSize, PlacedWidget} from "./coords.ts"
import {collides, canPlace} from "./collision.ts"

// ---------------------------------------------------------------------------
// toPercentBox
// ---------------------------------------------------------------------------

test("toPercentBox: 8×8 grid, top-left half-sized rect", () => {
    const g: GridSize = {cols: 8, rows: 8}
    const r: GridRect = {col: 0, row: 0, w: 4, h: 4}
    const box = toPercentBox(r, g)
    assert.equal(box.left, "0%")
    assert.equal(box.top, "0%")
    assert.equal(box.width, "50%")
    assert.equal(box.height, "50%")
})

test("toPercentBox: 12×12 grid, full-grid rect", () => {
    const g: GridSize = {cols: 12, rows: 12}
    const r: GridRect = {col: 0, row: 0, w: 12, h: 12}
    const box = toPercentBox(r, g)
    assert.equal(box.left, "0%")
    assert.equal(box.top, "0%")
    assert.equal(box.width, "100%")
    assert.equal(box.height, "100%")
})

test("toPercentBox: 4096×4096 grid, centred half-sized rect", () => {
    const g: GridSize = {cols: 4096, rows: 4096}
    const r: GridRect = {col: 1024, row: 1024, w: 2048, h: 2048}
    const box = toPercentBox(r, g)
    assert.equal(box.left, "25%")
    assert.equal(box.top, "25%")
    assert.equal(box.width, "50%")
    assert.equal(box.height, "50%")
})

test("toPercentBox: returns string-valued box, never an array", () => {
    const g: GridSize = {cols: MAX_DIM, rows: MAX_DIM}
    const r: GridRect = {col: 0, row: 0, w: MAX_DIM, h: MAX_DIM}
    const box = toPercentBox(r, g)
    assert.equal(typeof box.left, "string")
    assert.equal(typeof box.top, "string")
    assert.equal(typeof box.width, "string")
    assert.equal(typeof box.height, "string")
    assert.ok(!Array.isArray(box))
})

// ---------------------------------------------------------------------------
// validateGridSize
// ---------------------------------------------------------------------------

test("validateGridSize: cols < MIN_DIM is rejected", () => {
    assert.throws(() => validateGridSize({cols: 7, rows: 8}))
})

test("validateGridSize: cols > MAX_DIM is rejected", () => {
    assert.throws(() => validateGridSize({cols: 4097, rows: 8}))
})

test("validateGridSize: minimum valid grid (8×8) succeeds", () => {
    assert.doesNotThrow(() => validateGridSize({cols: MIN_DIM, rows: MIN_DIM}))
})

test("validateGridSize: maximum valid grid (4096×4096) succeeds", () => {
    assert.doesNotThrow(() => validateGridSize({cols: MAX_DIM, rows: MAX_DIM}))
})

// ---------------------------------------------------------------------------
// GridRect validation via clampRect
// ---------------------------------------------------------------------------

test("clampRect: zero w is invalid", () => {
    const g: GridSize = {cols: 8, rows: 8}
    assert.throws(() => clampRect({col: 0, row: 0, w: 0, h: 1}, g))
})

test("clampRect: negative w is invalid", () => {
    const g: GridSize = {cols: 8, rows: 8}
    assert.throws(() => clampRect({col: 0, row: 0, w: -1, h: 1}, g))
})

test("clampRect: fractional w is invalid", () => {
    const g: GridSize = {cols: 8, rows: 8}
    assert.throws(() => clampRect({col: 0, row: 0, w: 1.5, h: 1}, g))
})

test("clampRect: out-of-bounds rect is clamped to fit", () => {
    const g: GridSize = {cols: 8, rows: 8}
    // col:9 is beyond the grid — should be clamped
    const clamped = clampRect({col: 9, row: 0, w: 1, h: 1}, g)
    assert.ok(clamped.col >= 0)
    assert.ok(clamped.col + clamped.w <= g.cols)
    assert.ok(clamped.row >= 0)
    assert.ok(clamped.row + clamped.h <= g.rows)
})

// ---------------------------------------------------------------------------
// collides — AABB truth table
// ---------------------------------------------------------------------------

test("collides: disjoint rects → false", () => {
    assert.equal(collides({col: 0, row: 0, w: 2, h: 2}, {col: 3, row: 0, w: 2, h: 2}), false)
})

test("collides: edge-touching horizontally → false (not a collision)", () => {
    assert.equal(collides({col: 0, row: 0, w: 2, h: 2}, {col: 2, row: 0, w: 2, h: 2}), false)
})

test("collides: edge-touching vertically → false (not a collision)", () => {
    assert.equal(collides({col: 0, row: 0, w: 2, h: 2}, {col: 0, row: 2, w: 2, h: 2}), false)
})

test("collides: corner-touching → false (no area overlap)", () => {
    assert.equal(collides({col: 0, row: 0, w: 2, h: 2}, {col: 2, row: 2, w: 2, h: 2}), false)
})

test("collides: containment → true", () => {
    assert.equal(collides({col: 0, row: 0, w: 4, h: 4}, {col: 1, row: 1, w: 2, h: 2}), true)
})

test("collides: identical rects → true", () => {
    assert.equal(collides({col: 0, row: 0, w: 2, h: 2}, {col: 0, row: 0, w: 2, h: 2}), true)
})

test("collides: overlapping (not just touching) → true", () => {
    assert.equal(collides({col: 0, row: 0, w: 3, h: 3}, {col: 2, row: 2, w: 3, h: 3}), true)
})

// ---------------------------------------------------------------------------
// canPlace
// ---------------------------------------------------------------------------

function makeWidget(id: string, col: number, row: number, w: number, h: number): PlacedWidget {
    return {id, placement: {kind: "grid", col, row, w, h}}
}

function makeAbsoluteWidget(id: string): PlacedWidget {
    return {id, placement: {kind: "absolute"}}
}

test("canPlace: no siblings → can place", () => {
    const g: GridSize = {cols: 8, rows: 8}
    const r: GridRect = {col: 0, row: 0, w: 2, h: 2}
    assert.equal(canPlace(r, "w1", [], g), true)
})

test("canPlace: non-overlapping sibling → can place", () => {
    const g: GridSize = {cols: 8, rows: 8}
    const r: GridRect = {col: 0, row: 0, w: 2, h: 2}
    const siblings: PlacedWidget[] = [makeWidget("w2", 3, 0, 2, 2)]
    assert.equal(canPlace(r, "w1", siblings, g), true)
})

test("canPlace: overlapping grid sibling → cannot place", () => {
    const g: GridSize = {cols: 8, rows: 8}
    const r: GridRect = {col: 0, row: 0, w: 3, h: 3}
    const siblings: PlacedWidget[] = [makeWidget("w2", 2, 2, 3, 3)]
    assert.equal(canPlace(r, "w1", siblings, g), false)
})

test("canPlace: absolute sibling is ignored (exempt from collision)", () => {
    const g: GridSize = {cols: 8, rows: 8}
    // Place widget at same position as an absolute sibling — should be allowed
    const r: GridRect = {col: 0, row: 0, w: 4, h: 4}
    const siblings: PlacedWidget[] = [makeAbsoluteWidget("abs1")]
    assert.equal(canPlace(r, "w1", siblings, g), true)
})

test("canPlace: absolute subject widget → always true regardless of siblings", () => {
    const g: GridSize = {cols: 8, rows: 8}
    // An absolute widget being placed — not subject to collision checks
    const r: GridRect = {col: 0, row: 0, w: 4, h: 4}
    const siblings: PlacedWidget[] = [makeWidget("w2", 0, 0, 4, 4)]
    // canPlace with the widget itself being absolute (id belongs to an absolute widget)
    // The subject's placement kind is passed via the PlacedWidget list or we test differently.
    // Per spec: absolute widgets are excluded from BOTH sides of collision.
    // We verify by passing widget id "abs1" whose placement.kind is "absolute" in siblings —
    // that is the "obstacle" side. For the "subject" side we use a separate API call:
    // When the subject widget (identified by id) appears in siblings as absolute, skip.
    const absWidget = makeAbsoluteWidget("abs1")
    const siblingsWithAbs: PlacedWidget[] = [...siblings, absWidget]
    // "abs1" is the subject; it's in siblings as absolute → canPlace returns true
    assert.equal(canPlace(r, "abs1", siblingsWithAbs, g), true)
})

test("canPlace: self-vs-self by id → not counted as collision", () => {
    const g: GridSize = {cols: 8, rows: 8}
    const r: GridRect = {col: 0, row: 0, w: 2, h: 2}
    // Same widget appears in siblings (e.g. during re-layout) — should not collide with itself
    const siblings: PlacedWidget[] = [makeWidget("w1", 0, 0, 2, 2)]
    assert.equal(canPlace(r, "w1", siblings, g), true)
})

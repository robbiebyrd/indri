import test from "node:test"
import assert from "node:assert/strict"

import {parseColor, clampNumber, parseDate, dedupeValues} from "./picker-helpers.ts"

// ---------------------------------------------------------------------------
// parseColor
// ---------------------------------------------------------------------------

test("parseColor: #rgb expands to #rrggbb", () => {
    assert.equal(parseColor("#abc"), "#aabbcc")
})

test("parseColor: #rrggbb is returned canonically (lowercase)", () => {
    assert.equal(parseColor("#AABBCC"), "#aabbcc")
})

test("parseColor: already lowercase #rrggbb is returned unchanged", () => {
    assert.equal(parseColor("#ff0000"), "#ff0000")
})

test("parseColor: leading/trailing whitespace is trimmed", () => {
    assert.equal(parseColor("  #fff  "), "#ffffff")
})

test("parseColor: #rgb expands each nibble correctly", () => {
    assert.equal(parseColor("#f0a"), "#ff00aa")
})

test("parseColor: invalid string returns undefined", () => {
    assert.equal(parseColor("red"), undefined)
    assert.equal(parseColor("#gg0000"), undefined)
    assert.equal(parseColor("#12345"), undefined)
    assert.equal(parseColor(""), undefined)
    assert.equal(parseColor("#0000000"), undefined)
})

test("parseColor: #000 → #000000", () => {
    assert.equal(parseColor("#000"), "#000000")
})

test("parseColor: #fff → #ffffff", () => {
    assert.equal(parseColor("#fff"), "#ffffff")
})

// ---------------------------------------------------------------------------
// clampNumber
// ---------------------------------------------------------------------------

test("clampNumber: no bounds → value unchanged", () => {
    assert.equal(clampNumber(5), 5)
    assert.equal(clampNumber(-100), -100)
})

test("clampNumber: value below min → clamped to min", () => {
    assert.equal(clampNumber(3, 8), 8)
})

test("clampNumber: value above max → clamped to max", () => {
    assert.equal(clampNumber(100, 0, 50), 50)
})

test("clampNumber: value within [min, max] → unchanged", () => {
    assert.equal(clampNumber(25, 0, 50), 25)
})

test("clampNumber: min === max → value clamped to that point", () => {
    assert.equal(clampNumber(7, 5, 5), 5)
})

// ---------------------------------------------------------------------------
// parseDate
// ---------------------------------------------------------------------------

test("parseDate: valid ISO date round-trips unchanged", () => {
    assert.equal(parseDate("2024-03-15"), "2024-03-15")
})

test("parseDate: leading/trailing whitespace is trimmed", () => {
    assert.equal(parseDate("  2024-01-01  "), "2024-01-01")
})

test("parseDate: invalid format returns undefined", () => {
    assert.equal(parseDate("15/03/2024"), undefined)
    assert.equal(parseDate("not a date"), undefined)
    assert.equal(parseDate(""), undefined)
})

test("parseDate: invalid calendar date (Feb 30) returns undefined", () => {
    assert.equal(parseDate("2024-02-30"), undefined)
})

test("parseDate: year boundaries are accepted", () => {
    assert.equal(parseDate("2000-01-01"), "2000-01-01")
    assert.equal(parseDate("1999-12-31"), "1999-12-31")
})

// ---------------------------------------------------------------------------
// dedupeValues
// ---------------------------------------------------------------------------

test("dedupeValues: no duplicates → order preserved", () => {
    assert.deepEqual(dedupeValues(["a", "b", "c"]), ["a", "b", "c"])
})

test("dedupeValues: duplicates removed keeping first occurrence", () => {
    assert.deepEqual(dedupeValues(["a", "b", "a", "c", "b"]), ["a", "b", "c"])
})

test("dedupeValues: empty array → empty array", () => {
    assert.deepEqual(dedupeValues([]), [])
})

test("dedupeValues: single element → same element", () => {
    assert.deepEqual(dedupeValues(["x"]), ["x"])
})

test("dedupeValues: all same value → single element", () => {
    assert.deepEqual(dedupeValues(["z", "z", "z"]), ["z"])
})

test("dedupeValues: order of first occurrence is stable", () => {
    assert.deepEqual(dedupeValues(["c", "a", "b", "a", "c"]), ["c", "a", "b"])
})

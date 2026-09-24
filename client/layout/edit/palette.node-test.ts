import test from "node:test"
import assert from "node:assert/strict"
import {z} from "zod"

import type {PlacedWidget, GridSize} from "../grid/coords.ts"
import {firstFree} from "./place.ts"
import {registerWidget, getAllWidgets, resetRegistry} from "../registry/registry.ts"

// ---------------------------------------------------------------------------
// firstFree
// ---------------------------------------------------------------------------

const g10: GridSize = {cols: 10, rows: 10}

test("firstFree: empty grid → places at (0,0)", () => {
    const r = firstFree({w: 2, h: 2}, [], g10)
    assert.ok(r)
    assert.equal(r.col, 0)
    assert.equal(r.row, 0)
    assert.equal(r.w, 2)
    assert.equal(r.h, 2)
})

test("firstFree: first cell occupied → places at next free position", () => {
    const sibs: PlacedWidget[] = [
        {id: "w1", placement: {kind: "grid", col: 0, row: 0, w: 2, h: 2}},
    ]
    const r = firstFree({w: 2, h: 2}, sibs, g10)
    assert.ok(r)
    assert.equal(r.row, 0)
    assert.equal(r.col, 2)
})

test("firstFree: first row fully occupied → wraps to next row", () => {
    const sibs: PlacedWidget[] = [
        {id: "w1", placement: {kind: "grid", col: 0, row: 0, w: 10, h: 1}},
    ]
    const r = firstFree({w: 2, h: 2}, sibs, g10)
    assert.ok(r)
    assert.equal(r.row, 1)
    assert.equal(r.col, 0)
})

test("firstFree: full grid → returns undefined", () => {
    const sibs: PlacedWidget[] = [
        {id: "w1", placement: {kind: "grid", col: 0, row: 0, w: 10, h: 10}},
    ]
    const r = firstFree({w: 1, h: 1}, sibs, g10)
    assert.equal(r, undefined)
})

test("firstFree: size larger than remaining space → returns undefined", () => {
    const sibs: PlacedWidget[] = [
        {id: "w1", placement: {kind: "grid", col: 0, row: 0, w: 6, h: 6}},
    ]
    const r = firstFree({w: 6, h: 6}, sibs, g10)
    assert.equal(r, undefined)
})

test("firstFree: absolute siblings are ignored (do not block placement)", () => {
    const sibs: PlacedWidget[] = [
        {id: "abs1", placement: {kind: "absolute"}},
    ]
    const r = firstFree({w: 2, h: 2}, sibs, g10)
    assert.ok(r)
    assert.equal(r.col, 0)
    assert.equal(r.row, 0)
})

test("firstFree: scan bounded to 64×64 even on a large grid", () => {
    const bigGrid: GridSize = {cols: 4096, rows: 4096}
    const sibs: PlacedWidget[] = [
        {id: "w1", placement: {kind: "grid", col: 0, row: 0, w: 64, h: 64}},
    ]
    const r = firstFree({w: 1, h: 1}, sibs, bigGrid)
    assert.equal(r, undefined)
})

test("firstFree: 1×1 widget fits at (0,0) on empty large grid", () => {
    const r = firstFree({w: 1, h: 1}, [], {cols: 4096, rows: 4096})
    assert.ok(r)
    assert.equal(r.col, 0)
    assert.equal(r.row, 0)
})

// ---------------------------------------------------------------------------
// Widget registry — palette lists exactly the registered widget types
//
// We cannot import widgets.ts in Node tests because it imports React Native
// components. Instead we define inline test definitions that mirror the real
// ones, following the pattern in registry.node-test.ts.
// ---------------------------------------------------------------------------

const textSchema = z.object({
    text: z.string(),
    fontSize: z.number().positive().optional(),
}).strict()

const imageSchema = z.object({
    uri: z.string().url(),
}).strict()

function makeTestDefs() {
    resetRegistry()
    registerWidget({type: "text", schema: textSchema, fields: [{key: "text", label: "Text", kind: "text"}], defaults: {text: ""}, Component: null, api: () => ({})})
    registerWidget({type: "image", schema: imageSchema, fields: [{key: "uri", label: "URL", kind: "uri", accept: "image"}], defaults: {uri: "https://example.com/placeholder.png"}, Component: null, api: () => ({})})
    registerWidget({type: "subgrid", schema: z.object({grid: z.object({cols: z.number(), rows: z.number()}), widgets: z.record(z.string(), z.unknown())}), fields: [], defaults: {grid: {cols: 3, rows: 3}, widgets: {}}, Component: null, api: () => ({})})
}

test("palette lists exactly the registered widget types", () => {
    makeTestDefs()
    const types = getAllWidgets().map(d => d.type).sort()
    assert.deepEqual(types, ["image", "subgrid", "text"])
})

test("each registered widget has non-empty defaults that pass its own schema", () => {
    makeTestDefs()
    for (const def of getAllWidgets()) {
        const result = def.schema.safeParse(def.defaults)
        assert.ok(
            result.success,
            `widget "${def.type}" defaults fail schema: ${JSON.stringify(!result.success && result.error?.issues)}`,
        )
    }
})

test("addWidget payload built from defaults validates against the widget schema", () => {
    makeTestDefs()
    for (const def of getAllWidgets()) {
        const result = def.schema.safeParse(def.defaults)
        assert.ok(
            result.success,
            `addWidget defaults for "${def.type}" invalid: ${JSON.stringify(!result.success && result.error?.issues)}`,
        )
    }
})

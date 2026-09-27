import test from "node:test"
import assert from "node:assert/strict"
import { buildPositionalMap, resolvePath } from "./positional-map.ts"

test("buildPositionalMap sorts keys alphanumerically", () => {
    const obj = { stage: {}, players: {}, code: "X", data: {} }
    const map = buildPositionalMap(obj)
    assert.equal(map["code"], 0)
    assert.equal(map["data"], 1)
    assert.equal(map["players"], 2)
    assert.equal(map["stage"], 3)
})

test("resolvePath converts integer array to string dot-path", () => {
    const obj = { a: { b: { c: "v" } } }
    // a=0, a.b=0, a.b.c=0
    const result = resolvePath([0, 0, 0], obj)
    assert.equal(result, "a.b.c")
})

test("resolvePath passes array indices through unchanged", () => {
    const obj = { board: [["", ""], ["", ""]] }
    // board=0, then raw array indices 1, 1
    const result = resolvePath([0, 1, 1], obj)
    assert.equal(result, "board.1.1")
})

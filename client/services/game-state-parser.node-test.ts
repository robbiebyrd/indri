// Runtime regression tests for the delta reducer. This is a plain Node test
// (no jest/RN needed — the parser only depends on a type). Run with:
//   pnpm test
import test from "node:test";
import assert from "node:assert/strict";

import {GameStateParser} from "./game-state-parser.ts";

function delta(ts: number, updated?: Record<string, unknown>, removed?: string[]): any {
    return {op: "update", ts: new Date(ts).toISOString(), updated, removed};
}

test("a newer delta applies on top of the keyframe, and the keyframe is not mutated", () => {
    const p = new GameStateParser<any>();
    const base = {code: "G1", n: 1};
    p.set(base, new Date(1000));
    p.update(delta(2000, {n: 2}));
    assert.equal(p.current().n, 2, "newer delta applied");
    assert.equal(base.n, 1, "keyframe object not mutated");
});

test("a delta newer than an incoming keyframe survives the re-keyframe (no loss)", () => {
    const p = new GameStateParser<any>();
    p.update(delta(3000, {x: "new"}));
    p.set({x: "base"}, new Date(1000));
    assert.equal(p.current().x, "new", "future delta preserved after keyframe");
});

test("a delta before any keyframe doesn't throw; state is undefined until keyframe", () => {
    const p = new GameStateParser<any>();
    assert.doesNotThrow(() => p.update(delta(5000, {a: 1})), "delta-before-keyframe does not throw");
    assert.equal(p.current(), undefined, "state undefined before keyframe");
    p.set({a: 0}, new Date(4000));
    assert.equal(p.current().a, 1, "delta replays once keyframe arrives");
});

test("prototype pollution via a crafted path is blocked", () => {
    const p = new GameStateParser<any>();
    p.set({}, new Date(1000));
    p.update(delta(2000, {"__proto__.polluted": "yes"}));
    assert.equal(({} as any).polluted, undefined, "Object.prototype not polluted");
});

test("numeric intermediate segments preserve arrays", () => {
    const p = new GameStateParser<any>();
    p.set({board: [["", "", ""]]}, new Date(1000));
    p.update(delta(2000, {"board.0.1": "X"}));
    const board = p.current().board;
    assert.ok(Array.isArray(board) && board[0][1] === "X", "array preserved on nested update");
});

test("removed deletes a key", () => {
    const p = new GameStateParser<any>();
    p.set({a: 1, b: 2}, new Date(1000));
    p.update(delta(2000, undefined, ["b"]));
    assert.equal(p.current().b, undefined, "removed key deleted");
    assert.equal(p.current().a, 1, "sibling key retained");
});

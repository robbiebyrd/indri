// Runtime regression tests for the delta reducer. This is a plain Node test
// (no jest/RN needed — the parser only depends on a type). Run with:
//   node --experimental-strip-types services/game-state-parser.node-test.ts
import {GameStateParser} from "./game-state-parser.ts";

let failures = 0;

function assert(name: string, cond: boolean): void {
    console.log((cond ? "PASS " : "FAIL ") + name);
    if (!cond) failures++;
}

function delta(ts: number, updated?: Record<string, unknown>, removed?: string[]): any {
    return {op: "update", ts: new Date(ts).toISOString(), updated, removed};
}

// A newer delta applies on top of the keyframe, and the keyframe is not mutated.
{
    const p = new GameStateParser<any>();
    const base = {code: "G1", n: 1};
    p.set(base, new Date(1000));
    p.update(delta(2000, {n: 2}));
    assert("newer delta applied", p.current().n === 2);
    assert("keyframe object not mutated", base.n === 1);
}

// A delta newer than an incoming keyframe survives the re-keyframe (no loss).
{
    const p = new GameStateParser<any>();
    p.update(delta(3000, {x: "new"}));
    p.set({x: "base"}, new Date(1000));
    assert("future delta preserved after keyframe", p.current().x === "new");
}

// A delta before any keyframe doesn't throw; state is undefined until keyframe.
{
    const p = new GameStateParser<any>();
    let threw = false;
    try {
        p.update(delta(5000, {a: 1}));
    } catch {
        threw = true;
    }
    assert("delta-before-keyframe does not throw", !threw);
    assert("state undefined before keyframe", p.current() === undefined);
    p.set({a: 0}, new Date(4000));
    assert("delta replays once keyframe arrives", p.current().a === 1);
}

// Prototype pollution via a crafted path is blocked.
{
    const p = new GameStateParser<any>();
    p.set({}, new Date(1000));
    p.update(delta(2000, {"__proto__.polluted": "yes"}));
    assert("Object.prototype not polluted", ({} as any).polluted === undefined);
}

// Numeric intermediate segments preserve arrays.
{
    const p = new GameStateParser<any>();
    p.set({board: [["", "", ""]]}, new Date(1000));
    p.update(delta(2000, {"board.0.1": "X"}));
    const board = p.current().board;
    assert("array preserved on nested update", Array.isArray(board) && board[0][1] === "X");
}

// removed deletes a key.
{
    const p = new GameStateParser<any>();
    p.set({a: 1, b: 2}, new Date(1000));
    p.update(delta(2000, undefined, ["b"]));
    assert("removed key deleted", p.current().b === undefined && p.current().a === 1);
}

console.log(failures === 0 ? "\nALL PARSER TESTS PASSED" : `\n${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);

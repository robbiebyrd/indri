import test from "node:test";
import assert from "node:assert/strict";

import {lua} from "fengari";
import {DEFAULT_INSTRUCTION_BUDGET, LuaRuntime} from "./runtime.ts";

import type {LuaResult} from "./runtime.ts";

function expectOk<T>(r: LuaResult<T>, what: string): T {
    assert.ok(r.ok, `${what} should have succeeded, got: ${r.ok ? "" : r.error}`);
    return r.value;
}

function expectError<T>(r: LuaResult<T>, what: string): string {
    if (r.ok) assert.fail(`${what} should have failed`);
    return r.error;
}

/**
 * Current persistent stack depth. Reading it through `guard` is deliberate: the
 * value observed is the depth `guard` recorded on entry, which is the same
 * depth it restores to on exit, so the probe cannot itself perturb the answer.
 */
function stackDepth(rt: LuaRuntime): number {
    return expectOk(rt.guard((L) => lua.lua_gettop(L)), "reading the stack depth");
}

test("a syntax error is returned as a value, not thrown", () => {
    const rt = new LuaRuntime();
    const error = expectError(rt.run("return (", "broken"), "an unparseable chunk");
    assert.match(error, /unexpected symbol/);
    assert.match(error, /broken/, "the chunk name locates the failure");
    rt.dispose();
});

test("a runtime error carries the lua message and the chunk name", () => {
    const rt = new LuaRuntime();
    const error = expectError(rt.run("error('boom')", "widget-42"), "a chunk calling error()");
    assert.match(error, /boom/);
    assert.match(error, /widget-42:1:/, "lua prefixes the message with chunkname:line");
    rt.dispose();
});

test("an error object with no string form is reported instead of crashing the host", () => {
    // `lua_tostring` returns null for a table, so this is the path where a naive
    // to_jsstring(null) would throw a TypeError out of the runtime.
    const rt = new LuaRuntime();
    const error = expectError(rt.run("error({code = 1})", "table-error"), "error() with a table");
    assert.match(error, /no string representation/);
    rt.dispose();
});

test("a runaway loop is aborted by the instruction budget", () => {
    const rt = new LuaRuntime({instructionBudget: 5_000});

    const started = Date.now();
    const error = expectError(rt.run("while true do end", "runaway"), "an infinite loop");
    const elapsed = Date.now() - started;

    assert.match(error, /instruction budget of 5000 exceeded/);
    assert.ok(elapsed < 2_000, `the abort must be prompt, took ${elapsed}ms`);
    rt.dispose();
});

test("the state still works after a chunk errored", () => {
    const rt = new LuaRuntime({instructionBudget: 5_000});

    expectError(rt.run("error('boom')", "bad"), "a runtime error");
    expectError(rt.run("return (", "worse"), "a syntax error");
    expectError(rt.run("while true do end", "worst"), "a budget abort");

    // Globals set before the failures survive, and new chunks still run.
    expectOk(rt.run("survivor = 1 + 1", "good"), "a chunk after three failures");
    expectOk(rt.run("assert(survivor == 2, 'globals were lost')", "check"), "reading a global");
    rt.dispose();
});

test("the lua stack returns to its baseline after 100 mixed successes and failures", () => {
    const rt = new LuaRuntime({instructionBudget: 5_000});

    const baseline = stackDepth(rt);
    assert.equal(baseline, 0, "a fresh sandboxed state starts empty");

    const chunks = [
        "return 1 + 1", // succeeds and discards a result
        "local t = {} for i = 1, 10 do t[i] = i end return table.concat(t)",
        "return (", // load failure: the message is left on the stack
        "error('boom')", // pcall failure: a string error object
        "error({code = 1})", // pcall failure: a non-string error object
        "while true do end", // budget abort from inside the hook
    ];

    for (let i = 0; i < 100; i++) {
        for (const src of chunks) rt.run(src, `mixed-${i}`);
    }

    assert.equal(stackDepth(rt), baseline, "every run path must drain what it pushed");
    rt.dispose();
});

test("guard restores the stack even when the operation pushes and then throws", () => {
    const rt = new LuaRuntime();
    assert.equal(stackDepth(rt), 0);

    expectOk(rt.guard((L) => {
        lua.lua_pushnil(L);
        lua.lua_pushnil(L);
        return lua.lua_gettop(L);
    }), "an operation leaving values behind");
    assert.equal(stackDepth(rt), 0, "values left by a successful operation are dropped");

    expectError(rt.guard((L) => {
        lua.lua_pushnil(L);
        throw new Error("host blew up");
    }), "an operation that throws");
    assert.equal(stackDepth(rt), 0, "values left by a throwing operation are dropped");

    // Nested entries restore to their own entry depth, not to zero.
    expectOk(rt.guard((L) => {
        lua.lua_pushnil(L);
        assert.equal(expectOk(rt.guard((inner) => lua.lua_gettop(inner)), "the nested depth"), 1);
        expectError(rt.guard(() => {
            throw new Error("nested blew up");
        }), "a nested throw");
        return lua.lua_gettop(L);
    }), "a nested guard");
    assert.equal(stackDepth(rt), 0);

    rt.dispose();
});

test("guard returns the operation's value and captures a host exception", () => {
    const rt = new LuaRuntime();

    assert.equal(expectOk(rt.guard(() => 42), "a plain operation"), 42);
    assert.match(
        expectError(rt.guard(() => {
            throw new TypeError("host blew up");
        }), "a throwing operation"),
        /TypeError: host blew up/,
    );

    rt.dispose();
});

test("the instruction budget resets at the start of each top-level call", () => {
    // 700 loop iterations, so this chunk is expensive but finite.
    const chunk = "local s = 0 for i = 1, 700 do s = s + i end return s";

    // Pin its cost from below: at a 1_000-instruction budget it aborts, so one
    // run costs more than 1_000 instructions.
    const tight = new LuaRuntime({instructionBudget: 1_000});
    assert.match(
        expectError(tight.run(chunk, "cost-probe"), "the probe chunk under a tight budget"),
        /instruction budget/,
    );
    tight.dispose();

    // ...and from above, by the 20 runs below all succeeding at 2_000. Cost is
    // therefore in (1_000, 2_000], so any two runs already exceed one budget
    // and 20 runs exceed it roughly sevenfold. Without a per-call reset, the
    // second iteration would fail.
    const rt = new LuaRuntime({instructionBudget: 2_000});
    for (let i = 0; i < 20; i++) {
        expectOk(rt.run(chunk, `reset-${i}`), `call ${i} must not inherit a spent budget`);
    }
    rt.dispose();
});

test("dispose is idempotent and later calls fail as values", () => {
    const rt = new LuaRuntime();
    expectOk(rt.run("return 1", "before"), "a chunk before dispose");

    rt.dispose();
    assert.doesNotThrow(() => rt.dispose(), "dispose must be safe to call twice");

    assert.match(expectError(rt.run("return 1", "after"), "run after dispose"), /disposed/);
    assert.match(expectError(rt.guard(() => 1), "guard after dispose"), /disposed/);
});

test("disposing from inside a guarded operation does not throw out of guard", () => {
    // guard's stack restore runs in a finally, and lua_close frees the stack,
    // so an unconditional restore here would throw past every guarantee guard
    // makes. `op` is arbitrary host code, so this is reachable today.
    const rt = new LuaRuntime();
    const r = rt.guard((L) => {
        lua.lua_pushnil(L);
        rt.dispose();
        return "disposed mid-call";
    });

    assert.equal(expectOk(r, "an operation that disposes the runtime"), "disposed mid-call");
    assert.match(expectError(rt.run("return 1", "after"), "run after a mid-call dispose"), /disposed/);
});

test("onError fires for failures and not for successes", () => {
    const seen: string[] = [];
    const rt = new LuaRuntime({onError: (err) => seen.push(err)});

    expectOk(rt.run("return 1 + 1", "fine"), "a working chunk");
    assert.deepEqual(seen, [], "a success must not be reported as an error");

    const returned = expectError(rt.run("error('boom')", "bad"), "a failing chunk");
    assert.equal(seen.length, 1, "one report per failure");
    assert.equal(seen[0], returned, "onError sees exactly what the caller is handed");

    rt.dispose();
});

test("an onError handler that throws does not escape the runtime", () => {
    const rt = new LuaRuntime({
        onError: () => {
            throw new Error("reporter is broken");
        },
    });

    // console.warn is silenced so the expected diagnostic is not test noise.
    const warn = console.warn;
    const warnings: unknown[][] = [];
    console.warn = (...args: unknown[]) => {
        warnings.push(args);
    };
    try {
        assert.match(expectError(rt.run("error('boom')", "bad"), "a failing chunk"), /boom/);
    } finally {
        console.warn = warn;
    }

    assert.equal(warnings.length, 1, "the broken reporter is surfaced, not swallowed");
    rt.dispose();
});

test("the sandbox is intact through the runtime", () => {
    const rt = new LuaRuntime();

    for (const name of [
        // never opened
        "io", "os", "package", "debug", "coroutine",
        // installed by luaopen_base, nil'd by createSandboxedState
        "load", "loadstring", "dofile", "loadfile", "require", "collectgarbage",
        "rawset", "rawget", "rawequal", "rawlen",
    ]) {
        expectOk(rt.run(`assert(${name} == nil)`, "sandbox"), `${name} must not be reachable`);
    }

    // The libraries that ARE open still work, so the sandbox is a deny list and
    // not a broken state.
    expectOk(rt.run("assert(string.rep('ab', 2) == 'abab')", "sandbox"), "string is open");
    expectOk(rt.run("assert(table.concat({'a', 'b'}, '-') == 'a-b')", "sandbox"), "table is open");
    expectOk(rt.run("assert(math.max(3, 7) == 7)", "sandbox"), "math is open");

    rt.dispose();
});

test("a non-positive instruction budget is rejected at construction", () => {
    assert.throws(() => new LuaRuntime({instructionBudget: 0}), RangeError);
    assert.throws(() => new LuaRuntime({instructionBudget: -1}), RangeError);
    assert.throws(() => new LuaRuntime({instructionBudget: Number.NaN}), RangeError);
    assert.ok(DEFAULT_INSTRUCTION_BUDGET > 0);
});

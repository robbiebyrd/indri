// Guards the Metro arrangement in `metro.config.js`.
//
// React Native defines a global `process`, so fengari's
// `typeof process !== "undefined"` guards are TRUE on device and it takes Node
// code paths against a `process` that is not Node's. Two rules follow, and both
// were found the hard way — the bundle resolved fine and then threw on import,
// on web and on the iOS emulator:
//
//   1. liolib/loslib/ldblib/loadlib are dropped from the bundle. They are the
//      source of every Node-only require, and liolib.js:134 reads
//      `process.versions.node` at module scope, which RN does not provide.
//   2. luaconf.js:60 CALLS `require('os').platform()` at module scope, so `os`
//      cannot be an empty object.
//
// This test reproduces that module environment in Node so a regression fails
// `pnpm test` instead of only showing up in a device run.
//
// What this does NOT prove: that Hermes executes the result. Only a device run
// proves that.
import test from "node:test";
import assert from "node:assert/strict";
import {createRequire} from "node:module";

const require_ = createRequire(import.meta.url);

/** Must mirror DROPPED_LUA_LIBS in metro.config.js. */
const DROPPED_LUA_LIBS = /^\.\/(liolib|loslib|ldblib|loadlib)\.js$/;

/** Must mirror SHIMMED_MODULES in metro.config.js. */
const SHIMMED: Readonly<Record<string, string | null>> = {
    fs: null,                    // null => empty object
    os: "../../shims/os.js",
    util: null,
};

const FENGARI_ORIGIN = /[/\\]fengari(-interop)?[/\\]/;

type ModuleWithLoad = {
    _load(request: string, parent: {filename?: string} | null, isMain: boolean): unknown
}

function withMetroModuleEnv<T>(fn: () => T): T {
    const mod = require_("node:module") as ModuleWithLoad;
    const original = mod._load;
    mod._load = function (request, parent, isMain) {
        if (FENGARI_ORIGIN.test(parent?.filename ?? "")) {
            if (DROPPED_LUA_LIBS.test(request)) return {};
            if (request in SHIMMED) {
                const shim = SHIMMED[request];
                return shim ? require_(shim) : {};
            }
        }
        return original.call(this, request, parent, isMain);
    };
    try {
        return fn();
    } finally {
        mod._load = original;
    }
}

test("fengari loads and runs Lua under the exact module set Metro provides", () => {
    const result = withMetroModuleEnv(() => {
        const {lauxlib, lua, lualib, to_luastring, to_jsstring} = require_("fengari");
        const L = lauxlib.luaL_newstate();
        for (const [name, openf] of [
            ["_G", lualib.luaopen_base],
            ["string", lualib.luaopen_string],
            ["table", lualib.luaopen_table],
            ["math", lualib.luaopen_math],
        ] as const) {
            lauxlib.luaL_requiref(L, to_luastring(name), openf, 1);
            lua.lua_pop(L, 1);
        }
        assert.equal(
            lauxlib.luaL_loadbuffer(
                L, to_luastring("return string.rep('q', 3) .. math.max(1, 2)"),
                null, to_luastring("=shimtest"),
            ),
            lua.LUA_OK,
        );
        assert.equal(lua.lua_pcall(L, 0, 1, 0), lua.LUA_OK);
        return to_jsstring(lua.lua_tostring(L, -1));
    });
    assert.equal(result, "qqq2");
});

test("io, os, debug and package libraries are absent from the module graph", () => {
    // The strongest form of the sandbox: these are not merely left unopened,
    // they are not in the bundle, so no later change to state.ts can open them.
    const openers = withMetroModuleEnv(() => {
        const {lualib} = require_("fengari");
        return {
            io: lualib.luaopen_io,
            os: lualib.luaopen_os,
            debug: lualib.luaopen_debug,
            pkg: lualib.luaopen_package,
        };
    });
    for (const [name, fn] of Object.entries(openers)) {
        assert.equal(typeof fn, "undefined", `luaopen_${name} must not be reachable`);
    }

    const loaded = Object.keys(require_.cache).filter((p) => p.includes("/fengari/src/"));
    for (const lib of ["liolib.js", "loslib.js", "ldblib.js", "loadlib.js"]) {
        assert.ok(
            !loaded.some((p) => p.endsWith(`/${lib}`)),
            `${lib} must not be in the module graph`,
        );
    }
});

test("the os shim satisfies luaconf's module-scope platform() call", () => {
    const os = require_("../../shims/os.js") as {platform: () => string};
    assert.equal(typeof os.platform, "function");
    assert.notEqual(os.platform(), "win32", "win32 would select the wrong LUA_PATH separator");
});

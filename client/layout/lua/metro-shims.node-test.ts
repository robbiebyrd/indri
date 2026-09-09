// Guards the Metro shim contract in `metro.config.js`.
//
// fengari references Node built-ins from code paths this app never executes,
// but TWO of them are CALLED at module scope and so cannot be empty objects:
//
//   luaconf.js:60   require('os').platform()
//   ldblib.js:474   require('readline-sync').setDefaultOptions(...)
//
// Both were found the hard way — the bundle resolved fine and then threw on
// import, on web and on the iOS emulator. This test reproduces the bundled
// module environment in Node so that regression fails `pnpm test` instead of
// only showing up in a device run.
//
// What this does NOT prove: that Hermes executes the result. Only a device run
// proves that.
import test from "node:test";
import assert from "node:assert/strict";
import {createRequire} from "node:module";

const require_ = createRequire(import.meta.url);

/** Must mirror NODE_ONLY_MODULES in metro.config.js. */
const BLOCKED = new Set([
    "fs", "os", "path", "child_process", "crypto", "util", "tmp", "readline-sync",
]);

/** Must mirror SHIMS in metro.config.js. */
const SHIMS: Readonly<Record<string, string>> = {
    "os": "../../shims/os.js",
    "readline-sync": "../../shims/readline-sync.js",
};

type ModuleWithLoad = {
    _load(request: string, parent: {filename?: string} | null, isMain: boolean): unknown
}

const FENGARI_ORIGIN = /[/\\]fengari(-interop)?[/\\]/;

test("fengari loads and runs Lua under the exact module set Metro provides", () => {
    const mod = require_("node:module") as ModuleWithLoad;
    const original = mod._load;

    mod._load = function (request, parent, isMain) {
        if (BLOCKED.has(request) && FENGARI_ORIGIN.test(parent?.filename ?? "")) {
            const shim = SHIMS[request];
            return shim ? require_(shim) : {};
        }
        return original.call(this, request, parent, isMain);
    };

    try {
        const {lauxlib, lua, lualib, to_luastring, to_jsstring} = require_("fengari");
        const L = lauxlib.luaL_newstate();
        lauxlib.luaL_requiref(L, to_luastring("_G"), lualib.luaopen_base, 1);
        lua.lua_pop(L, 1);
        lauxlib.luaL_requiref(L, to_luastring("string"), lualib.luaopen_string, 1);
        lua.lua_pop(L, 1);

        assert.equal(
            lauxlib.luaL_loadbuffer(L, to_luastring("return string.rep('z', 3)"), null, to_luastring("=shimtest")),
            lua.LUA_OK,
        );
        assert.equal(lua.lua_pcall(L, 0, 1, 0), lua.LUA_OK);
        assert.equal(to_jsstring(lua.lua_tostring(L, -1)), "zzz");
    } finally {
        mod._load = original;
    }
});

test("the os shim satisfies luaconf's module-scope platform() call", () => {
    const os = require_("../../shims/os.js") as {platform: () => string};
    assert.equal(typeof os.platform, "function");
    assert.notEqual(os.platform(), "win32", "win32 would select the wrong LUA_PATH separator");
});

test("the readline-sync shim satisfies ldblib's module-scope setDefaultOptions call", () => {
    const rl = require_("../../shims/readline-sync.js") as {setDefaultOptions: (o: object) => void};
    assert.equal(typeof rl.setDefaultOptions, "function");
    assert.doesNotThrow(() => rl.setDefaultOptions({prompt: "x"}));
});

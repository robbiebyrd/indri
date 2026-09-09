// Learn more: https://docs.expo.dev/guides/customizing-metro/
const {getDefaultConfig} = require('expo/metro-config');
const path = require('path');

const config = getDefaultConfig(__dirname);

const emptyModule = path.resolve(__dirname, 'shims/empty.js');
const osModule = path.resolve(__dirname, 'shims/os.js');

/**
 * Making fengari bundle and run under Metro/Hermes.
 *
 * fengari is a pure-JS Lua VM, which is why it can run under Hermes at all.
 * But React Native defines a global `process` (for `process.env`), so every
 * `typeof process !== "undefined"` guard inside fengari is TRUE on device and
 * it takes its Node code paths — against a `process` that is not Node's.
 *
 * Two separate problems follow, needing different fixes.
 *
 * 1. WHOLE LIBRARIES WE NEVER WANT. `liolib`, `loslib`, `ldblib` and `loadlib`
 *    implement Lua's io/os/debug/package. Game scripts must never reach any of
 *    them, and they are the source of every Node-only require (`fs`, `tmp`,
 *    `crypto`, `child_process`, `readline-sync`) plus the module-scope
 *    `process.versions.node` read at liolib.js:134 that throws on RN. They are
 *    required only by `lualib.js` and `linit.js`, and only to assign their
 *    `luaopen_*` functions — which we never call. So we drop them outright.
 *    `luaopen_io`/`luaopen_os`/`luaopen_debug`/`luaopen_package` become
 *    undefined, which is what we want: the sandbox is then enforced by the
 *    bundle rather than merely by policy.
 *
 * 2. WHAT THE SURVIVING MODULES STILL TOUCH. After the drop, only two external
 *    requires remain from code that actually loads: `fs` (lauxlib.js:901, used
 *    solely by luaL_loadfilex, which we never call) and `os` (luaconf.js:60,
 *    which CALLS `.platform()` at module scope — an empty object there is what
 *    produced "os.platform is not a function" on web).
 *
 * `resolver.extraNodeModules` does NOT work for this: it is a fallback,
 * consulted only when normal resolution fails, so really-installed packages
 * like `tmp` bypassed it entirely. `resolveRequest` overrides. It is scoped by
 * requesting module so nothing outside fengari is affected.
 *
 * `sprintf-js` is deliberately untouched: it is pure JS, lstrlib genuinely uses
 * it, and it bundles fine.
 *
 * `layout/lua/metro-shims.node-test.ts` reproduces this arrangement in Node, so
 * a regression here fails `pnpm test` rather than only a device run.
 */
const FENGARI_ORIGIN = /[/\\]node_modules[/\\](?:\.pnpm[/\\][^/\\]+[/\\]node_modules[/\\])?fengari(?:-interop)?[/\\]/;

/** Lua standard libraries dropped from the bundle entirely. See (1) above. */
const DROPPED_LUA_LIBS = /^\.\/(liolib|loslib|ldblib|loadlib)\.js$/;

/** External modules the surviving fengari code still references. See (2). */
const SHIMMED_MODULES = {
    fs: emptyModule,
    os: osModule,
    // fengari-interop's `require('util').inspect.custom`. Its own try/catch
    // tolerates a missing export, but Metro must still RESOLVE it, so this
    // entry is required the moment interop is imported (story 020).
    util: emptyModule,
};

const defaultResolveRequest = config.resolver.resolveRequest;

config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (FENGARI_ORIGIN.test(context.originModulePath ?? '')) {
        if (DROPPED_LUA_LIBS.test(moduleName)) {
            return {type: 'sourceFile', filePath: emptyModule};
        }
        const shim = SHIMMED_MODULES[moduleName];
        if (shim) {
            return {type: 'sourceFile', filePath: shim};
        }
    }
    const next = defaultResolveRequest ?? context.resolveRequest;
    return next(context, moduleName, platform);
};

module.exports = config;

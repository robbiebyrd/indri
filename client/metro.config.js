// Learn more: https://docs.expo.dev/guides/customizing-metro/
const {getDefaultConfig} = require('expo/metro-config');
const path = require('path');

const config = getDefaultConfig(__dirname);

const emptyModule = path.resolve(__dirname, 'shims/empty.js');
const osModule = path.resolve(__dirname, 'shims/os.js');
const readlineSyncModule = path.resolve(__dirname, 'shims/readline-sync.js');

/**
 * fengari is a pure-JS Lua VM, which is why it runs under Hermes at all. It
 * still statically references Node built-ins and two Node-only packages from
 * code paths this app never executes (luaL_loadfilex, the os/io libraries,
 * package/require). Metro resolves requires at bundle time regardless of the
 * `typeof process !== "undefined"` guards around them, so each needs somewhere
 * to point.
 *
 * `extraNodeModules` is NOT sufficient here: it is a fallback consulted only
 * when normal resolution fails. `tmp` and `readline-sync` are really installed
 * as fengari dependencies, so Metro resolves them for real and never consults
 * the fallback — which is how `tmp` ended up pulling `crypto` on iOS, and how
 * `tmp` called `.platform()` on an empty `os` stub on web. `resolveRequest`
 * overrides instead of falling back, so it catches both cases.
 *
 * The redirect is scoped by requesting module so it cannot affect any other
 * package that legitimately needs `path`, `util` or `crypto`.
 *
 * `sprintf-js` is deliberately absent: it is pure JS, lstrlib genuinely uses
 * it, and it bundles fine.
 */
const FENGARI_ORIGIN = /[/\\]node_modules[/\\](?:\.pnpm[/\\][^/\\]+[/\\]node_modules[/\\])?fengari(?:-interop)?[/\\]/;

const NODE_ONLY_MODULES = new Set([
    'fs',
    'os',
    'path',
    'child_process',
    'crypto',
    'util',
    'tmp',
    'readline-sync',
]);

// Two of these are CALLED at module scope, so an empty object is not enough:
// luaconf.js does os.platform(), ldblib.js does readlineSync.setDefaultOptions().
// Everything else is only ever *required*, never invoked at load time.
const SHIMS = {os: osModule, 'readline-sync': readlineSyncModule};

const defaultResolveRequest = config.resolver.resolveRequest;

config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (
        NODE_ONLY_MODULES.has(moduleName) &&
        FENGARI_ORIGIN.test(context.originModulePath ?? '')
    ) {
        // `os` needs a real platform() — see shims/os.js. Everything else
        // is only ever *required*, never called at module scope.
        return {type: 'sourceFile', filePath: SHIMS[moduleName] ?? emptyModule};
    }
    const next = defaultResolveRequest ?? context.resolveRequest;
    return next(context, moduleName, platform);
};

module.exports = config;

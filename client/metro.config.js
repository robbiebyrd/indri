// Learn more: https://docs.expo.dev/guides/customizing-metro/
const {getDefaultConfig} = require('expo/metro-config');
const path = require('path');

const config = getDefaultConfig(__dirname);

const empty = path.resolve(__dirname, 'shims/empty.js');

// fengari is a pure-JS Lua VM, which is why it works under Hermes at all — but
// it still statically references Node built-ins from code paths this app never
// executes. Metro resolves requires at bundle time regardless of the runtime
// guards around them, so each one needs somewhere to point.
//
// `sprintf-js` is deliberately NOT shimmed: it is pure JS, lstrlib genuinely
// uses it, and it bundles fine.
config.resolver.extraNodeModules = {
    ...config.resolver.extraNodeModules,
    fs: empty,
    os: empty,
    path: empty,
    child_process: empty,
    util: empty,
    tmp: empty,
    'readline-sync': empty,
};

module.exports = config;

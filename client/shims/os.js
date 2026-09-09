// Minimal `os` stand-in for fengari.
//
// fengari's luaconf.js calls `require('os').platform()` at MODULE SCOPE (line
// 60) purely to choose the path separator baked into LUA_PATH_DEFAULT. An empty
// object there throws "platform is not a function" before anything else runs —
// which is what broke both the web bundle and the iOS emulator.
//
// The returned value is irrelevant to us: `package`/`require` are never exposed
// to Lua, so LUA_PATH_DEFAULT is never read. It only has to be non-'win32' and
// callable.
module.exports = {
    platform: () => 'linux',
    EOL: '\n',
    constants: {},
    tmpdir: () => '/tmp',
};

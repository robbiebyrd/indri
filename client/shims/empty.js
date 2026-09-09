// Empty stand-in for Node built-ins and Node-only packages that fengari
// references behind `typeof process !== "undefined"` guards.
//
// React Native defines a global `process` (for process.env), so those guards
// are TRUE at runtime on device and fengari takes its Node branch. The code it
// then reaches for — luaL_loadfilex, the os/io libraries, package/require — is
// never called by this app, but Metro still has to RESOLVE the requires at
// bundle time. This module is what they resolve to.
//
// fengari-interop's `require('util').inspect.custom` is inside a try/catch, so
// resolving to {} there is safe too.
module.exports = {};

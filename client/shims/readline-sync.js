// Minimal `readline-sync` stand-in for fengari.
//
// fengari's ldblib.js calls setDefaultOptions() at MODULE SCOPE (line 474) to
// configure the interactive `lua_debug>` prompt used by Lua's debug.debug().
// The debug library is never opened for game scripts, so the prompt is
// unreachable — but the module-scope call still has to succeed for the bundle
// to load.
module.exports = {
    setDefaultOptions: () => {},
    prompt: () => '',
    question: () => '',
};

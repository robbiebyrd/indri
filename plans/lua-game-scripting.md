# Plan: Server-side Lua game scripting

**Created:** 2026-09-12 | **Status:** Draft | **Effort:** XL | **Branch:** `POC-00001/lua-game-scripting`

## Summary

Replace per-game Go handlers with sandboxed server-side Lua. A game becomes a directory of
`config.json` + `*.lua` run by the stock `indri` binary — no Go compilation, no custom `main.go`.
Scripts register action handlers, read and write authoritative game state inside the existing
lock + version-fence mutation path, emit deferred events and timers, and reach a small set of
permission-gated capabilities (HTTP, assets). Every built-in action stays in Go; scripts extend them
through the router's existing `received`/`processed` hook phases rather than replacing them.
`router.RegisterHandler` survives as the framework's Go escape hatch; `example/tictactoe`'s Go
handlers do not.

## Architecture Context

- Inbound: transport resolves session → `router.Dispatch(session, action, payload)` runs the
  `received` → `<action>` → `processed` phases (`internal/handlers/router/act.go:15`). Handlers are a
  flat `[]Handler`; several may share an action. `invokeHandler` already recovers panics.
- Handler contract is connection-free: `Handle(actions.Request{Session,Payload}) (actions.Result{Responses,Session,DisconnectIDs}, error)` (`internal/handlers/actions/handler.go`).
- Writes: `gameRepo.Store.Mutate(id, apply)` → `mutation.Run` acquires lock `"game:"+id`, loads,
  snapshots via `events.ToMap`, runs `apply`, CAS-saves on `Version`, retries ≤10, then
  `publishDiff` → `events.Diff` → `SanitizeDelta` → `Publisher` → `boot.monitorGameChanges` →
  broadcast. **`apply` is the only correct place for game-state edits.**
- `models.Game` JSON shape (`events.ToMap`) is what `events.Diff` paths and the client both speak.
  `Version` is `json:"-"` and `ID` is a `bson.ObjectID` — neither survives a JSON round trip.
- Existing client-side Lua (`client/layout/lua/`, fengari) is *presentation* scripting: `indri.send`,
  `indri.state()`, `indri.on()`, read-only proxied state. Server Lua mirrors that vocabulary but is
  authoritative. `layout.OpSetScript.Source` belongs to the client layer — do not overload it.
- Gap: nothing schedules deferred work anywhere in the repo; no Lua dependency exists in `go.mod`.

ADR: Lua edits state only inside `indri.mutate(gameId, fn)`, which is `Store.Mutate`'s `apply`
closure — it inherits the existing lock + version fence instead of adding a second concurrency
model. Alternative rejected: script returns an op/command list (cheaper diff, far worse authoring
ergonomics, and `events.Diff` already infers the ops).

ADR: there is no `indri.commit()` mid-script. A nested `Mutate` on the same key deadlocks
`lock.InProcess`'s keyed mutex. `indri.after(0, action, payload)` covers the use case.

## Research Findings

- `github.com/yuin/gopher-lua` v1.1.1, pure Go, Lua 5.1, active through Apr 2026. Chosen: no cgo
  (matches the plain-binary build), largest ecosystem, documented `SetContext` interruption.
- `L.SetContext(ctx)` interrupts between VM main-loop iterations only. gopher-lua issue #521: a
  single long library call (`string.rep`, `string.format("%999999999d")`) is **not** interruptible.
  Library-level size caps are therefore the real timeout, not a nicety.
- gopher-lua has **no heap cap** (issue #230). `RegistrySize`/`RegistryMaxSize`/`CallStackSize` bound
  the value and call stacks only. Mitigation here is input caps + marshalling size caps.
  `lua.Options.MaxCallStackSize` could not be verified — check the vendored `state.go` before use.
- No sandbox mode exists; dangerous globals must be nil'd after `OpenBase` (issue #27).
- `parse.Parse` + `lua.Compile` → `*lua.FunctionProto` is immutable and shareable across states;
  `L.NewFunctionFromProto(proto)` instantiates per state. `LState` is not goroutine-safe → pool.
- `L.PreloadModule(name, loader)` serves `require` from memory; `package.preload` is consulted before
  the filesystem searchers. There is no `PreloadString` (issue #119) — precompile to a proto.
- Nakama's `match_loop(ctx, dispatcher, tick, state, messages) -> state` returns whole state and does
  **not** diff — it is an in-memory live match with no per-tick persistence, so it is not a precedent
  for a Mongo + CAS document model. Colyseus tracks per-property dirty flags; Roblox replicates a live
  tree. None of them fit; the `apply`-closure placement does.
- Roblox switched signals from immediate to deferred dispatch and caps reentrancy depth at 10.
- `layeh.com/gopher-luar` and `gopher-json` are lightly maintained; hand-roll the converters.
- Scheduling: Mongo `findOneAndUpdate` lease is the only option that works without making Redis
  mandatory (`machinery`'s delayed tasks are Redis-broker-only; `cron`/`tasks` have no persistence).

## Security Considerations

- Trust tier is **operator-authored** (confirmed). In-process Lua is defense-in-depth, not a hard
  jail: a script can still exhaust heap. Do not later accept public uploads without re-planning.
- Strip after `OpenBase`: `dofile`, `loadfile`, `load`, `loadstring`, `collectgarbage`, `print`,
  `setmetatable`, `getmetatable`, `rawset`, `rawget`, `newproxy`. Never call `OpenIo`, `OpenOs`,
  `OpenDebug`. Clear `package.path`/`package.cpath`; nil `package.loadlib`.
- Reject binary chunks by construction (`load` is removed); if a load-from-string host function is
  ever added, reject a leading `\x1bLua` signature — gopher-lua does not verify hand-crafted bytecode.
- `Version` and `ID` must not be settable from Lua — a forged `Version` defeats the CAS fence.
- No session-binding capability is exposed to Lua at all. `register`, `login`, `logout`, `reconnect`,
  `kick`, `create`, `join` and `leave` stay in Go, so password hashing, token issuance and the
  `sessionId` connection key never sit behind the sandbox boundary. Scripts read `req.session` but
  cannot write one. Authorization resolves from the caller's own session, never a payload `userId`.
- Lua sees `privateData` (it is authoritative); sanitization stays on the outbound path unchanged.
- `indri.http` is SSRF-guarded via `net.Dialer.Control` (re-invoked per redirect hop, which is what
  closes the DNS-rebinding TOCTOU), text content-types only, `io.LimitReader` byte cap, hop cap.
- `indri.assets.read` resolves against a fixed root, rejects `..`/symlink escape, text extensions only.
- Script errors returned to clients are `models.WSError` + a correlation id; stack traces stay in logs.
  `IncludeGoStackTrace: false`.

## Performance Considerations

- Budget: p99 Lua invocation < 5 ms for a 50 KB game state. Fail the plan if exceeded; the fallback is
  a lazy read proxy over the state instead of an eager `LTable`.
- `SetContext` costs ~40% on a CPU-bound microbenchmark. Accepted — the alternative is no timeout.
- Per invocation this adds map→`LTable` and `LTable`→map versus today. `events.ToMap` + `events.Diff`
  already run per `Mutate` and are unchanged.
- Compile once at boot to `*FunctionProto`; pool `LState`s. Never `NewState` per request.
- Scheduler poll is one `findOneAndUpdate` per tick per instance; index `{fireAt, claimedUntil}`.

## Open Questions

### Critical (P1)
1. Do scripts live beside `config.json` on disk, or in Mongo per game? — **Default: on disk**, listed
   in a `scripts: []` key in `config.json`, resolved relative to it, compiled at boot. Mongo storage
   is what the future admin UI needs; it is out of scope here.

### Unresolved / waiting on signal
- Per-User permission-gated libraries need a User→grants model that does not exist yet, and an admin
  UI to edit it. Step 13 builds the capability-injection mechanism the grants would drive; the grant
  *source* stays a static per-script config list. Revisit when the admin UI is specced.
- `stage.Service.SetScript` writes `stage.scriptId`, a path with no field on `models.Stage`, and
  `stage.Service` is never wired into the injector. Dead/aspirational. Not fixed here — flagged.

## Steps

### Step 1: A hardened LState
- **Test:** `internal/services/lua/sandbox_test.go` — every stripped global is `LNil`; `os`/`io`/`debug`
  tables absent; a script that spins forever dies on the context deadline; `string.rep("x", 1e9)` and
  `string.format("%2000000000d", 1)` error instead of allocating.
- **Implement:** `internal/services/lua/sandbox.go` — `newState(cfg Config) *lua.LState`.
- **Code:**
```go
L := lua.NewState(lua.Options{SkipOpenLibs: true, RegistrySize: 1024 * 20,
    RegistryMaxSize: 1024 * 80, RegistryGrowStep: 32, CallStackSize: 256,
    MinimizeStackMemory: true, IncludeGoStackTrace: false})
for _, lib := range []struct{ n string; f lua.LGFunction }{
    {lua.LoadLibName, lua.OpenPackage}, {lua.BaseLibName, lua.OpenBase},
    {lua.TabLibName, lua.OpenTable}, {lua.StringLibName, lua.OpenString},
    {lua.MathLibName, lua.OpenMath},
} {
    L.Push(L.NewFunction(lib.f)); L.Push(lua.LString(lib.n))
    if err := L.PCall(1, 0, nil); err != nil { return nil, err }
}
for _, g := range unsafeGlobals { L.SetGlobal(g, lua.LNil) }
L.SetField(L.GetGlobal("package"), "path", lua.LString(""))
L.SetField(L.GetGlobal("package"), "cpath", lua.LString(""))
capStringLib(L, cfg.MaxStringBytes) // wrap rep/format; see Constraint
```
- **Constraint:** security — the deadline cannot interrupt one long library call (gopher-lua #521), so
  `capStringLib` is the timeout for those paths, not a convenience.
- **Validation:** `go test ./internal/services/lua/`

### Step 2: Compile cache and state pool
- **Test:** `internal/services/lua/pool_test.go` — 50 goroutines invoking concurrently never race
  (`-race`); a global assigned during one invocation is invisible to the next invocation on the same
  pooled state.
- **Implement:** `internal/services/lua/compile.go`, `pool.go`.
- **Code:**
```go
func compile(src, name string) (*lua.FunctionProto, error) {
    chunk, err := parse.Parse(strings.NewReader(src), name)
    if err != nil { return nil, err }
    return lua.Compile(chunk, name) // immutable, shared across states
}
// Per-invocation isolation: give the handler a fresh environment (Lua 5.1 setfenv)
env := L.NewTable()
L.SetMetatable(env, envReadonlyMeta(L.Get(lua.GlobalsIndex)))
L.SetFEnv(fn, env)
```
- **Depends on:** Step 1
- **Validation:** `go test -race ./internal/services/lua/`

### Step 3: Value marshalling with caps that error
- **Test:** `internal/services/lua/marshal_test.go` — table-driven round trip over nested maps, dense
  arrays, sparse/mixed-key tables, ints vs floats, `nil`, empty map vs empty array; depth > cap and
  size > cap return an error; a cyclic table returns an error; functions and userdata are rejected.
- **Implement:** `internal/services/lua/marshal.go` — `toLua(*lua.LState, any) (lua.LValue, error)`,
  `fromLua(lua.LValue, *budget) (any, error)`.
- **Code:**
```go
// Absence in the returned table means deletion, so truncation must never look
// like a delete: caps return an error, they never silently drop.
if b.depth > b.maxDepth { return nil, fmt.Errorf("state depth exceeds %d", b.maxDepth) }
if b.nodes++; b.nodes > b.maxNodes { return nil, fmt.Errorf("state exceeds %d nodes", b.maxNodes) }
```
- **Constraint:** correctness — dense `1..n` integer keys marshal to `[]interface{}`, anything else to
  `map[string]interface{}`, matching `gopher-json` semantics and what `events.Diff` compares.
- **Validation:** `go test ./internal/services/lua/`

### Step 4: Game ↔ Lua fidelity
- **Test:** `internal/services/lua/game_test.go` — a fully populated `models.Game` survives
  `gameToLua` → `applyLua` unchanged, **including `Version`, `ID`, `CreatedAt`, `DeletedAt`**;
  deleting a key in Lua removes it from the Go map; a script assigning `state.version = 999` or
  `state.id = "..."` cannot change the Go field.
- **Implement:** `internal/services/lua/game.go`.
- **Code:**
```go
func applyLua(g *models.Game, v lua.LValue, b *budget) error {
    m, err := fromLua(v, b); if err != nil { return err }
    raw, err := json.Marshal(m); if err != nil { return err }
    var next models.Game // zero value, so an absent key really deletes
    if err := json.Unmarshal(raw, &next); err != nil { return err }
    next.ID, next.Version = g.ID, g.Version // server-owned; not forgeable from Lua
    next.CreatedAt, next.DeletedAt = g.CreatedAt, g.DeletedAt
    *g = next
    return nil
}
```
- **Constraint:** security — `Version` is `json:"-"`; restoring it explicitly is what keeps the CAS
  fence intact. Unmarshalling into the *existing* `g` would merge maps and break deletion.
- **Depends on:** Step 3
- **Validation:** `go test ./internal/services/lua/`

### Step 5: Script loading and action registration
- **Test:** `internal/services/lua/engine_test.go` — a script calling `indri.on("move", fn)` twice for
  distinct actions yields two entries; registering the same action twice errors; a syntax error names
  the file and line; registration is refused outside load time.
- **Implement:** `internal/services/lua/engine.go`, and a `Scripts []string` field on `models.Script`
  resolved relative to the config file in `internal/repo/script/script.go` (return errors, do not
  `log.Fatalf`).
- **Code:**
```go
// Load runs each chunk once in a registration state; indri.on() records handlers.
type Engine struct {
    handlers map[string]handlerRef // action -> (proto, fn name)
    protos   map[string]*lua.FunctionProto
    pool     *statePool
}
func (e *Engine) Actions() []string { /* sorted keys, for router + transport wiring */ }
```
- **Depends on:** Step 2
- **Validation:** `go test ./internal/services/lua/`

### Step 6: The script handler, reachable through the router
- **Test:** `internal/services/boot/handlers_test.go` — extend with
  `TestRegisterHandlers_CoversEveryScriptAction`: every action in `Engine.Actions()` is reachable via
  `router.Dispatch`, mirroring the existing `...CoversEveryActionPackage` test and using
  `router.Reset()` + `t.Cleanup(router.Reset)`.
- **Implement:** `internal/handlers/actions/script/handler.go`; register in `boot.registerHandlers`
  after the built-ins, one `router.RegisterHandler("lua_"+a, a, ...)` per declared action; add
  `LuaEngine` to `injector.ServicesInjector`.
- **Code:**
```go
func (h *Handler) Handle(req actions.Request) (actions.Result, error) {
    ctx, cancel := context.WithTimeout(h.i.GlobalContext, h.cfg.Timeout)
    defer cancel()
    return h.i.LuaEngine.Invoke(ctx, h.action, req) // panics already recovered by invokeHandler
}
```
- **Depends on:** Step 5
- **Validation:** `go test ./internal/services/boot/ ./internal/handlers/...`

### Step 7: `indri.mutate` — the transaction bracket
- **Test:** `internal/services/lua/mutate_test.go` — `fn` returning nil performs no write; returning a
  deep-equal state performs no write; returning a changed state publishes exactly the changed paths;
  a nested `indri.mutate` on the same game returns a Lua error rather than deadlocking; a forced CAS
  conflict re-runs `fn` against the reloaded state.
- **Implement:** `internal/services/lua/host_mutate.go`.
- **Code:**
```go
err := h.i.GameRepo.Mutate(gameID, func(g *models.Game) error {
    ledger.resetAttempt()                    // Step 8: discard effects from the failed try
    before, err := events.ToMap(g); if err != nil { return err }
    ret, err := callLua(L, fn, toLua(L, before))
    if err != nil { return err }
    if ret == lua.LNil { return mutation.ErrAbort }
    if err := applyLua(g, ret, budget); err != nil { return err }
    after, err := events.ToMap(g); if err != nil { return err }
    if reflect.DeepEqual(before, after) { return mutation.ErrAbort }
    return nil
})
```
- **Constraint:** correctness — `apply` runs up to 10 times, so `fn` must be re-runnable. That is why
  every effect is buffered (Step 8) rather than performed inline.
- **Depends on:** Steps 4, 6
- **Validation:** `go test -race ./internal/services/lua/ ./internal/repo/game/`

### Step 8: The effect ledger
- **Test:** `internal/services/lua/effects_test.go` — an effect queued inside a `mutate` attempt that
  is retried is emitted exactly once; effects from an aborted or errored mutate are dropped; effects
  queued outside `mutate` flush when the handler returns; ordering is preserved across both levels.
- **Implement:** `internal/services/lua/effects.go`.
- **Code:**
```go
type ledger struct{ committed, attempt []effect } // two levels
func (l *ledger) queue(e effect)   { l.attempt = append(l.attempt, e) }
func (l *ledger) resetAttempt()    { l.attempt = l.attempt[:0] }
func (l *ledger) commitAttempt()   { l.committed = append(l.committed, l.attempt...); l.resetAttempt() }
```
- **Depends on:** Step 7
- **Validation:** `go test ./internal/services/lua/`

### Step 9: `indri.reply` and `indri.send`
- **Test:** `internal/services/lua/host_io_test.go` — `reply` lands in `Result.Responses` in order;
  `send` dispatches only after the handler returns; a handler that sends its own action terminates at
  the depth cap with a script error; fan-out beyond the per-event cap errors.
- **Implement:** `internal/services/lua/host_io.go`; a depth counter carried on `context.Context`.
- **Code:**
```go
const maxEventDepth, maxEventFanout = 10, 32 // Roblox's deferred-signal depth as the reference
if depthFrom(ctx) >= maxEventDepth {
    return luaError(L, "event depth %d exceeded", maxEventDepth)
}
// drained after the handler returns, never dispatched inline from inside it
```
- **Constraint:** correctness — inline dispatch of a script-emitted event is the single largest source
  of unbounded recursion in every event system surveyed. Queue then drain, always.
- **Depends on:** Step 8
- **Validation:** `go test ./internal/services/lua/`

### Step 10: The shared helper library, in Lua
- **Test:** `internal/services/lua/lib/lib_test.go` — Go tests driving each helper against fixture
  states: `current_scene`, `scene_data`, `team_of(player_id)`, `players_in_team`, `leader_by_score`
  (including the tie case), `each_team`.
- **Implement:** `internal/services/lua/lib/indri/game.lua` + `//go:embed`, served through
  `L.PreloadModule`; `require` resolves only from the in-memory registry.
- **Code:**
```lua
local M = {}
function M.current_scene(state)
  local stage = state.stage or {}
  return stage.currentScene, (stage.scenes or {})[stage.currentScene]
end
function M.team_of(state, player_id)
  for id, team in pairs(state.teams or {}) do
    for _, pid in ipairs(team.playerIds or {}) do if pid == player_id then return id, team end end
  end
end
```
- **Constraint:** security — `require("./anything")` and any absolute path must fail; `package.path`
  was cleared in Step 1, so `preload` is the only reachable searcher. Assert it in the test.
- **Depends on:** Step 5
- **Validation:** `go test ./internal/services/lua/...`

### Step 11: Durable schedule store
- **Test:** `internal/repo/schedule/schedule_test.go` — two concurrent claimers each get disjoint
  docs (no double-claim); a claim that expires is re-claimable; `Cancel(id)` removes an unfired entry;
  entries survive a store restart (new `Store` over the same collection still sees them).
- **Implement:** `internal/repo/schedule/{schedule.go,interface.go}` with
  `var _ Storer = (*Store)(nil)`, index on `{fireAt:1, claimedUntil:1}` + a TTL index on `firedAt`.
- **Code:**
```go
// Claim leases one due entry atomically — the same CAS idiom as saveWithVersion.
res := s.col.FindOneAndUpdate(ctx,
    bson.M{"fireAt": bson.M{"$lte": now}, "claimedUntil": bson.M{"$lt": now}},
    bson.M{"$set": bson.M{"claimedBy": s.instanceID, "claimedUntil": now.Add(leaseTTL)}},
    options.FindOneAndUpdate().SetSort(bson.D{{Key: "fireAt", Value: 1}}).SetReturnDocument(options.After))
```
- **Constraint:** infra — Mongo-backed, not Redis, so timers work in the default single-instance mode
  as well as under `INDRI_LOCK_BACKEND=redis`.
- **Validation:** `go test ./internal/repo/schedule/`

### Step 12: `indri.after` / `indri.at` / `indri.cancel`
- **Test:** `internal/services/scheduler/scheduler_test.go` — a scheduled action dispatches through
  `router.Dispatch` after `fireAt` with its payload and a nil session; `cancel` before `fireAt`
  prevents dispatch; a handler error marks the entry fired and does not hot-loop.
- **Implement:** `internal/services/scheduler/scheduler.go`, run as a third goroutine in
  `boot.Serve`'s errgroup; host functions queue through the Step 8 ledger.
- **Code:**
```lua
local id = indri.after(30, "turn_timeout", { player = pid })  -- relative
indri.at("2026-09-12T18:00:00Z", "round_start", {})           -- absolute
indri.cancel(id)                                              -- e.g. player moved in time
```
- **Constraint:** correctness — the fired handler reads state via `indri.mutate` at fire time. A state
  snapshot frozen at schedule time is deliberately *not* delivered; replaying it would lose every
  update written in between.
- **Depends on:** Steps 9, 11
- **Validation:** `go test -race ./internal/services/scheduler/`

### Step 13: Capability injection
- **Test:** `internal/services/lua/capability_test.go` — a script without the `http` grant sees
  `indri.http == nil`; grants are per-script, so one script's grant is invisible to another loaded in
  the same process; an ungranted call cannot be reached by walking `_G` or a shared metatable.
- **Implement:** `internal/services/lua/capability.go`; per-script `grants: []string` in `config.json`.
- **Code:**
```go
// Object-capability: absent means unreachable. Never a permission check inside a
// function that is always present — that makes "what can this script do" an audit
// of every function body instead of a glance at what was injected.
for _, g := range script.Grants { caps[g](L, tbl, scopedTo(script)) }
```
- **Depends on:** Step 6
- **Validation:** `go test ./internal/services/lua/`

### Step 14: `indri.http.get` and `indri.assets.read`
- **Test:** `internal/services/lua/host_http_test.go` — refuses loopback, RFC1918, link-local and
  unspecified IPs; refuses a redirect whose *second* hop resolves private; refuses
  `application/octet-stream`; truncating at the byte cap is an error not a short read; the per-call
  timeout fires. `host_assets_test.go` — `../` escape and symlink escape rejected, non-text extension
  rejected, byte cap enforced.
- **Implement:** `internal/services/lua/host_http.go`, `host_assets.go`.
- **Code:**
```go
dialer := &net.Dialer{Timeout: cfg.DialTimeout, Control: func(_, addr string, _ syscall.RawConn) error {
    host, _, err := net.SplitHostPort(addr); if err != nil { return err }
    ip := net.ParseIP(host)
    if ip == nil { return fmt.Errorf("refusing non-IP address %q", addr) }
    if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
        ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
        return fmt.Errorf("refusing disallowed IP %s", ip)
    }
    return nil
}}
// Control re-runs on every dial including each redirect hop: that is what closes
// the DNS-rebinding TOCTOU, not a pre-flight resolve-and-check.
body := io.LimitReader(resp.Body, cfg.MaxBytes+1)
```
- **Constraint:** security — `http` is **not** injected into action-handler context. Blocking network
  I/O inside `indri.mutate` holds the game lock and re-runs on every CAS retry. Grant it only to
  scheduled/lifecycle scripts, and assert that in the capability test.
- **Depends on:** Step 13
- **Validation:** `go test ./internal/services/lua/`

### Step 15: Port tic-tac-toe
- **Test:** `example/tictactoe/game_test.go` — every case in the deleted
  `server/handlers/move/handler_test.go` re-expressed as a golden test: fixture state + action →
  expected published delta. Covers turn enforcement, out-of-bounds, occupied cell, row/column/diagonal
  wins, draw.
- **Implement:** `example/tictactoe/game.lua`; delete `example/tictactoe/server/`; `main.go` collapses
  to `boot.Boot` + `boot.Serve` (or is deleted in favour of `cmd/server`).
- **Code:**
```lua
local game = require("indri.game")
indri.on("move", function(req)
  indri.mutate(req.session.gameId, function(state)
    local _, team = game.team_of(state, req.session.userId)
    if not team or not team.data.turn then return nil end -- nil = no-op
    local id, scene = game.current_scene(state)
    -- validate + place + score, then:
    return state
  end)
end)
```
- **Depends on:** Steps 10, 12
- **Validation:** `go test ./example/... && go vet ./...`

### Step 16: Lua hooks on built-in actions
- **Test:** `internal/services/lua/hooks_test.go` — a script registering `indri.before("join", fn)`
  runs before the Go `join` handler and after it for `indri.after_action("join", fn)`; the hook sees
  the action name and payload; a hook returning an error aborts the phase chain the same way a Go
  handler's error does; a hook that mutates state does so through `indri.mutate` like any other
  handler; hooks on an unknown action name are refused at load time.
- **Implement:** `internal/services/lua/hooks.go` — register one Lua handler under the literal
  `received` and `processed` actions, dispatching internally to the per-action hook table. No Go
  action package is deleted and no new session capability is added.
- **Code:**
```lua
-- Built-in actions stay in Go. A game extends them; it does not reimplement them.
indri.before("join", function(req)
  if req.payload.team == "spectators" then return indri.reject("spectators are closed") end
end)
indri.after_action("join", function(req)
  indri.mutate(req.session.gameId, function(state) state.data.lastJoin = req.session.userId; return state end)
end)
```
- **Constraint:** correctness — `router.Dispatch` runs `received` for *every* message, so the hook
  handler must return immediately when no script registered that action. Measure it: the no-hook path
  must not add measurable latency to actions that have no hooks.
- **Depends on:** Step 15
- **Validation:** `go test ./... && go vet ./...`

### Step 17: Transport parity and observability
- **Test:** extend `TestRestRoutesMatchRegisteredActions` to cover script-declared actions; add the
  equivalent assertion for GraphQL mutations (the gap CLAUDE.md notes nothing catches today).
  `internal/services/lua/metrics_test.go` — kill-reason counters increment separately for deadline,
  string cap, depth cap and script error.
- **Implement:** dynamic REST routes + GraphQL mutations for `Engine.Actions()`; per-action duration
  histogram; script errors as `models.WSError` carrying a correlation id, full detail logged only.
- **Code:**
```go
// One generic counter forces every consumer to re-derive root cause from logs.
luaKills.WithLabelValues(action, reasonDeadline).Inc()
```
- **Depends on:** Step 16
- **Validation:** `go test ./... && go test -race ./...`

### Step 18: Documentation
- **Implement:** `docs/SCRIPTING.md` (the `indri.*` surface, the state contract, the effect/retry
  rules, capability grants, how to test a script); update `docs/ARCHITECTURE.md`, `docs/PROTOCOL.md`,
  `README.md`, `CLAUDE.md` ("Adding a game action" becomes Lua-first), `.env.example` with
  `INDRI_LUA_TIMEOUT_MS`, `INDRI_LUA_MAX_STRING_BYTES`, `INDRI_LUA_MAX_STATE_DEPTH`,
  `INDRI_LUA_MAX_STATE_NODES`, `INDRI_LUA_EVENT_DEPTH`, `INDRI_LUA_HTTP_TIMEOUT_MS`,
  `INDRI_LUA_HTTP_MAX_BYTES`, `INDRI_SCHEDULER_POLL_MS`.
- **Depends on:** Step 17
- **Validation:** `go build ./... && go test ./...`

## Acceptance Criteria

- [ ] `example/tictactoe` contains no Go handler code and runs on the stock `cmd/server` binary.
- [ ] A Lua handler's state edits go through `Store.Mutate`, so a concurrent-write test proves no
      lost update, matching `TestAddPlayer_ConcurrentNoLostUpdates`.
- [ ] Returning nil or an unchanged state writes nothing and publishes nothing.
- [ ] Every `internal/handlers/actions/` package still exists and is still registered; a script can
      hook `join`/`create`/`leave` before and after without owning session binding.
- [ ] A script cannot read the filesystem, open a socket, load bytecode, reach `os`/`io`/`debug`, or
      change `Version`/`ID`.
- [ ] An infinite loop and a `string.rep` bomb are both killed, with distinct kill-reason metrics.
- [ ] Effects emitted during a retried mutation are delivered exactly once.
- [ ] A scheduled event fires after a server restart; a cancelled one never fires.
- [ ] `indri.http` is unreachable from an action handler and SSRF-guarded where it is reachable.
- [ ] p99 Lua invocation < 5 ms on a 50 KB state.
- [ ] All tests pass under `go test -race ./...`.

## Checklist (non-TDD cleanup)

- [ ] `go vet ./...` clean
- [ ] `gopher-lua` v1.1.1 added; verify `lua.Options.MaxCallStackSize` exists before using it
- [ ] `docs/PROTOCOL.md` lists script-declared actions on all three transports
- [ ] Dead `stage.Service.SetScript` / unwired `stage.Service` filed as a separate cleanup story

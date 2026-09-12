---
status: approved
approved_at: "2026-09-12T01:27:37.148Z"
updated: "2026-09-12T01:27:37.148Z"
---
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

**Scope boundary:** this removes the *recompile*, not the per-process cardinality. `boot.Boot` takes
one `-script` path and `i.Script` is a singleton, so a process still serves exactly one game type —
the same granularity as today. Multi-game hosting in one process is explicitly out of scope.

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

### Research Enhancement

- **Correction (framework):** `lua.Options` in v1.1.1 is exactly
  `{CallStackSize, RegistrySize, RegistryMaxSize, RegistryGrowStep, SkipOpenLibs, IncludeGoStackTrace, MinimizeStackMemory}`.
  **`MaxCallStackSize` does not exist** — do not reference it. `MinimizeStackMemory` does.
- **Correction (framework):** `*lua.LFunction` is **not portable across `LState`s**.
  `NewFunctionFromProto` closes over the creating state's env, and `indri.on("move", function() ... end)`
  registers an *anonymous closure with upvalues* that a `*FunctionProto` alone cannot reconstruct.
  Handler registration must therefore happen **per pooled state**, not once globally. See Step 5.
- **Correction (framework):** timeouts are not distinguishable by `ApiError.Type`. A context deadline
  surfaces as `RaiseError(ctx.Err().Error())` → an ordinary `ApiErrorRun`, identical to a script
  `error()`. Detect a timeout with `ctx.Err() != nil` after `PCall` returns, never by string match.
  Only `ApiErrorPanic` is reliably distinct (hard Go callstack overflow).
- **Confirmed (framework):** `lua.GlobalsIndex` (`-10002`), `LState.SetFEnv/GetFEnv`,
  `PreloadModule(name string, loader LGFunction)`, `SetFuncs`, `parse.Parse`, `lua.Compile`,
  `NewFunctionFromProto`, `CallByParam(lua.P{...})`, `LTable.Len/MaxN/ForEach` all exist as used.
  `OpenPackage` must be opened before `OpenBase` — `PreloadModule` raises if `package` is absent.
- **Correction (pattern):** the repo has **no metrics library, no `/metrics` endpoint, no
  instrumentation of any kind** — stdlib `log` only. Step 17's Prometheus snippet is wrong for this
  codebase. See Step 17.
- **Risk (pattern):** CI (`.github/workflows/ci.yml`) runs `go build`/`go vet`/`go test -race` on a
  plain runner with **no Mongo or Redis service containers**. `internal/repo/game/game_concurrency_test.go:20`
  `t.Skipf`s when Mongo is unreachable, so DB-dependent tests report green in CI **without ever
  running**. Any step whose test needs a game store must either follow `boot/handlers_test.go`'s
  DB-free bare-injector pattern or add a CI service container. Otherwise this plan ships untested.
- **Ref:** gopher-lua v1.1.1 source (`state.go:94`, `linit.go`, `auxlib.go:432`, `table.go:50`),
  repo-research agent, framework-docs agent.

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

### Research Enhancement

- **Accepted risk (P1) — in-process Lua does not bound heap.** The string caps stop `string.rep`
  bombs but not `while true do t[#t+1] = {} end`, table growth, concatenation, `gsub`, or module
  caches. gopher-lua has no allocator hook (issue #230), and `RegistryMaxSize` bounds only the value
  stack. **An accidental operator script can OOM the whole server.** At the operator-authored trust
  tier this is accepted, not solved — say so plainly in `docs/SCRIPTING.md` rather than implying a
  sandbox guarantee. Mitigate with `debug.SetMemoryLimit`, a container memory limit, and a source-size
  cap. If the trust tier ever moves to user-uploaded scripts, this design must move out of process.
- **Deliberate omission:** Lua cannot populate `Result.DisconnectIDs`. There is no
  `indri.disconnect()`. `kick` remains the only producer, and it resolves the target's session
  server-side after verifying the caller is host (`actions/kick/handler.go`). State this as an
  explicit non-capability alongside `indri.commit()` and session binding, so an implementer does not
  add it on seeing the struct field.
- **Forbidden hooks:** `login`, `register`, `reconnect`, `logout` are not hookable — `login`'s payload
  carries a plaintext password and `received` runs pre-auth. See Step 16.
- **Ref:** second opinion (gpt-5.6-sol) finding 9; gap analysis Gaps 3, 10.

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

### Step 0: Context and action name reach the handler (prerequisite)
- **Test:** `internal/handlers/router/act_test.go` — a handler receives the dispatched action name and
  a context; cancelling that context makes an in-flight `GameRepo.Mutate` return promptly instead of
  blocking on the lock; `lock.InProcess.Acquire` returns `ctx.Err()` when the context is cancelled
  **while already waiting**, not only before.
- **Implement:** add `Context context.Context` and `Action string` to `actions.Request`; thread it
  through `router.Dispatch`, `GameRepo.Mutate` (currently uses the boot-time `*s.ctx`,
  `internal/repo/game/game.go:241`) and `lock.Manager.Acquire`.
- **Code:**
```go
// inprocess.go blocks uncancellably today: ctx is checked once, then entry.mu.Lock()
// waits forever. A Lua deadline cannot unwind a handler stuck here.
acquired := make(chan struct{})
go func() { entry.mu.Lock(); close(acquired) }()
select {
case <-acquired:
    return &inProcessHandle{manager: m, key: key, entry: entry}, nil
case <-ctx.Done():
    go func() { <-acquired; m.release(key, entry) }() // hand off, do not leak the lock
    return nil, ctx.Err()
}
```
- **Constraint:** correctness — without this, a Lua invocation killed by its deadline leaves a
  goroutine blocked on a Mongo call or a game lock, and Step 16's hooks cannot see the action name.
  Every later step assumes it. Do this first.
- **Validation:** `go test -race ./internal/handlers/... ./internal/services/lock/ ./internal/repo/game/`

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
    return lua.Compile(chunk, name) // proto is immutable and shareable; LFunction is NOT
}
// Per-invocation isolation: fresh env table whose __index falls through to real globals.
env, mt := L.NewTable(), L.NewTable()
L.SetField(mt, "__index", L.Get(lua.GlobalsIndex))
L.SetMetatable(env, mt)
L.SetFEnv(fn, env) // fn must be NewFunctionFromProto'd on *this* L
```
- **Depends on:** Step 1
- **Validation:** `go test -race ./internal/services/lua/`

### Research Enhancement

- **Correction:** a state must be **discarded, not returned to the pool**, after a deadline kill,
  `ApiErrorPanic`, or stack overflow — the interrupted stack, context and any globals it touched are
  contaminated. Only clean returns go back in the pool. Add a test that a post-timeout state is not
  reused.
- **Edge case:** `package.loaded` and module tables are shared per state. A script mutating a required
  module's table leaks that mutation to the next invocation on that state. Freeze module tables with a
  `__newindex` that raises, and assert it.
- **Ref:** framework-docs agent (gopher-lua v1.1.1 `state.go:25`, `state.go:1866`), second opinion
  (gpt-5.6-sol) finding 8.

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
  `map[string]interface{}`, matching `gopher-json` semantics and what `events.Diff` compares. Use
  `LTable.Len()` plus an empty hash part to decide; `ForEach` walks, it does not classify.
- **Validation:** `go test ./internal/services/lua/`

### Research Enhancement

- **Security (P2):** `events.joinPath` builds delta paths by raw concatenation —
  `internal/services/events/delta.go:121` returns `prefix + "." + key`. A Lua key containing `.` makes
  a literal key indistinguishable from a nested path, so a client applies the delta to the wrong node.
  **Reject any key containing `.` or `$`, or starting with `$`, at the Lua→Go boundary** — the latter
  two are also Mongo field-name violations. Test each rejection.
- **Correctness:** `events.ToMap` round-trips through JSON, so every number becomes `float64`.
  Integers beyond 2^53 lose precision silently. Reject out-of-range integers at the boundary and
  document the numeric domain.
- **Edge case:** decide and test empty-table semantics. A Lua `{}` has `Len() == 0` and an empty hash
  part, so it is ambiguous between `[]` and `{}`; `events.Diff` compares them with
  `reflect.DeepEqual` and will see a spurious change if the choice is not stable.
- **Ref:** second opinion (gpt-5.6-sol) finding 10; `internal/services/events/delta.go:121-142`.

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

### Research Enhancement

- **Security (P1):** restoring only `ID`/`Version`/timestamps is **not enough**. A whole-document
  round trip also lets a script rewrite `Code`, `Private`, `Players` membership, `Team.PlayerIDs`,
  `Player.Host` and `Player.Connected` — desynchronising the game from `models.Session`'s independently
  stored `GameID`/`TeamID`/`UserID` and silently changing who is host. Add a **structural invariant
  check after `applyLua`**: reject the mutation (script error, no write) if the script changed the
  player-id set, any `PlayerIDs` membership, any `Host`/`Connected` flag, `Code`, or `Private`.
  Script-owned and freely writable: every `*Data` map, the whole `Stage`, and `Player.Score`.
- **Rationale:** the alternative the second opinion proposed — expose only a narrow `ScriptState` of
  stage plus data — was rejected because games legitimately need `Player.Score` and per-player data,
  and a narrow view would force a parallel typed command API for each. An invariant check is
  enforceable, testable and keeps `state.players[id].score = n` ergonomic. Membership changes stay in
  Go (`GameService.ConnectPlayer`/`RemovePlayer`), reached only via built-in actions.
- **Test additions:** a script that adds, removes or renames a player is rejected; a script that
  promotes itself to host is rejected; a script that sets `score` and scene data succeeds.
- **Correctness (P1) — the empty-collection hole.** Lua has one table type, so an empty Go slice and
  an empty Go map both marshal to `{}` and cannot be told apart on the way back. `Team.PlayerIDs`,
  `Stage.SceneOrder` and `Game.Players` have **no `omitempty`**, so this is reachable with ordinary
  state. Proven in both directions:
```
json: cannot unmarshal object into Go struct field team.playerIds of type []string
json: cannot unmarshal array into Go struct field game.players of type map[string]int
```
  No global choice in Step 3 can fix it — picking "empty means object" breaks `playerIds: []`, picking
  "empty means array" breaks `players: {}`, and a game can have both empty at once.

  **Decision: a reflect-guided coercion pass at the Game boundary, owned by this step.** Ambiguity only
  bites where the Go type is concrete, and exactly there the Go type supplies the answer. Walk
  `models.Game`'s reflect type alongside the decoded map and fix the two mismatches before
  `json.Unmarshal`:
```go
// Empty map where the field is a slice -> empty slice, and vice versa.
// Only applied where the target type is concrete; inside PublicData the target
// is interface{}, which accepts either, so nothing is coerced and nothing can break.
func coerceEmpty(v any, t reflect.Type) any {
    switch t.Kind() {
    case reflect.Slice:
        if m, ok := v.(map[string]any); ok && len(m) == 0 { return []any{} }
    case reflect.Map:
        if s, ok := v.([]any); ok && len(s) == 0 { return map[string]any{} }
    }
    return v // recurse into struct fields, slice elems and map values
}
```
  Alternatives rejected: adding `omitempty` to the models (changes the client-visible keyframe and
  delta shape to fix an internal marshalling problem); a metatable array-marker (only survives
  pass-through, so a script-created `{}` is still ambiguous); narrowing to a `ScriptState` (already
  rejected above, and would not fix `playerIds` anyway).
- **Test additions for the hole:** a game with an empty `Players` map **and** an empty `PlayerIDs`
  slice at the same time survives the round trip with both shapes intact; a script that clears
  `sceneOrder` yields `[]` not `{}`; a script that clears `players` yields `{}` not `[]`.
  Note the existing "fully populated game survives unchanged" criterion cannot catch this — a fully
  populated game has no empty collections.
- **Ref:** second opinion (gpt-5.6-sol) finding 3; `internal/models/game.go:19-27`,
  `internal/models/session.go:16-18`; `internal/models/team.go:5`, `internal/models/stage.go:5`
  (both lacking `omitempty`); story 048 implementation note.

### Step 5: Script loading and action registration
- **Test:** `internal/services/lua/engine_test.go` — a script calling `indri.on("move", fn)` twice for
  distinct actions yields two entries; registering the same action twice errors; a syntax error names
  the file and line; registration is refused outside load time.
- **Implement:** `internal/services/lua/engine.go`, and a `Scripts []string` field on `models.Script`
  resolved relative to the config file in `internal/repo/script/script.go` (return errors, do not
  `log.Fatalf`).
- **Code:**
```go
// A registered handler is an anonymous closure with upvalues; it cannot be moved
// between states. Each pooled state runs every chunk itself at construction and
// owns its own *lua.LFunction table. The Engine holds only protos + the manifest.
type Engine struct {
    protos   []*lua.FunctionProto // compiled once at boot, shared, immutable
    actions  []string             // manifest, collected on one throwaway state
    pool     *statePool           // each state: NewFunctionFromProto + run + collect
}
type luaState struct {
    L        *lua.LState
    handlers map[string]*lua.LFunction // this state's closures, never shared
}
```
- **Depends on:** Step 2
- **Validation:** `go test ./internal/services/lua/`

### Research Enhancement

- **Correction (P1) — this replaces the "(proto, fn name)" design in `**Implement:**` above.**
  `indri.on("move", function(req) ... end)` registers an anonymous closure that may capture upvalues.
  A `*FunctionProto` does not preserve upvalues, and a `*lua.LFunction` is bound to the `LState` that
  created it. Storing one handler map globally and calling it on a pooled state is wrong. Instead:
  every pooled state runs all chunks at construction and builds its own `handlers` map; the Engine
  keeps only the protos and the action-name manifest.
- **Edge case:** the manifest must be collected once at boot on a throwaway state and then **frozen**.
  If two states disagree about which actions exist (non-deterministic registration, e.g. registering
  under a key derived from `os.time`), fail boot loudly rather than serving an inconsistent router.
- **Best practice:** reserve and reject the action names `received` and `processed` plus every
  built-in action at load time, with a named error. Nothing stops a script shadowing `login` today.
- **Ref:** framework-docs agent (`function.go` `newLFunctionL`, `state.go:1627`); second opinion
  (gpt-5.6-sol) finding 8.

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

### Research Enhancement

- **Correction (P1):** `mutation.Run` returns plain `nil` for **both** an aborted attempt
  (`mutation.go:58-60`) and a committed write (`mutation.go:70-72`), so the caller cannot tell whether
  anything was written — and the effect ledger needs exactly that signal. Add a committed-reporting
  variant, e.g. `Store.MutateResult(ctx, id, apply) (committed bool, err error)`, threading the
  `committed` value `save` already computes back out. Without it Step 8 cannot work at all.
- **Edge case:** the Redis lock lease is 10s. A Lua invocation that outlives it can run concurrently
  with another instance's attempt on the same game. The version fence keeps the *write* safe, but both
  scripts will have run. Keep the Lua deadline well under the lease (default 100ms vs 10s) and assert
  that relationship at boot.
- **Test additions:** `apply` is called again with reloaded state after a forced CAS miss; a script
  that errors leaves the document untouched and publishes nothing.
- **Ref:** second opinion (gpt-5.6-sol) finding 1; `internal/services/mutation/mutation.go:57-72`.

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

### Research Enhancement

- **Correction (P1) — "exactly once" is not achievable and the test name must change.** Effects flush
  *after* the Mongo commit, so a crash in that window loses them; retrying the flush duplicates them.
  Mongo here is a standalone `mongod` (CLAUDE.md: "does not need to be a replica set"), so there are
  **no multi-document transactions** and a true state+outbox atomic write is impossible. Split the
  guarantee by effect kind and say so in the docs:
  - `reply` / `send` — **best-effort**. Losing them costs a player one frame; the client recovers on
    `refresh`, which returns a full keyframe. Acceptable.
  - `after` / `at` / `cancel` — **at-least-once, and this one matters**: a lost timer hangs a game
    forever. Write the schedule row inside the same `apply` closure as part of the game document where
    possible, or accept at-least-once and require an idempotency key per scheduled entry (Step 11).
- **Correction:** `commitAttempt` needs the `committed bool` that Step 7's enhancement adds to
  `Mutate`; it cannot infer commit from a `nil` error.
- **Test rename:** "emitted exactly once" → "emitted once per commit, and a duplicate delivery is
  absorbed by the idempotency key".
- **Ref:** second opinion (gpt-5.6-sol) finding 1; CLAUDE.md standalone-mongod constraint.

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

### Research Enhancement

- **Correction (P1) — which channel carries a script error.** `boot.handleClientMessage:130` only
  `log.Printf`s a dispatch error; **WebSocket silently drops it**. GraphQL
  (`resolvers/resolver.go:82`) and REST (`rest/rest.go:115`) both propagate the Go `error` to the
  caller. So `Invoke` must return script errors as a `models.WSError` packed into
  `Result.Responses` (the convention CLAUDE.md states), **not** as its Go `error` return — otherwise
  WS players see nothing while REST callers see a raw Go error string. Test all three transports.
- **Edge case:** REST and GraphQL surface only the *first* response (`rest.go:148`,
  `resolver.go:77`). A script calling `indri.reply` twice behaves differently per transport. Either
  cap `reply` at one call per invocation with an error on the second, or document the divergence.
- **Author visibility:** a logged stack trace with a correlation id is useless to a script author who
  cannot read server logs. Decide now: either the error response carries the script name and line for
  operator-authored scripts (acceptable at this trust tier), or Step 20's harness is the only
  debugging surface. Recommend the former, since authors are operators here.
- **Ref:** gap analysis Gap 2; `internal/services/boot/handlers.go:130-132`.

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

### Research Enhancement

- **Bug in the snippet (P1):** `{"claimedUntil": {"$lt": now}}` never matches a freshly inserted
  document, which has no `claimedUntil` at all. Initialise the field on insert (or add an
  `$or: [{claimedUntil: {$exists: false}}, ...]` branch). As written, nothing would ever fire.
- **Schema additions:** the document needs `gameId` (indexed), `scriptVersion`, `attempts`,
  `idempotencyKey`, and an explicit `state` of `pending | leased | done | dead`. Without a top-level
  `gameId` there is no way to cancel every timer for a game — only `indri.cancel(id)` one at a time.
- **Leak (P1):** the proposed TTL on `firedAt` only expires entries that *already fired*. A timer for
  an abandoned game is never claimed, never gets `firedAt`, and lives in Mongo forever. There is **no
  game deletion anywhere in the repo** — `internal/repo/game/interface.go` has no `Delete`, `models.Game`
  has no ended/status field, and `DeletedAt` is declared but never read or written. So there is no
  cascade trigger to hook. Add an absolute TTL on `fireAt` (fire time + a generous grace window) so
  orphans expire regardless, and a `CancelForGame(gameID)` used by `leave` when the last player goes.
- **Missing failure cases to model:** crash after dispatch but before marking done (re-fires — hence
  the idempotency key); lease expiry while a slow handler still runs (two instances dispatch
  concurrently — renew the lease during execution); cancel racing a claim; permanent failure marked
  `done` instead of `dead`, losing the signal; a persisted action name that no longer exists after a
  script upgrade (hence `scriptVersion` — dead-letter rather than silently no-op, because
  `router.Dispatch` succeeds quietly when nothing matches).
- **Delivery is at-least-once.** State it in the docs; do not imply otherwise.
- **Ref:** second opinion (gpt-5.6-sol) finding 4; gap analysis Gap 5.

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

### Research Enhancement

- **Contract gap (P1):** a timer fires with **no session**, so the payload must be fully
  self-contained. The step's own examples violate this — `indri.after(30, "turn_timeout", {player=pid})`
  carries no game id, yet Step 15's handlers reach state via `indri.mutate(req.session.gameId, ...)`.
  Fix: `indri.after` implicitly stamps the current game id onto the entry, and the fired request
  exposes it as `req.gameId` (always present, session or not). Document that `req.session` is nil for
  timer-fired actions and that every built-in Go handler rejects a nil session, so a scheduled action
  name must be a script action, never a built-in.
- **Interaction:** scheduled dispatch goes through the same `received`/`processed` phases as Step 16,
  so any hook that dereferences `req.session` faults on a timer fire. Test that case explicitly.
- **Ref:** gap analysis Gap 4; grep of `req.Session == nil` across `internal/handlers/actions/`.

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

### Research Enhancement

- **Validation gap:** an unknown grant name (`"htpp"`) must fail boot with a named error, not silently
  grant nothing. `config.json` has no validation of any kind today — `script.go`'s `NewStore` is a raw
  `json.Unmarshal`. Add a known-capability check and a `schemaVersion` field so a later format change
  is detectable.
- **Edge case:** a pre-existing `config.json` with no `scripts` key boots with zero registered actions
  — a working game silently becomes a shell with no move handler. Warn loudly at boot when the script
  list is empty, and say so in the migration note.
- **Edge case:** two files in `scripts: []` registering the same action must fail at load, not
  last-write-wins. Step 5's dedup test reads as within-one-file; extend it across files.
- **Ref:** gap analysis Gap 8; `internal/repo/script/script.go`.

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

### Research Enhancement

- **Dangling reference (P1):** this step gates `http` to "scheduled/**lifecycle** scripts", but no step
  in the plan defines a lifecycle event, and nothing in the repo has one — `models.Game` has no status,
  ended or phase field at all. **Step 19 now defines them; it must land before this step's grant scope
  is meaningful.** Until then, `http` is reachable only from scheduled callbacks.
- **Ref:** gap analysis Gap 1.

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

### Research Enhancement

- **Security (P1) — pre-auth hooks must be forbidden.** `router.Dispatch` runs the `received` phase
  for **every** message including `login` and `register`, and `login`'s payload carries a **plaintext
  password** (`actions/login/handler.go:27`). As specified, `indri.before("login", fn)` — or any
  generic `received` hook reading `req.payload` — hands that password to a script, breaking this
  plan's own promise that password handling never sits behind the sandbox boundary. **Reject hooks on
  `login`, `register`, `reconnect` and `logout` at load time**, and strip the payload for any hook
  firing on an unauthenticated request. Test both.
- **Correction:** `processed` is **not** an unconditional after-phase. `act.go:40-42` returns
  immediately when a handler errors, so a failed action never reaches `processed`. Name the two hook
  kinds honestly: `before` and `after-success`. There is no `after-always` without changing `Dispatch`.
- **Correction:** the hook cannot see the action name today — `act.go:28` constructs
  `actions.Request{Session, Payload}` only. This is what Step 0 adds; make the dependency explicit.
- **Atomicity:** a `before` hook that mutates state commits independently of the built-in action that
  follows. If `join` then fails, the hook's write stands. Recommend before-hooks be validation-only
  (return a rejection, perform no `indri.mutate`), and put state changes in after-success hooks.
- **Ref:** second opinion (gpt-5.6-sol) finding 6; gap analysis Gap 3; `internal/handlers/router/act.go:15-47`.

### Step 17: Transport parity and observability
- **Test:** extend `TestRestRoutesMatchRegisteredActions` to cover script-declared actions; add the
  equivalent assertion for GraphQL mutations (the gap CLAUDE.md notes nothing catches today).
  `internal/services/lua/metrics_test.go` — kill-reason counters increment separately for deadline,
  string cap, depth cap and script error.
- **Implement:** dynamic REST routes + GraphQL mutations for `Engine.Actions()`; per-action duration
  histogram; script errors as `models.WSError` carrying a correlation id, full detail logged only.
- **Code:**
```go
// No metrics library exists in this repo (stdlib log only). Keep counters in a
// plain struct behind expvar, and log each kill with its reason so the cause is
// never re-derived from a generic "script failed" line.
func classify(ctx context.Context, err error) string {
    if ctx.Err() != nil { return "deadline" } // NOT ApiError.Type — see Step 1 notes
    var ae *lua.ApiError
    if errors.As(err, &ae) && ae.Type == lua.ApiErrorPanic { return "panic" }
    return "script_error"
}
log.Printf("lua kill action=%q reason=%s dur=%s id=%s", action, reason, dur, corrID)
```
- **Depends on:** Step 16
- **Validation:** `go test ./... && go test -race ./...`

### Research Enhancement

- **Correction (P1) — the original Prometheus snippet was wrong for this repo.** `grep prometheus`
  over `go.mod` and the tree returns nothing: there is no metrics library, no `/metrics` endpoint in
  `internal/entrypoints/http/server.go`, and no instrumentation anywhere. CLAUDE.md: "Logging is
  stdlib `log` throughout." Either use `expvar` (stdlib, zero new dependency) or make adding
  `prometheus/client_golang` plus a `/metrics` route an explicit, separately-justified decision with
  its own line in the dependency checklist. Do not smuggle it in.
- **Correction (P1) — dynamic GraphQL mutations are impossible.** `schema.graphqls` is hand-written
  SDL and `generated/generated.go` is gqlgen build-time codegen implementing a fixed `MutationResolver`
  interface. Nothing can add a field at boot. Add **one static generic mutation** instead and validate
  the name against the engine's manifest:
  ```graphql
  scriptAction(name: String!, payload: JSON!): JSON
  ```
  This is a wire-contract change that needs a client update and a note in `docs/PROTOCOL.md`.
- **Correction — REST is a payload allowlist, not a passthrough.** `transport/rest/routes.go` maps
  each action to a builder that accepts only named keys, deliberately so "a caller cannot smuggle
  extra keys into a payload". A script action's payload shape is unknown to Go, so it needs a
  wildcard route — a materially weaker posture than every existing route. Document it in Step 18 and
  bound it (max keys, max depth, max bytes) rather than accepting arbitrary JSON.
- **Ref:** gap analysis Gaps 11, 12; second opinion (gpt-5.6-sol) finding 7.

### Step 18: Documentation
- **Implement:** `docs/SCRIPTING.md` (the `indri.*` surface, the state contract, the effect/retry
  rules, capability grants, how to test a script); update `docs/ARCHITECTURE.md`, `docs/PROTOCOL.md`,
  `README.md`, `CLAUDE.md` ("Adding a game action" becomes Lua-first), `.env.example` with
  `INDRI_LUA_TIMEOUT_MS`, `INDRI_LUA_MAX_STRING_BYTES`, `INDRI_LUA_MAX_STATE_DEPTH`,
  `INDRI_LUA_MAX_STATE_NODES`, `INDRI_LUA_EVENT_DEPTH`, `INDRI_LUA_HTTP_TIMEOUT_MS`,
  `INDRI_LUA_HTTP_MAX_BYTES`, `INDRI_SCHEDULER_POLL_MS`.
- **Depends on:** Step 17
- **Validation:** `go build ./... && go test ./...`

### Research Enhancement

- **Also document:** delivery guarantees per effect kind (Step 8), at-least-once timers (Step 11), the
  nil-session timer contract (Step 12), forbidden hooks on auth actions (Step 16), the generic
  `scriptAction` GraphQL mutation and the wildcard REST route (Step 17), reserved action names, and
  the accepted in-process OOM risk.
- **Also update:** `.air.toml` — `include_ext = ["go","tpl","tmpl","html"]` does not watch `.lua`, so
  editing a script under `air` today triggers nothing. See Step 21.
- **Ref:** gap analysis Gaps 6, 12.

### Step 19: Game lifecycle events
- **Test:** `internal/services/lua/lifecycle_test.go` — `indri.on("game:created"|"player:joined"|"player:left"|"scene:changed", fn)`
  fires with the game id and the relevant subject; a lifecycle handler can `indri.mutate`; a lifecycle
  error does not fail the originating built-in action; registering an unknown lifecycle name fails at
  load.
- **Implement:** `internal/services/lua/lifecycle.go`. Emit from the existing Go call sites
  (`GameService.New`, `ConnectPlayer`, `RemovePlayer`, and wherever `currentScene` changes) through the
  same deferred queue as `indri.send`, never inline.
- **Code:**
```lua
indri.on("player:left", function(ev)   -- ev.gameId, ev.userId; ev.session is nil
  indri.cancel_for_game(ev.gameId)     -- stop that game's turn timers (Step 11)
end)
```
- **Constraint:** correctness — lifecycle events carry no session, same contract as timers (Step 12).
  Emit after the originating mutation commits, so a handler never observes uncommitted state.
- **Depends on:** Step 12
- **Validation:** `go test -race ./internal/services/lua/`

### Step 20: A Lua-native test harness for script authors
- **Test:** `internal/services/lua/harness_test.go` — the harness runs a fixture directory
  (`state.json` + `event.json` + `expected.json`) with no Mongo, and reports a readable diff on
  mismatch; a deliberately failing fixture exits non-zero.
- **Implement:** `cmd/indri-script/main.go` — `indri-script test ./scripts` and
  `indri-script check ./config.json` (compile + manifest + grant validation, no server boot). Uses the
  in-memory game store from Step 21 so an author never needs a database.
- **Code:**
```
$ indri-script test ./example/tictactoe
  move/rejects-out-of-turn ....... ok
  move/diagonal-win .............. FAIL
    - stage.scenes.board.data.winner: want "x", got nil
```
- **Constraint:** the plan's premise is "no Go compilation" for game authors, yet every other step
  validates with `go test`. Without this step that premise is false.
- **Depends on:** Step 15
- **Validation:** `go test ./cmd/indri-script/ && go vet ./...`

### Step 21: Make the new tests actually run in CI
- **Test:** the suite from Steps 7, 11 and 12 runs and **fails** when the logic is broken, on a clean
  CI runner.
- **Implement:** either add `services: mongodb` to `.github/workflows/ci.yml`, or add an in-memory
  `game.Storer` + `schedule.Storer` fake and write those tests against it. Also add `lua` to
  `.air.toml`'s `include_ext`.
- **Code:**
```yaml
# ci.yml has no services: block today, so every Mongo-dependent test t.Skip()s
services:
  mongodb:
    image: mongo:7
    ports: ["27017:27017"]
```
- **Constraint:** blocking — `internal/repo/game/game_concurrency_test.go:20` `t.Skipf`s when Mongo is
  unreachable and CI has no database, so DB-dependent tests report **green without ever running**.
  Every stateful step in this plan would ship unverified. Prefer the in-memory fake: it matches
  `boot/handlers_test.go`'s bare-injector precedent and keeps CI dependency-free.
- **Depends on:** Step 11
- **Validation:** `go test ./...` on a runner with no local Mongo, confirming no unexpected skips.

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
- [ ] A script cannot change player membership, host/connected flags, the game code or its privacy.
- [ ] A Lua key containing `.` or `$`, or an integer beyond 2^53, is rejected at the boundary.
- [ ] Hooks on `login`/`register`/`reconnect`/`logout` are refused at load time.
- [ ] A script error reaches the player over WebSocket, GraphQL **and** REST.
- [ ] A timer for an abandoned game expires instead of living in Mongo forever.
- [ ] `indri-script test` runs a game's fixtures with no database and no Go toolchain knowledge.
- [ ] The new stateful tests fail when the logic is broken on a clean CI runner — no silent skips.
- [ ] All tests pass under `go test -race ./...`.

## Checklist (non-TDD cleanup)

- [ ] `go vet ./...` clean
- [ ] `gopher-lua` v1.1.1 added. `lua.Options.MaxCallStackSize` **does not exist** — do not use it
- [ ] Decide and record: `expvar` vs adding `prometheus/client_golang` + a `/metrics` route (Step 17)
- [ ] `.air.toml` `include_ext` includes `lua`
- [ ] `docs/PROTOCOL.md` lists script-declared actions on all three transports
- [ ] Dead `stage.Service.SetScript` / unwired `stage.Service` filed as a separate cleanup story

## Enrichment Summary

**Deepened:** 2026-09-12
**Gaps found:** 12 (spec-flow-analyzer) + 10 findings (second opinion), 4 overlapping
**Agents used:** spec-flow-analyzer, framework-docs-researcher, repo-research-researcher
**Second opinion:** yes — openai/gpt-5.6-sol. Verdict: "not implementation-ready" (accepted in part)
**Confidence:** high on the factual corrections (verified against v1.1.1 source and repo files);
the two rejections below are judgement calls

### Key Discoveries

- `*lua.LFunction` is not portable across `LState`s and `indri.on` registers an anonymous closure with
  upvalues, so handler registration must be per pooled state. Invalidated the original Step 5 design.
- `lua.Options.MaxCallStackSize` does not exist; timeouts are indistinguishable from script errors by
  `ApiError.Type` and must be detected via `ctx.Err()`.
- `mutation.Run` returns `nil` for both abort and commit, so the effect ledger cannot work without a
  committed-reporting `Mutate` variant.
- WebSocket silently drops handler errors (`boot/handlers.go:130`) while GraphQL and REST propagate
  them — script errors must travel in `Result.Responses`, not the Go `error`.
- `login`'s payload carries a plaintext password and `received` hooks run pre-auth.
- gqlgen's resolver interface is build-time codegen; per-script mutations are impossible.
- The repo has no metrics library, no game deletion, and CI has no database — three assumptions the
  original plan made silently.

### New Risks Identified

- **In-process OOM (P1, accepted):** no heap bound exists; an accidental allocation loop kills the
  server. Mitigated by container limits and honest documentation, not solved. Revisit if the trust
  tier changes.
- **Silent CI skips (P1, mitigated by Step 21):** DB-dependent tests `t.Skip` on a runner with no
  Mongo, so the plan's stateful tests would have reported green without running.
- **Orphaned timers (P1, mitigated by Step 11):** no game deletion exists anywhere, so nothing would
  ever have cancelled a timer for an abandoned game.
- **Uncancellable lock waits (P1, mitigated by Step 0):** `lock.InProcess.Acquire` blocks past its one
  `ctx.Err()` check, so a Lua deadline could not unwind a contended handler.
- **Membership corruption (P1, mitigated by Step 4):** whole-document round-trip let a script rewrite
  players, teams, host flags and privacy, desynchronising the game from live sessions.

### Second-opinion findings rejected, with reasons

- **"Expose only a narrow `ScriptState`"** — rejected. Games legitimately need `Player.Score` and
  per-player data; a narrow view forces a typed Go command per field. A post-apply structural
  invariant check (Step 4) is enforceable and testable while keeping the state ergonomic.
- **"Introduce per-game engine routing / `GameTypeID`"** — rejected as out of scope. One script per
  process is the existing architecture, not a regression this plan introduces. The useful half — a
  script version stamped on persisted timers — is adopted in Step 11.

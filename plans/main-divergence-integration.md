---
status: in_progress
approved_at: "2026-10-01T13:29:44.842Z"
updated: "2026-10-01T14:02:08.596Z"
started_at: "2026-10-01T14:02:08.596Z"
---
# Plan: Integrate the diverged local and remote `main`

**Created:** 2026-10-01 | **Status:** Draft (revised — trunk reversed) | **Effort:** XL | **Branch:** `POC-00003/main-integration`

## Summary

Local `main` and `origin/main` both descend from `26a633a` (2026-09-09) and neither has ever
seen the other: 105 local commits (75,724 insertions) against 201 remote commits (32,370
insertions). A trial merge produces 132 conflicts, 73 of them `add/add`. **Local `main` is the
trunk.** `origin/main`'s unique work — four storage backends, the player slot model, the compact
MessagePack delta, and the shared transport conformance suite — is ported onto it. Each
duplicated subsystem is decided explicitly rather than merged textually.

> **Revision note.** This plan originally made `origin/main` the trunk. That was reversed after
> verifying that `origin/main` never received the connection-independent dispatch refactor. The
> two sides diverged on *orthogonal* axes — remote is newer on storage, older on dispatch — so
> neither was strictly ahead. See Research Findings.

## Architecture Context

- Both sides kept the base shape: `boot.Boot` → three-layer injector → handlers over repos,
  deltas from `events.Diff`, fanned out on a publisher.
- **Local owns the dispatch model and everything above it.** `actions.Request`/`actions.Result`
  (`internal/handlers/actions/handler.go`) make handlers connection-independent; the Lua engine,
  gqlgen GraphQL, REST, SSE and WebRTC all depend on it. 50 local files reference `actions.Request`.
- **Remote owns the storage floor and the wire format.** Four backends behind `Storer`, string
  IDs, pre-declared player slots (`AssignSlot`, `SlotID` on the session, slot-keyed `Players`),
  positional path encoding and MessagePack framing.
- The port is therefore *downward*: storage and wire format slide underneath a dispatch layer
  that does not change. That is the inverse of the original plan and is why it is cheaper.
- `internal/services/mutation` and `internal/services/lock` are shared from the base; local added
  `RunResult`, remote changed neither. No work.

## Research Findings

- **The dispatch divergence is the decisive fact.** `origin/main`:
  `Handle(s transport.Conn, decodedMsg map[string]interface{}) error`. `main`:
  `Handle(Request) (Result, error)`. Local refactored in `d03e6b0`, its first post-base commit
  (16 files, +366/−393, "GraphQL groundwork"). 20 trunk handler files are conn-coupled.
- Local's game `Storer` already carries `Mutate(ctx, …)` and `MutateResult(ctx, …)` and already
  has a `MemoryStore`. The storage port is *additive backends*, not an interface rewrite.
- **Local's layout is the stricter security boundary, in both languages.** Verified directly:
  `origin/main`'s `applyAddWidget` is `widgets[op.WidgetID] = op.Widget` — an unconditional
  overwrite where local returns `errExists`; `origin/main`'s TS `SceneSchema` and
  `GameLayoutSchema` have no `.strict()` where local has it on both; `origin/main`'s Go validator
  has no `checkKeys` equivalent, does not require `type`/`placement`/`kind`, has no
  percentage-format check, and rejects a legitimate numeric `z` on an absolute placement.
- The Go and TS layout constants (`8`/`4096`, depth `4`) and the AABB predicate are **textually
  identical within each ref**. But `origin/main`'s two halves have drifted from each other in
  three places, so adopting its layout would mean rewriting `validate.go` to restore parity.
  Adopting local's requires no Go-side work.
- `origin/main` **does** have a Lua layer — `internal/handlers/luahandler` (8 files),
  `gopher-lua v1.1.2`, wired at `boot.go:54`. It registers scripts *on* built-in action names;
  local forbids that and uses `indri.before`/`indri.after_action`. One analysis claimed the trunk
  had no Lua at all — that was wrong and its downstream conclusions are void.
- `origin/main` has **neither** parity test (`TestRestRoutesMatchRegisteredActions`,
  `TestGraphQLMutationsMatchRegisteredActions` — zero hits on that ref). Local has both. Keeping
  local as trunk preserves them.
- `origin/main`'s `OriginChecker` **accepts every origin when no allowlist is set**
  (`internal/transport/origin.go:12-21`, commits `512079a`/`de4a4d4`/`e03ca3b`); local's
  `OriginPolicy` fails closed. This is a deliberate, documented posture change on remote.
- Remote's delta path splitting is plain `.split('.')`. Local's `splitDeltaPath` handles
  backslash-escaped `.` and `\` inside document keys. Adopting positional encoding wholesale
  would lose that.
- Remote's `events.Diff` still returns dotted paths (`delta.go:14`); positional encoding is
  applied downstream by `EncodePath` (`positional.go:38`). Remote also added `diffSlice`
  (`delta.go:53`), so arrays diff per index.

## Security Considerations

- Keeping local's layout validator keeps unknown-key rejection, required `type`/`placement`/`kind`,
  percentage-format checks and the `privateData`-at-any-depth scan. Dropping to remote's would
  silently remove all four.
- **The origin policy is a live decision, not a carry.** Local fails closed; the trunk's own docs
  say an open default "can still reach a localhost or private-network server through a visitor's
  browser." Default to local's closed policy and treat remote's as an opt-in.
- The slot port touches authorisation. `kick` and the layout action must keep resolving the
  caller from their *own* connection's `sessionId`, never a client-supplied id, under slots.
- `SanitizeDelta` and `GameService.Sanitize` must stay in agreement. If the compact delta is
  adopted (Step 6), its pair-array `SanitizeDelta` has to strip exactly what the map form stripped.

## Performance Considerations

- The compact delta is the only performance-motivated port. Weigh MessagePack payload savings
  against losing escaped-key path support and rewriting every `scripttest` fixture and the client
  parser. Re-baseline `client/services/game-state-parser.bench.node-test.ts` either way.
- Postgres and SQLite stores must keep the version-CAS fence; remote already fences
  `Update`/`UpdateField`/`DeleteField` (`26b9688`) and retries on drift (`046a524`). Port both.
- The Lua engine's 100 ms budget, compile cache and state pool are untouched by this work.

## Reference Mapping

| Source concept | Source location (`origin/main`) | Target (trunk = local `main`) |
|---|---|---|
| sqlite/postgres clients | `internal/clients/{sqlite,postgres}` | same paths, new |
| id helper + sentinels | `internal/repo/ids`, `internal/repo/errors.go` | same paths, new |
| shared change publisher | `internal/repo/game/changes.go` | merge into local's `core.go` |
| game backends | `game/{sqlite,postgres}*.go` | same paths, against local's `Storer` |
| user/session backends | `repo/{user,session}/{memory,sqlite,postgres}.go` | same paths, new |
| player slots | `AssignSlot`, `models.Session.SlotID` | local `repo/game/player.go`, `models/session.go` |
| compact delta | `events/{positional,clientview,wire}.go` | decision, Step 6 |
| conformance suite | `internal/transport/transporttest/` | same path; run against local's 5 transports |
| real-store handler tests | `internal/handlers/handlertest/` | same path, new |
| `indri.refresh`/`refreshSelf` | `internal/handlers/luahandler/` | harvested into `internal/services/lua` |

## Open Questions

### Critical (P1 - Blockers)

1. **Is local `main` green?** Step 1 answers it. The working tree currently has one uncommitted
   story move (`stories/066-…` deleted, re-added under `archive/`) that must be settled first.
2. **Adopt the compact delta at all?** It costs the client parser, every `scripttest` fixture and
   escaped-key path support, and buys payload size. Default if unanswered: **do not port it** in
   this integration; file it as its own plan. Local's dotted-path delta already works end to end.

### Important (P2 - Affects implementation)

3. Does the slot model replace user-keyed membership outright, or coexist? Remote deleted the
   team helpers (`AddPlayerToTeam`, `ChangePlayerTeam`, `HasPlayerOnTeam`, `PlayerOnWhichTeam`)
   that local's handlers and Lua `checkInvariants` still call. Default: port slots as the
   membership model and migrate those call sites — half-and-half is the worst outcome.
4. Keep remote's `client/packages/protocol-client` packaging? Local's failover supervisor is the
   load-bearing part and remote has no failover at all. Default: keep local's; harvest remote's
   MessagePack codec only if Q2 is answered yes.
5. Eleven archived stories (`#516d`, `#254f`, `#c6b2`, `#dd9e`, `#ed7a`, `#2b59`, `#5e72`,
   `#9d66`, `#d50f`, `#8ef7`, `#e28e`) were completed on both sides by different commits. Default:
   keep the trunk's copy and note the duplicate delivery.

### Unresolved / waiting on signal

- Whether `example/tictactoe/game.lua` needs changes under slots. It authorises on
  `session.teamId`, which slots preserve — but this was reasoned, not executed. Revisit when
  Step 4 lands; the fixture suite is the signal.

## Steps

### Step 1: Verified baseline on an integration branch off local `main`
- **Implement:** settle the uncommitted story move, branch, record the baseline.
- **Code:**
  ```bash
  git status                      # settle the stories/066-* move first
  git switch -c POC-00003/main-integration main
  go build ./... && go vet ./... && go test -race ./... 2>&1 | tee .integration/baseline.txt
  (cd client && rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test)
  ```
- **Constraint:** clean `--frozen-lockfile` install — a warm `node_modules` hides undeclared deps.
- **Validation:** `baseline.txt` committed; every later step is judged against it.

### Step 2: Port the SQLite and Postgres clients and the repo primitives
- **Test:** `internal/repo/ids/ids_test.go`; sentinel errors asserted by the contract suites.
- **Implement:** `internal/clients/{sqlite,postgres}`, `internal/repo/ids`, `internal/repo/errors.go`.
- **Code:**
  ```go
  // internal/repo/errors.go — remote's sentinels, which the contract suites assert on.
  var (
      ErrNotFound  = errors.New("repo: not found")
      ErrDuplicate = errors.New("repo: duplicate")
      ErrConflict  = errors.New("repo: version conflict")
  )
  ```
- **Constraint:** SQLite needs a single connection so `:memory:` DBs and per-connection pragmas
  are shared (`2579bab`); Postgres needs one capped pool closed on shutdown (`ec04cc0`).
- **Depends on:** Step 1
- **Validation:** `go build ./... && go test ./internal/repo/ids/`

### Step 3: Add SQLite and Postgres game backends against local's `Storer`
- **Test:** `internal/repo/game/store_contract_test.go` — the existing shared suite, extended to
  run against all four backends including `MutateResult`'s committed flag.
- **Implement:** `game/{sqlite,postgres}{,_host,_player}.go`, plus the shared change publisher.
- **Code:**
  ```go
  // Local's Storer already has what the engine needs — the backends implement it as-is:
  //   Mutate(ctx, id, apply) error
  //   MutateResult(ctx, id, apply) (committed bool, err error)
  var (
      _ Storer = (*Store)(nil)        // mongo
      _ Storer = (*MemoryStore)(nil)
      _ Storer = (*SQLiteStore)(nil)
      _ Storer = (*PostgresStore)(nil)
  )
  ```
- **Constraint:** keep the version CAS on `Update`/`UpdateField`/`DeleteField` (`26b9688`) and
  retry on version drift rather than returning `ErrConflict` (`046a524`).
- **Depends on:** Step 2
- **Validation:** `go test ./internal/repo/game/`

### Step 4: Port the player slot model
- **Test:** `internal/repo/game/slot_contract_test.go` — one slot per player; only a slot's holder
  may leave or disconnect it; assignment is team-aware.
- **Implement:** `models.Session.SlotID`, slot-keyed `Game.Players`, `AssignSlot`, and the
  `join`/`leave`/`kick`/`inquire`/`layout` call sites.
- **Code:**
  ```go
  // Replaces user-keyed membership. The team helpers local still calls
  // (AddPlayerToTeam, ChangePlayerTeam, HasPlayerOnTeam, PlayerOnWhichTeam)
  // are migrated to slots in this step — see Open Question 3.
  AssignSlot(id, teamId, userId, displayName string) (slotId string, err error)
  ```
- **Constraint:** authorisation still resolves the caller from their own connection's `sessionId`.
  `kick` is the reference implementation and must keep working under slots.
- **Constraint:** local's Lua `checkInvariants` refuses membership and host changes from scripts —
  it must keep refusing them when membership becomes slot-shaped.
- **Depends on:** Step 3
- **Validation:** `go test -race ./internal/repo/game/ ./internal/handlers/actions/...`

### Step 5: Port user and session backends
- **Test:** `repo/{user,session}/store_contract_test.go` across mongo, memory, sqlite, postgres.
- **Implement:** `Storer` interfaces plus memory/sqlite/postgres for both repos.
- **Code:**
  ```go
  // Behaviours remote had to fix after shipping — port them with the stores, not after:
  //   2527068  sessions expire after sessionMaxAge (postgres)
  //   6b8bdd4  ...and on sqlite and memory
  //   f5870ec  Update returns ErrNotFound when no row was updated
  //   33bf874  moving a session onto a taken user reports repo.ErrDuplicate
  //   284d617  password survives an Update that omits it
  ```
- **Constraint:** sessions stay one-per-user — the unique index on `userId` is load-bearing.
- **Depends on:** Step 4
- **Validation:** `go test ./internal/repo/user/ ./internal/repo/session/`

### Step 6: Decide the compact delta protocol — **decision point**
- **Implement:** either port `events/{positional,clientview,wire}.go` + MessagePack framing +
  layout frames + keyframe-on-shape-change, or decline and keep local's dotted-path delta.
- **Constraint:** porting costs the client parser, every `scripttest` fixture, and local's
  `splitDeltaPath` escaped-key support (remote splits on a plain `.`). It buys payload size and
  is the only reason remote needed the `OpLayout` layout-frame channel at all.
- **Constraint:** if ported, `SanitizeDelta`'s pair-array form must strip exactly what the map
  form stripped, or it disagrees with `GameService.Sanitize`.
- **Depends on:** Step 3
- **Validation:** `go test ./internal/services/events/` and the client parser suite; re-baseline
  `game-state-parser.bench.node-test.ts` either way.

### Step 7: Adopt the transport conformance suite and the real-store handler harness
- **Test:** `internal/transport/transporttest/` run against all five local transports — ws,
  graphql, sse, rest, webrtc.
- **Implement:** `transporttest`, plus `internal/handlers/handlertest` for real-store handler tests.
- **Code:**
  ```go
  // One suite, every transport — replaces five hand-rolled near-duplicates.
  func TestConformance(t *testing.T) { transporttest.Run(t, newTransportUnderTest) }
  ```
- **Constraint:** both parity tests must keep failing when an action lacks a route or a mutation —
  they are local-only and are the guard remote never had.
- **Depends on:** Step 1
- **Validation:** `go test ./internal/transport/...`

### Step 8: Harvest the `luahandler` behaviours into the engine
- **Test:** `internal/services/lua/hooks_test.go` — `indri.refresh()`, `indri.refreshSelf()` and
  post-`Mutate` execution ordering, reached through `indri.before`/`indri.after_action`.
- **Implement:** add the behaviours to `internal/services/lua`; nothing from
  `internal/handlers/luahandler` is copied, because its registration model is the one local forbids.
- **Code:**
  ```go
  // Remote registered scripts ON built-in names — the model local rejects:
  //   router.RegisterHandler("script:"+action, action, New(i, script))
  // Engine model: script actions are "lua_"+action and never claim a built-in.
  ```
- **Depends on:** Step 4
- **Validation:** `go test ./internal/services/lua/`

### Step 9: Settle the origin policy and harvest the remaining trunk fixes
- **Test:** `internal/transport/origin_test.go` — closed-by-default stays closed.
- **Implement:** keep local's fail-closed `OriginPolicy`; port the trunk's genuinely useful odds
  and ends — `?debug=1` JSON selection, CLI flags for every setting (`995b144`), pinned
  pnpm/node/go versions (`6650363`), and the empty-game-list Join screen fix (`21113cb`).
- **Constraint:** remote's open-by-default origin posture is **not** adopted silently. If it is
  wanted, it is an explicit opt-in with its own documentation.
- **Depends on:** Step 7
- **Validation:** `go test ./internal/transport/ && go build ./...`

### Step 10: Reconcile docs, config and duplicated story history
- **Implement:** `CLAUDE.md`, `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md`,
  `docs/PROTOCOL.md`, `.env.example`, `.github/workflows/ci.yml`, and the 11 archived stories
  completed twice.
- **Constraint:** CLAUDE.md must describe the integrated system — four backends, slots, and
  whichever delta format Step 6 chose.
- **Depends on:** Step 9
- **Validation:** full suite against `baseline.txt`; clean client install.

## Acceptance Criteria

- [ ] `POC-00003/main-integration` builds, vets and passes `go test -race ./...` with no
      regression against the Step 1 baseline
- [ ] Clean `pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test` passes in `client/`
- [ ] `go run ./cmd/indri-script test ./example/tictactoe` is green
- [ ] All four backends pass the shared contract suites for game, user and session
- [ ] `transporttest` runs against all five transports
- [ ] Both parity tests still pass, and still fail when an action lacks a route or mutation
- [ ] The origin policy is closed by default
- [ ] Every decision point records what was dropped and why
- [ ] CLAUDE.md describes the integrated architecture

## Checklist (non-TDD cleanup)

- [ ] `gofmt` clean across the branch
- [ ] Duplicated `stories/archive/` entries deduplicated, duplicate delivery noted in worklogs
- [ ] All commits GPG-signed; verify with `git log --show-signature`
- [ ] No merge performed on `main` itself until the branch is accepted

# Handlers / services slice review

Scope: `internal/handlers/**`, `internal/services/{authentication,session,user,game,stage,connection,broadcast,utils}`,
`internal/utils/**`, `internal/types/**`, `internal/models/**`. Tree: `wip/codebase-review` (6330d92, based on main 3eab208).

Tooling: `go vet` on every package in scope is clean. `go test -race` passes on every package that has tests
(kick, layout, luahandler, services/game, services/broadcast). The other 20 packages in scope have **no test files** (see HS-19).
(The Go build cache is blocked by the sandbox, so the runs used `GOCACHE=$TMPDIR/gocache`.)

Three findings were reproduced by throwaway tests in a scratch copy of the repo, not in the repo itself:
`scratchpad/repro/internal/handlers/actions/kick/repro_test.go` (HS-1) and
`scratchpad/repro/internal/handlers/luahandler/repro_test.go` (HS-4, HS-10).

## Prior findings re-check

| Prior | Finding | Status | Evidence |
|---|---|---|---|
| Sep 9 / 6760eda | create: inverted `private` type assertion | FIXED | `internal/handlers/actions/create/handler.go:36` uses the comma-ok form |
| Sep 9 / 6760eda | infinite loop on game-code collision | CHANGED | `internal/services/game/game.go:270` is still an unbounded `for {}`, but an `Exists` error now breaks out of it. It is also unreachable from `create`, which requires a non-empty `code` (`handlers/utils/keys.go:17`) |
| Sep 9 / 6760eda | phantom games from Get/GetByCode on repo error | FIXED | `game.go:88-104` returns the error |
| Sep 9 / 6760eda | malformed login-redirect JSON | FIXED | `create/handler.go:40`, `join/handler.go:38`, `inquire/handler.go:48` are valid JSON |
| Sep 9 / 6760eda | debug `fmt.Println` | FIXED | no `fmt.Println` left in scope |
| Sep 9 / 56d1a6f | enumerable ObjectID session tokens | FIXED | `internal/services/utils/token.go:12` (256-bit `crypto/rand`) |
| Sep 9 / 56d1a6f | reconnect inverted GameID condition | FIXED | `reconnect/handler.go:71` |
| Sep 9 / 56d1a6f | kick authorized the target, not the caller | FIXED, but see HS-1 | `kick/handler.go:38` → `handlers/utils/host.go:16`. The caller is resolved from their own connection. However, host status is read from whoever *now* sits in the session's recorded slot, so a stale session still passes |
| Sep 9 / 56d1a6f | RemovePlayer called with a code | FIXED | `kick/handler.go:62` passes `g.ID` |
| Sep 9 / 56d1a6f | getConnectionForPlayer read the caller's key | FIXED | removed; targeting goes through the session store (`broadcast.go:151-205`) |
| Sep 9 / 56d1a6f | inquire panics on wrong types | FIXED | `inquire/handler.go:52,88,99` use comma-ok |
| Sep 9 / a1e0be6 | no panic recovery in dispatch | FIXED | `router/act.go:52-61` |
| Sep 9 / 7dc395f | sendToPlayer/Players/Team matched nothing | FIXED (now unused) | `broadcast.go:160-205` resolves through sessions. `BroadcastToPlayer(s)` has no callers (HS-21) |
| Sep 9 / 7dc395f | privateData leaked in deltas | FIXED for deltas and keyframes; REGRESSED on one path | `events.ClientView` is used by `game.go:195-202`. The Lua `indri.refresh()` path still uses `GameService.Sanitize`, which is not `ClientView` (HS-9) |
| SEC-1 | logout does not invalidate the token | FIXED | `logout/handler.go:40` calls `SessionService.Delete` |
| BUG-2 | register discarded the Write error | FIXED | `register/handler.go:38-40` |
| #d035 | login/reconnect on an already-authenticated connection | FIXED | `login/handler.go:46`, `reconnect/handler.go:40` |
| #d108 | router registry not reset between tests | FIXED | `router/register.go:37` `Reset()` |
| B #2 | layout host check looked players up by user id | FIXED, but see HS-1 | `layout/handler.go:31` → `RequireHost` looks up by slot (`host.go:40`). It never checks that the slot still belongs to the caller |
| B #5 | leave/kick never clear the session's GameID/SlotID/TeamID | **STILL-OPEN** (HS-2) | `leave/handler.go:49` and `kick/handler.go:62` only clear the game slot. `repo/session/fields.go:30-43` `applyUpdate` skips empty fields, so `UpdateSession` *cannot* clear them even if a handler tried |
| B cleanup | `authResponse` defined twice | **STILL-OPEN** (HS-20) | `login/handler.go:13-17`, `reconnect/handler.go:13-17` |
| B cleanup | one MessagePack encode per recipient in `broadcastToSessions` | **CHANGED, still open** (HS-16) | encoding moved to `deliverLocal`: `broadcast.go:257` `transport.WriteEncoded(c, d.Payload)` still runs once per matching connection |

Checked and found clean: the two-meanings-of-`sessionId` rule. Every `SetKey` stores `session.ID` (`login:62`,
`reconnect:56`). Every reply carries the token (`login:71`, `reconnect:65`). Every server-side lookup (`logout:40`,
`kick:60,68`, `HandleDisconnect`, `deliverLocal`) uses the ID. No handler reads a client-supplied `userId`. No game write
in scope bypasses `Mutate`: `AssignSlot`, `RemovePlayer`, `DisconnectPlayer`, layout and Lua all go through it.

---

### [P1] A player who left keeps host powers through their stale session slot, and can kick the new host or edit the layout
- ID: HS-1
- Category: security
- Location: internal/handlers/utils/host.go:27-41; enabled by internal/handlers/actions/leave/handler.go:49 and internal/repo/game/operations.go:152
- Confidence: CONFIRMED (reproduced: `TestRepro_StaleSlotHijacksHost`)
- Prior: B #2 / Sep 9 kick authorization (regression in the fix)
- What: `RequireHost` trusts `session.SlotID` and `session.GameID` and checks `g.Players[slot].Host`. It never checks that `g.Players[slot].UserID == *session.UserID`. `leave` clears the slot but not the session (HS-2). When the host leaves, the game has no host, and `assignSlot` gives the host flag to the next joiner, who lands in the first empty slot. That is usually the slot the old host just left.
- Failure scenario: Alice (host, slot p0) sends `leave`. Bob joins team red, lands in p0, and becomes host. Alice sends `{"action":"kick","code":"HIJ","slotId":"p0"}`. `RequireHost` sees her session at `GameID=g, SlotID=p0` and `Players["p0"].Host==true`, so it authorizes her. Bob is removed and disconnected. The repro logs `kick err=<nil>; bob still seated=false`. The same path gives Alice every `layout` op, including `setScript`.
- Suggested fix: in `RequireHost`, also require `session.UserID != nil && g.Players[slot].UserID == *session.UserID`. Also clear the session on leave and kick (HS-2). Add a test for this case next to `TestLayout_OnlyTheHostCanEditTheLayout`.

### [P1] Kick and leave don't end game membership: the removed player keeps receiving the game and can still act in it
- ID: HS-2
- Category: security
- Location: internal/handlers/actions/leave/handler.go:49; internal/handlers/actions/kick/handler.go:62-70; internal/repo/session/fields.go:30-43; internal/handlers/actions/reconnect/handler.go:71-80; internal/handlers/luahandler/handler.go:146-164; internal/services/broadcast/broadcast.go:151-158
- Confidence: CONFIRMED (by reading the code; the stale session state is also shown in the HS-1 repro log)
- Prior: B #5 (STILL-OPEN)
- What: after `leave` or `kick`, the session still says `GameID/TeamID/SlotID = <old game>`. Everything that resolves "the caller's game" from the session keeps treating the removed player as a member:
  - `gameSessions` (`broadcast.go:152`) still sends them every delta and keyframe.
  - `reconnect` writes them a fresh keyframe of the game (`reconnect:71-80`).
  - `refresh` does the same.
  - Lua handlers (`luahandler/handler.go:146-164`) and the tic-tac-toe `move` handler (`example/tictactoe/server/handlers/move/handler.go:35`) run `Mutate` on that game for the team recorded in the session, and neither checks that the caller still holds a slot.
  - `UpdateSession` can't clear these fields even if a handler tried, because `applyUpdate` ignores empty strings.
- Failure scenario: the host kicks a griefer. `CloseSessions` drops the connection, but the session and its token survive. The griefer sends `{"action":"reconnect","sessionId":"<token>"}`, gets the keyframe of the game they were kicked from, and keeps receiving every delta. They can then send the game's action (a Lua handler action, or `move` in tic-tac-toe) and it mutates the game as their old team. Kick doesn't keep them out.
- Suggested fix: add an explicit "leave game" session operation to the session repo (e.g. `ClearGame(sessionId)` that nils GameID/TeamID/SlotID), and call it from `leave`, from `kick` (on the target's session), and when `join` or `create` moves the caller to another game. Also have game-action resolution check `g.Players[session.SlotID].UserID == session.UserID`, the same check as HS-1. This is `plans/session-resume.md` bug 2.

### [P2] Plaintext passwords are written to the server log whenever login or register fails
- ID: HS-3
- Category: security
- Location: internal/handlers/router/router.go:16 and :24
- Confidence: CONFIRMED
- What: `HandleMessage` logs the whole decoded payload on any handler error (`log.Printf("error handling message %v: %v\n", decodedMsg, err)`; `decodedMsg` is `*map`, so `%v` prints `&map[email:… password:…]`). It also logs the raw bytes on a decode error. Login and register payloads carry the password.
- Failure scenario: a user mistypes their password. `Authenticate` returns "password does not match", and the log line contains `password:<what they typed>`, which is often their real password for another site, or one typo away from it. A login on an already-authenticated connection, a duplicate-email register, or a malformed login JSON leaks it the same way. Anyone with log access (log aggregation, support staff) collects credentials.
- Suggested fix: log only the action name and the error, never the payload. If payload logging is ever wanted for debugging, redact keys such as `password` and `sessionId` (the token).

### [P2] Every Lua handler run deletes `null` placeholders and turns `[]` into `{}` across all team and scene data
- ID: HS-4
- Category: bug
- Location: internal/handlers/luahandler/runtime.go:15-41 (jsonToLua `nil` → `LNil`), :46-71 (luaToJson: empty table → map), :135-180 (applyLuaToGame writes back unconditionally)
- Confidence: CONFIRMED (reproduced: `TestRepro_RoundTripDropsNullAndEmptyArray`)
- What: `applyLuaToGame` writes back *every* team's `data` and the current scene's `data` and `privateData` after each script run, whether or not the script touched them. The round trip through Lua loses information:
  - a JSON `null` becomes `LNil`, which removes the key;
  - an empty array becomes an empty table, which `luaToJson` returns as `map{}`;
  - arrays with `null` holes are truncated.
- Failure scenario: a scene carries `{"winningTeam": null, "moves": []}` (the documented shape-stability placeholders; tic-tac-toe's config has `winningTeam: null`). Any Lua action, even a no-op script, stores `{"moves": {}}`. `winningTeam` is gone and `moves` is now an object. The change publisher sees a shape change, so every Lua action re-keyframes the whole game instead of sending a delta. Client code that expects an array now gets an object. The repro prints `team data after: {"list":{}}` and `scene data after: {"moves":{}}`.
- Suggested fix: represent JSON null with a sentinel (e.g. a userdata `json.null`) and empty arrays with a tagged table (or a metatable marker), and convert them back. Or write back only the tables the script actually assigned. Add round-trip tests for `null`, `[]`, and `[1,null,3]`.

### [P2] Joining or creating a second game leaves a ghost, still-connected (possibly host) player in the first game
- ID: HS-5
- Category: bug
- Location: internal/handlers/actions/join/handler.go:62-85; internal/handlers/actions/create/handler.go:64-87; internal/repo/game/operations.go:128-171
- Confidence: CONFIRMED
- What: `assignSlot` enforces "one slot per user" only *within* the target game. Neither handler removes the caller from the game their session currently records before overwriting `session.GameID` with the new game.
- Failure scenario: Alice hosts game A (slot p0, `connected:true`), then sends `create` for game B. Her session now points at B. In A she stays `connected:true` and `host:true` forever: `HandleDisconnect` only ever marks her slot in B. Nobody in A can kick her, because they're not host, and A never gets a new host. A's slot stays taken, and `inquire` counts it.
- Suggested fix: in `join` and `create`, if `session.GameID` is set and differs from the target, call `RemovePlayer(oldGame, session.SlotID, userId)` first, or reject with "leave your current game first". Share this step through the HS-2 session helper.

### [P2] `reconnect` never marks the player connected again
- ID: HS-6
- Category: bug
- Location: internal/handlers/actions/reconnect/handler.go:71-80; `ConnectPlayer` in internal/repo/game/{mongo,memory,sqlite,postgres}_player.go:14 has no production caller
- Confidence: CONFIRMED
- What: a disconnect runs `DisconnectPlayer` (`entrypoints/websocket.go:57`). The resume path (`reconnect`) sends a keyframe but never calls `ConnectPlayer`, which is implemented in all four stores and called by nothing.
- Failure scenario: a phone drops Wi-Fi. The server marks the player `connected:false`, and the client reconnects with its token. The player is back and receiving deltas, but every other client shows them as disconnected until they happen to send `join` again (which goes through `assignSlot`'s "held" branch).
- Suggested fix: after the keyframe in `reconnect`, when `session.GameID` and `SlotID` are set, call `GameRepo.ConnectPlayer(gameId, slotId, userId)`. Its holder check already makes this safe against a stale slot.

### [P2] `inquire` reports every team, and so every game, as full
- ID: HS-7
- Category: bug
- Location: internal/handlers/actions/inquire/handler.go:137-159; internal/repo/game/operations.go:110-116
- Confidence: CONFIRMED
- What: `createGameInfo` treats `len(team.PlayerIDs) >= MaxPlayersPerTeam` as "full". `preDeclareSlots` fills `PlayerIDs` with every slot ID when the game is created, and `clearSlot`/`assignSlot` never shrink it. So `isFull` is always true.
- Failure scenario: a new empty game shows `{"full": true, "teams":[{"full":true},…]}` in `availableGames` (when `createTeams` is false), so lobby UIs hide or grey out every joinable game.
- Suggested fix: count occupied slots, i.e. `PlayerIDs` whose `g.Players[id].UserID != ""`. Add a handler test for it; the package has none.

### [P2] Handler failures are never reported to the client
- ID: HS-8
- Category: bug
- Location: internal/handlers/router/router.go:22-25; for example login/handler.go:50-53, join/handler.go:62-66, layout/handler.go:37-45, kick/handler.go:38-41
- Confidence: CONFIRMED
- What: a handler's returned error only goes to `log.Printf`. Apart from the unauthenticated redirects in create, join and inquire and a few writes in refresh, no handler writes a `models.WSError`. The documented error channel (PROTOCOL "Error", `connection.Service.WriteError`) goes unused on most failure paths.
- Failure scenario: wrong password, joining a full team, creating a duplicate code, a layout op that fails validation (PROTOCOL says "reject with the specific issue"), or a kick by a non-host: the client gets no reply at all and just waits. For the layout editor, a rejected drag can't be told apart from a slow one.
- Suggested fix: have `HandleMessage` write a generic `WSError` (with an `action` echo) when `Act` returns an error. Map known sentinel errors (auth failure, not host, validation) to specific codes, and never include the raw internal error text.

### [P2] Lua `indri.refresh()` / `refreshSelf()` send the wrong shape and bypass `ClientView`
- ID: HS-9
- Category: security
- Location: internal/handlers/luahandler/handler.go:43-72; internal/services/game/game.go:168-190
- Confidence: CONFIRMED (the reference client ignoring the message was inferred from the PROTOCOL.md classifier rules, not run)
- Prior: Sep 9 "privateData leaked" (partial regression)
- What: the refresh payload is `GameService.Sanitize(g)`, a bare `*models.Game`. It has four problems:
  1. It nils `PrivateData` only at the five struct levels, not "at every depth" the way `events.ClientView` does. So `data.foo.privateData` or `players.pN.data.privateData` reach every client.
  2. It still contains `data.layout`.
  3. It isn't a keyframe wrapper (`{sv, game}`), so the reference client's classifier (PROTOCOL.md "Client-side message classification") matches nothing and drops it.
  4. `refreshSelf` writes JSON text with `cs.Write`, ignoring the connection's MessagePack mode.

  `Sanitize` also mutates its argument, and it is a second definition of what clients may see, which CLAUDE.md forbids.
- Failure scenario: a script stores `game.scene.data.hand = {privateData = {...}}` (or the script's data nests `privateData` anywhere) and calls `indri.refresh()`. Every player receives the nested private data, and their client silently discards the message anyway.
- Suggested fix: drop both functions (`Mutate` already publishes the delta or keyframe), or make them call `GameService.WriteKeyframe` / broadcast `GameService.Keyframe(id)`. Delete `GameService.Sanitize` and `GetJSONBytes`.

### [P3] A Lua script that assigns an array to team or scene data panics the handler
- ID: HS-10
- Category: bug
- Location: internal/handlers/luahandler/runtime.go:149, :166, :173
- Confidence: CONFIRMED (reproduced: `TestRepro_ArrayTeamDataPanics`)
- What: `luaToJson(L, dataTable).(map[string]interface{})` is an unchecked assertion. A table with an array part returns `[]interface{}`.
- Failure scenario: a script does `game.teams.t1.data = msg.items`, and a client sends `items: [1,2]`. The handler panics with "interface conversion: interface {} is []interface {}, not map[string]interface {}". The router recovers it, the action silently fails (HS-8), and the whole thing is triggered by client input.
- Suggested fix: use the comma-ok form and return an error ("team data must be an object").

### [P3] Lua handlers have no time or instruction limit and load the full stdlib (`os`, `io`, `debug`, `load`)
- ID: HS-11
- Category: security
- Location: internal/handlers/luahandler/handler.go:101, :130; inquire_handler.go:23
- Confidence: PLAUSIBLE (whether it can be exploited depends on the deployed script)
- What: `lua.NewState()` opens every library and no `L.SetContext` is set. The script runs *inside* `Mutate`, while the game lock is held. The scripts come from the deployer (trusted), but they loop over client-supplied `msg` values, and `string.rep` and table growth are unbounded.
- Failure scenario: a script does `for i = 1, msg.count do … end` and a client sends `count: 1e12`. The goroutine spins forever while holding the game lock, and every other write to that game blocks. With Redis locks, the 10 s lease expires and the version fence discards the eventual write, but the CPU stays pinned.
- Suggested fix: pass `lua.Options{SkipOpenLibs: true}` and open only base (minus `dofile`/`loadfile`/`load`), table, string and math. Set `L.SetContext(ctx)` with a deadline (e.g. 100 ms) and set `RegistryMaxSize`/`CallStackSize`.

### [P3] The login/reconnect reply leaks `User.PrivateData`
- ID: HS-12
- Category: security
- Location: internal/services/user/user.go:26-29; internal/models/user.go:19
- Confidence: CONFIRMED
- What: `UserService.Sanitize` only clears `Password`. `User.PrivateData` is tagged `json:"privateData,omitempty"`, so it goes into `authResponse.User` on login and on reconnect.
- Failure scenario: the server keeps moderation notes or flags in a user's `privateData`, and the user reads them from their own login reply.
- Suggested fix: set `PrivateData = nil` in `Sanitize`, or tag it `json:"-"` like `Password`.

### [P3] Login allows account enumeration by timing and has no rate limit; each attempt costs a bcrypt
- ID: HS-13
- Category: security
- Location: internal/services/authentication/authentication.go:42-51; internal/services/utils/password.go:10-12
- Confidence: CONFIRMED (the timing difference follows from the code paths; it was not measured)
- What: an unknown email returns straight after the lookup. A known email runs `bcrypt.CompareHashAndPassword` at cost 10. There is no per-IP or per-account throttle on login or register.
- Failure scenario: an attacker sends `login` and then `inquire` on the same socket (messages are handled serially), or uses SSE `POST /sse/send`, which returns 204 only after the handler finishes. The reply is noticeably slower when the email exists. Unthrottled login spam also burns about one bcrypt of CPU per message.
- Suggested fix: when the user doesn't exist, compare against a fixed dummy hash. Add a per-connection and per-email attempt limiter.

### [P3] `create` leaves orphan games on failure, and game creation is unbounded
- ID: HS-14
- Category: bug
- Location: internal/handlers/actions/create/handler.go:49-67
- Confidence: CONFIRMED
- What: `GameService.New` commits the game before `AssignSlot` validates `teamId`, and nothing rolls it back. Nothing deletes games, and there is no per-user cap.
- Failure scenario: `{"action":"create","code":"party","teamId":"nope"}` creates game `party` with no players and returns an error. That code is now taken permanently. A logged-in client can loop this to reserve arbitrary codes and fill the database.
- Suggested fix: validate `teamId` against `Script.Teams` before `New`, or delete the game when `AssignSlot` fails. Also consider limiting games per user.

### [P3] `join`/`create` update the session only after reading the keyframe, so deltas in between are lost
- ID: HS-15
- Category: concurrency
- Location: internal/handlers/actions/join/handler.go:68-85; create/handler.go:70-87; internal/services/broadcast/broadcast.go:151-158
- Confidence: CONFIRMED (race window read from the code; not reproduced)
- What: the handler reads `fresh`, writes the keyframe, and only *then* sets `session.GameID`. The monitor picks recipients with `sr.Find("gameId", …)` when it delivers. A delta committed after the `fresh` read but before `SessionService.Update` goes to everyone except the joiner, and the joiner's keyframe is older than it.
- Failure scenario: Bob joins while Alice makes a move in that window. Bob's board never shows Alice's move until the next keyframe, and later positional deltas apply over a wrong base.
- Suggested fix: update the session (GameID, SlotID, TeamID) *before* reading `fresh` and writing the keyframe. A delta that then arrives before the keyframe is discarded by the client's keyframe cutoff.

### [P3] Delivery scans every connection per message and re-encodes the payload once per recipient
- ID: HS-16
- Category: performance
- Location: internal/services/broadcast/broadcast.go:231-261
- Confidence: CONFIRMED
- Prior: B cleanup "one MessagePack encode per recipient" (CHANGED; still open)
- What: `deliverLocal` walks all of `t.Conns()`, runs a linear `slices.Contains` over the recipient IDs for each, and encodes `d.Payload` again for every match. That is O(connections × recipients) plus N encodes per delta.
- Failure scenario: 5,000 connections on an instance and a 50-player game with a move every 100 ms: about 250k string compares and 50 MessagePack encodes of the same delta per move, per instance. It gets worse with keyframes, which are the size of the whole game.
- Suggested fix: build a `map[string]struct{}` of recipient IDs, and encode at most twice (MessagePack and JSON) and reuse the bytes (e.g. a `transport.Encoded` holding both).

### [P3] One session on several connections: presence and logout act on only one of them
- ID: HS-18
- Category: bug
- Location: internal/entrypoints/websocket.go:27-61; internal/handlers/actions/logout/handler.go:28-44
- Confidence: CONFIRMED
- What: login returns the *same* session and token for every login by a user, and `reconnect` lets any number of connections adopt it. Any one of them disconnecting marks the player `connected:false`. Logout deletes the session but leaves the other connections holding a dangling `sessionId` key.
- Failure scenario: a player is logged in on a laptop and a phone and closes the phone tab. Everyone sees them as disconnected while they keep playing on the laptop. If they log out on the phone, the laptop connection stays open and still receives broadcasts that were resolved before the delete, but every action it sends fails "session not found" with no reply (HS-8).
- Suggested fix: decide the policy: either one connection per session (close the previous connection on login or reconnect, via `CloseSessions`), or reference-count presence. On logout, `CloseSessions(sessionId)` so every connection is dropped.

### [P3] Most of the slice has no tests, including every auth-path handler
- ID: HS-19
- Category: tests
- Location: no `_test.go` in: handlers/actions/{create,inquire,join,leave,login,logout,reconnect,refresh,register}, handlers/router, handlers/utils, services/{authentication,session,user,stage,connection,utils}
- Confidence: CONFIRMED (`go test` output)
- What: nothing tests the router's recover and hook semantics, the login/reconnect/logout token lifecycle, `RequireHost` edge cases (HS-1 would have been caught), leave/kick session clearing (HS-2), the ghost slot on a game switch (HS-5), reconnect presence (HS-6), or inquire fullness (HS-7). `handlertest` already provides real in-memory stores, so these tests are cheap to write.
- Suggested fix: add handlertest-based tests for each action, starting with the P1 and P2 scenarios above. Add a router test that a panicking handler still returns an error and that `processed` hooks behave as documented.

### [P4] Router semantics: the first error aborts the remaining handlers and the `processed` hook, and the registry is unsynchronized
- ID: HS-17
- Category: maintainability
- Location: internal/handlers/router/act.go:27-47; router/register.go:13-17
- Confidence: CONFIRMED
- What: `runHandler` returns on the first failing handler. So a game handler registered on the same action as a built-in doesn't run if the built-in fails (for example, a Lua `inquire` notification is skipped when the built-in inquire rejects an unauthenticated caller). A failing `received` hook silently drops the message, and `processed` never sees failures. `registeredHandlerMap` is a plain slice with no lock, which is safe only because registration happens before `Serve`. None of this is documented in CLAUDE.md or PROTOCOL.md.
- Suggested fix: document it (the `received` hook doubles as a gate, and `processed` runs only on success), or run every handler and join the errors. Consider freezing the registry once `Serve` starts.

### [P4] `authResponse` is defined twice, and login and reconnect share most of their body
- ID: HS-20
- Category: maintainability
- Location: internal/handlers/actions/login/handler.go:13-17, :59-73; reconnect/handler.go:13-17, :53-69
- Confidence: CONFIRMED
- Prior: B cleanup (STILL-OPEN)
- Suggested fix: move the type and a `bindSessionAndReply(conn, session, token)` helper into `handlers/utils` or a shared `auth` package.

### [P4] Dead code and stubs in scope
- ID: HS-21
- Category: maintainability
- Location:
  - `internal/handlers/messages/` (the whole package; `AuthSuccess`/`AuthError` return nil)
  - `internal/types/errors/errors.go` (empty package)
  - `internal/services/stage/stage.go` (never constructed; it also has a latent path bug: `LoadSceneFromScript` writes `"stage.scene."` instead of `"stage.scenes."` at :151)
  - `GameService.GetJSONBytes`/`FetchByCode`/`Reset`/`Update`/`Sanitize` (game.go:73,121,163,149,170)
  - `session.ServiceInterface`, `FindID`, `Exists`, `Find`, and the no-op `Sanitize` (session/interface.go, session.go:22,65,73,84)
  - `UserService.Find`/`Exists`/`Update`
  - `BroadcastService.BroadcastToPlayer(s)`
  - `utils/session.ValidateStandardKeys`
  - the unused `models.ErrUnauthorized`/`ErrAlreadyAuthorized`/`ErrSessionWrongTeam`/`ErrSessionWrongGame`
  - `CreateSession.CreatedAt`, which `Authenticate` sets to `time.Time{}` (authentication.go:61) and the stores ignore
- Confidence: CONFIRMED (grep for callers outside tests)
- Suggested fix: delete it, or wire up what's needed (for example, use `ErrUnauthorized` in HS-8).

### [P4] `ValidateGameAndUser` is called with a slot id as the "userId", so the errors are misleading
- ID: HS-22
- Category: maintainability
- Location: internal/utils/session/session.go:13-23; internal/repo/game/operations.go:177, :198
- Confidence: CONFIRMED
- What: `removePlayer`/`setConnected` call `ValidateGameAndUser(id, slotId)`. An empty slot reports "userId is required", and an empty game id reports "gameCode is required", though it is an id, not a code.
- Suggested fix: inline a `requireNonEmpty(map[string]string{…})` or rename the function to match what it checks.

### [P4] Small handler cleanups
- ID: HS-23
- Category: maintainability
- Location: see each item
- Confidence: CONFIRMED
- What:
  - `join/handler.go:32` checks `gameCode == nil`, but `ParseGameCodeAndTeamID` never returns nil, and `join` accepts an empty `teamId`. The nil checks in `keys.go:17-19` are also dead.
  - The doc comments on `create` (:24) and `inquire` (:39) say "join game request".
  - `DecodeMessageWithAction` (message.go:12-20) logs the unmarshal error and then logs it again, printing `<nil>` when the JSON was valid but had no action.
  - `refresh` answers an unauthenticated caller with `ErrServerError` 1011 instead of `ErrUnauthorized` 3000 (refresh/handler.go:29), and returns nil on some failures and an error on others.
  - `inquire` runs `FindOpen(100)` even for `gameInfo` (handler.go:83).
  - `inquire` lists teams in map order, so the order is nondeterministic.
  - `HandleDisconnect` logs an "error" for every unauthenticated disconnect (websocket.go:37).
  - `sendToGame`/`sendToTeam` log every broadcast (broadcast.go:140,161).
- Suggested fix: tidy these up as a batch.

### [P4] The layout host check runs outside `Mutate` (TOCTOU)
- ID: HS-24
- Category: concurrency
- Location: internal/handlers/actions/layout/handler.go:31-45
- Confidence: CONFIRMED (narrow window)
- What: host status is read by `RequireHost` before `Mutate`. If the host is removed or moves in between, the write still lands. The window is small, but it undercuts the atomicity that `ValidateLayout` inside `apply` provides.
- Suggested fix: re-check `g.Players[slot].UserID == userId && Host` inside the `apply` closure.

### [P4] Lua per-attempt state: flags survive `Mutate` retries, and the state and stdlib are rebuilt every attempt
- ID: HS-25
- Category: bug
- Location: internal/handlers/luahandler/handler.go:98-139
- Confidence: CONFIRMED
- What: `pma` is shared across `mutation.Run` retries. If attempt 1 calls `indri.refresh()` and loses a version race, and attempt 2's script path doesn't call it, the refresh still fires. A new `LState`, plus parsing `stdlib`, runs on every attempt and every message.
- Suggested fix: reset `*pma = postMutateActions{}` at the top of `apply`. Pre-compile `stdlib` and the script once with `parse.Parse` and `L.NewFunctionFromProto`.

### [P4] The docs still describe the old identity model
- ID: HS-26
- Category: docs
- Location: docs/PROTOCOL.md:124, :260; CLAUDE.md:207
- Confidence: CONFIRMED
- What:
  - PROTOCOL.md's layout auth table still says ``g.Players[userId].Host``, but the lookup is by slot.
  - PROTOCOL.md and CLAUDE.md say the connection key is the "Mongo ObjectID" / `session.ID.Hex()`. IDs are now backend-agnostic strings from `ids.New()` (`repo/session/fields.go:18`).
  - PROTOCOL.md's `logout` section doesn't mention that the token is invalidated.
- Suggested fix: update the three spots.

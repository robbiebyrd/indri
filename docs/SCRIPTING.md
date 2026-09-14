# Server-side scripting

A game in Indri is a `config.json` and one or more Lua scripts. The framework ships the generic
actions — register, login, join, create, leave, kick, refresh, inquire, layout — and your script
supplies the verbs your game understands. `example/tictactoe/` is the worked example: two files,
no Go, running on the stock `cmd/server` binary.

This document is the reference for the person writing that script.

> **There are two Lua hosts in this repository and they are not the same thing.** This document is
> about the **server-side** host in `internal/services/lua/`, which owns game state. The **client**
> also runs Lua — the `script` field inside a layout (`docs/ARCHITECTURE.md`, "Lua is the binding
> layer") — which maps state onto widgets and can write nothing. Both spell their entry point
> `indri.on`, and `example/tictactoe/config.json` contains one of each. If you are writing something
> that draws a board, you want the client host; if you are writing something that decides a move, you
> want this one.

## Contents

- [Declaring a script](#declaring-a-script)
- [Registering handlers](#registering-handlers)
- [What a handler is given](#what-a-handler-is-given)
- [Changing state: `indri.mutate`](#changing-state-indrimutate)
- [What a script may and may not write](#what-a-script-may-and-may-not-write)
- [Talking to players: `indri.reply` and `indri.send`](#talking-to-players-indrireply-and-indrisend)
- [Timers: `indri.after`, `indri.at`, `indri.cancel`](#timers-indriafter-indriat-indricancel)
- [Lifecycle events](#lifecycle-events)
- [Hooks on built-in actions](#hooks-on-built-in-actions)
- [Capabilities](#capabilities)
- [The shipped library: `indri.game`](#the-shipped-library-indrigame)
- [The sandbox](#the-sandbox)
- [Every limit, in one table](#every-limit-in-one-table)
- [Failing: refusing versus faulting](#failing-refusing-versus-faulting)
- [Testing a script with `indri-script`](#testing-a-script-with-indri-script)
- [Which transports reach a script action](#which-transports-reach-a-script-action)
- [When you still need Go](#when-you-still-need-go)

## Declaring a script

`config.json` is the template a new game is stamped from (`models.Script`). Two of its fields are
about scripting:

```json
{
  "schemaVersion": 1,
  "scripts": [
    { "path": "game.lua" },
    { "path": "rules.lua", "grants": ["assets"] }
  ],
  "teams": { "red": { "name": "Red", "playerIds": [], "data": { "score": 0 } } },
  "stage": {
    "currentScene": "quiz",
    "sceneOrder": ["quiz"],
    "scenes": { "quiz": { "data": { "answer": "lemur" } } }
  }
}
```

- **`schemaVersion`** is the config format the file was written for. This build understands version
  `1` (`models.ScriptSchemaVersion`). A version this server does not know **fails boot** rather than
  being read with the wrong rules. A missing one is read as the current version and logged as a
  warning, because every config written before the field existed has none.
- **`scripts`** lists the Lua files to load, **in load order**. Each entry is
  `{"path": ..., "grants": [...]}`.
  - `path` is resolved **relative to the config file's own directory**, so a server started from
    another working directory finds the same files (`internal/repo/script/script.go`,
    `resolveScriptPaths`). An absolute path is left alone.
  - The same path twice is a boot failure.
  - A config with **no** `scripts` list loads and logs a loud warning: the server it produces answers
    nothing but the built-in actions.
- **`grants`** is that one script's host capabilities. See [Capabilities](#capabilities).

Everything else in `config.json` is the starting game: `config`, `teams`, `stage`, `data`,
`privateData`.

Check a config without booting anything:

```console
$ indri-script check ./example/tictactoe/config.json
./example/tictactoe/config.json is valid

scripts
  example/tictactoe/game.lua (no grants)

actions
  move

lifecycle events
  (none)
```

## Registering handlers

`indri.on(name, fn)` is the only way a script claims a name. It must be called **while the script is
loading** — at the top level of a chunk — and never afterwards.

One function registers into two separate namespaces, and the name says which:

| Name | Namespace | Reached by |
|---|---|---|
| `"move"`, `"guess"` — no colon | **action** | a client, or a timer |
| `"player:joined"` — contains a colon | **lifecycle event** | the server only |

The two are held in separate registries, so an action can never resolve to a lifecycle handler and a
lifecycle event can never be dispatched by a player.

### Names a script may not take

An action name is refused if it is a **dispatch phase** (`received`, `processed` — these run on every
message) or a **built-in framework action**. Registration is additive, not replacing: a script
claiming `login` would run *alongside* the real login handler, on a payload carrying a plaintext
password.

The built-ins are `create`, `inquire`, `join`, `kick`, `layout`, `leave`, `login`, `logout`,
`reconnect`, `refresh`, `register`.

`indri.on("join", …)` fails to load, and the message names the line that did it:

```
bad.lua:1: indri.on: the action name "join" is reserved: it is a built-in framework action
```

A name containing a colon must be one of the four events this server actually raises. A misspelling
is a **boot failure**, not a handler that silently never runs:

```
bad.lua:1: indri.on: "player:quit" is not a lifecycle event this server raises; it raises [game:created player:joined player:left scene:changed]
```

Registering the same name twice — in one script or across two — fails and names both lines.

### Registration must be deterministic

The engine keeps a pool of prepared Lua states and runs **every chunk on every state**. The set of
names collected at boot becomes the frozen manifest, and every state built afterwards is checked
against it. A script that derived an action name from anything varying between states would
otherwise give each pooled state a different action set, and whether a player could reach an action
would depend on which state their message landed on. That disagreement is a hard failure:

```
lua script registration is not deterministic: this state registered the actions [...],
but the manifest collected at boot says [...]
```

## What a handler is given

An action handler receives one table:

```lua
indri.on("guess", function(req)
  req.action   -- the action name, a string
  req.gameId   -- the game this dispatch belongs to, when there is one
  req.payload  -- the client's arguments, a table (the "action" key is already removed)
  req.session  -- the caller, or ABSENT when unauthenticated
end)
```

`req.session`, when present, carries exactly three fields and no more:

```lua
req.session.userId
req.session.gameId
req.session.teamId
```

`models.Session.Token` (the bearer token) and `models.Session.ID` (the broadcast targeting key) are
**deliberately not there**. Neither is a script's to read, and neither is needed to decide whether a
caller may make a move.

Two things about this table repay attention:

- **`req.session` is absent, not nil-valued, for an unauthenticated caller.** A script asking who
  called has to say what it means for the answer to be nobody. Always check it before reading
  `req.session.teamId`.
- **`req.gameId` is the field to read, not `req.session.gameId`.** A timer-fired action runs with
  **no session at all** (see [Timers](#timers-indriafter-indriat-indricancel)), so a handler reaching
  through `req.session` would fault on exactly the dispatches that most need to find their game.

Authority always comes from `req.session`, never from `req.payload`. A payload is whatever the client
chose to send.

## Changing state: `indri.mutate`

```lua
local committed = indri.mutate(function(state) ... return state end)
```

One argument: a callback. It is handed the game as a table and returns the game it wants stored, or
`nil` to make no change. The boolean result says whether the state **actually changed**: `false` for
an explicit `nil`, `false` for a return deep-equal to the input, `true` for a committed write.

The callback runs **inside the store's `apply` closure**, which is what puts a script edit under the
same distributed lock and the same version fence as every Go write, rather than inventing a second
concurrency model that would have to agree with the first.

Three consequences follow from that, and all three are things a script author has to hold in mind:

1. **The callback may run up to ten times.** Every version-fence miss reloads the game and calls it
   again against the state that actually won. It is given a freshly converted table each time.
   **Read everything you need from the `state` you were handed** — a value captured from a previous
   attempt describes a document that is no longer current.
2. **A nested `indri.mutate` on the same game is refused**, not allowed to deadlock. The lock is a
   plain keyed mutex with no notion of a holder, so a nested call would wait on a lock its own caller
   holds and never wake. Do all the work in one callback.
3. **Nothing a handler asks the host to do outside the document happens inline.** Replies, sent
   events, timers and scene-change events are all queued and released only once the write commits.
   See [the effect ledger](#the-effect-ledger).

The state you receive is the **JSON view** of `models.Game` — exactly what `events.ToMap` produces
and exactly what the client already holds. Field names are json tag names, numbers are floats,
timestamps are RFC 3339 strings. So it is `state.stage.currentScene`, `team.playerIds` and
`player.score`, never the Go field names.

Nothing is hidden from a script: `privateData` is visible here. Sanitization happens on the way *out*
(`GameService.Sanitize` for keyframes, `events.SanitizeDelta` for deltas), because Lua is
authoritative.

A worked handler, from a game whose scene carries `answer`:

```lua
local game = require("indri.game")

local function refuse(message)
  error(message, 0)
end

indri.on("guess", function(req)
  if req.session == nil then
    refuse("not authenticated")
  end

  local solved = indri.mutate(function(state)
    local scene = game.scene_data(state)

    if scene.answer == nil then
      refuse("this scene has no answer")
    end

    if scene.solvedBy ~= nil then
      refuse("already solved")
    end

    if (req.payload or {}).guess ~= scene.answer then
      return nil            -- no change, no write, no delta
    end

    scene.solvedBy = req.session.userId

    return state
  end)

  indri.reply({ correct = solved })
end)
```

### Deleting a field

Returning a table without a key **deletes that field**. The document a script returns is decoded into
a *zero* `models.Game` rather than merged over the existing one, precisely so that removal works.
This is why nothing in the marshaller is ever silently dropped or truncated: a truncated conversion
would be indistinguishable from a deliberate deletion and would erase game state.

### Table key rules

A table key becomes a document field name and a delta path segment, so four keys are refused
outright (`internal/services/lua/marshal.go`, `validateKey`):

| Refused | Why |
|---|---|
| `""` | Mongo has no such field |
| contains `.` | the delta protocol's path separator; it would forge a path segment |
| contains `$` | not a valid Mongo field name; a leading one reads as an update operator |
| contains NUL | not a valid Mongo field name |

Integer keys are allowed and rendered in base ten. Floats with a fraction, booleans, tables and
functions are not — they have no stable field name. Integers beyond ±2^53 are refused, because the
document round-trips through JSON as a float64 and would silently change value.

There is one thing Lua genuinely cannot express: the difference between an empty list and an empty
object. `coerceEmpty` walks `models.Game`'s reflect type alongside your document and fixes it for
every field whose Go type is concrete — so an empty `team.playerIds` and an empty `game.players` both
work. Inside a `data`/`privateData`/`playerData` map the target is `interface{}`, which accepts
either shape, so nothing there is touched.

## What a script may and may not write

A script is handed the whole document, which is what makes `state.players[id].score = n` ergonomic.
It is also what would let a script rewrite who is in the game. `checkInvariants`
(`internal/services/lua/game.go`) refuses the edit before anything is stored, and unwinds the write:

| Refused | Message |
|---|---|
| changing `game.code` | `script changed the game code from %q to %q` |
| changing `game.private` | `script changed the game privacy from %t to %t` |
| adding or removing a player | `script changed the player set (added [...], removed [...])` |
| changing any player's `host` | `script changed the host flag of player %q from %t to %t` |
| changing any player's `connected` | `script changed the connected flag of player %q from %t to %t` |
| adding or removing a team's members | `script changed the membership of team %q (added [...], removed [...])` |

**Why membership is not a script's to change.** `models.Session` stores `GameID`, `TeamID` and
`UserID` independently of the game document. A membership edit made from Lua does not desynchronise
one document, it desynchronises two, and nothing reconciles them afterwards. Host is worse: a script
could promote its own caller. Membership therefore stays in Go, reached only through the built-in
actions (`join`, `leave`, `kick`, …) which also maintain the session.

What a script **does** own is the game's content: `Player.Score`, every `data` / `privateData` /
`playerData` map, and the whole `Stage` — including `stage.currentScene`, which is how a game moves
scene (and which raises `scene:changed`).

`id`, `version`, `createdAt` and `deletedAt` are server-owned. A script's `id` key is dropped before
decoding; the other three are restored afterwards. `version` in particular would otherwise reset to
zero and silently destroy the CAS fence.

## Talking to players: `indri.reply` and `indri.send`

### `indri.reply(payload)`

Writes one document straight back to the caller of this action. `payload` must be a **table** —
every frame this repo writes to a client is a JSON object, and a bare string or number would produce
a document no client parses.

**A handler may reply at most once.** One dispatched action has one direct answer; a script that
replies twice has almost always reached the same line twice by accident. The second call raises,
which turns that into a file and a line instead of a frame a player never sees.

### `indri.send(action[, payload])`

Dispatches another action **after the handler that asked for it has returned** — never from inside
it. Inline dispatch of a script-emitted event is the largest source of unbounded recursion in every
event system surveyed, and it would also dispatch from inside `indri.mutate`'s callback, with the
game's lock held and the whole sub-tree repeated on every retry.

- The event carries **the caller's own session**, so a script cannot reach an action by sending it
  that the player could not have sent themselves.
- The payload must be a table of named arguments. An array is refused rather than coerced, because it
  would reach the handler as an empty payload with no sign that anything was dropped.
- The chain is capped at **10 deep** and **32 sends per invocation**.
- **The sub-dispatch's replies are dropped.** Nobody is waiting on a sent event — the handler that
  asked for it has already returned. Its `DisconnectIDs` *are* merged, so `indri.send("kick")` works.

Read the caps honestly: 32 at each of 10 levels is 32^10 dispatches in the worst case, so **the caps
are not what bounds the tree**. What bounds it is the deadline. Every effect is delivered inside the
`Invoke` that queued it, and each invocation derives its context from its caller's, so every dispatch
below one client message shares that message's 100 ms budget and unwinds together when it expires.
The caps keep an honest script honest; the deadline is what stops a hostile one.

### The effect ledger

Nothing `indri.reply`, `indri.send`, `indri.after`, `indri.at` or `indri.cancel` asks for happens when
you call it. Each is queued on a ledger and released afterwards
(`internal/services/lua/effects.go`).

The reason is the retry loop. `mutation.Run` re-runs the apply closure on every version-fence miss, so
an effect performed inline would fire **once per attempt, including the attempts whose state was
thrown away**. The ledger holds effects queued inside a mutate at an "attempt" level and either
promotes them wholesale when the write commits or discards them wholesale when it does not.

Order is preserved across the boundary: a reply queued before a mutate, one inside it and one after it
are delivered in that order.

**The guarantee, stated per kind rather than claimed away.** The buffer is flushed *after* the write
commits, and games are stored in a standalone mongod with no multi-document transactions — so the
game document and an outbox of its effects cannot be written in one atomic act. A crash in the window
between commit and flush loses whatever had not been delivered.

- `indri.reply` and `indri.send` are **best-effort**. A lost frame costs one player one frame; the
  client's next `refresh` returns a full keyframe and rebuilds from it. Nothing stays wrong.
- `indri.after`, `indri.at` and `indri.cancel` are **at-least-once**. This is the kind that matters —
  a lost timer hangs a game forever, because nothing else will ever fire it. The schedule store
  carries an idempotency key with a unique index so that the duplicate at-least-once permits is
  absorbed rather than played twice.

Nothing here is exactly-once, and nothing built on it may be documented as such. What the ledger
*does* guarantee is narrower and exact: an effect queued during a mutation is emitted once per
**commit** of that mutation, never once per attempt.

## Timers: `indri.after`, `indri.at`, `indri.cancel`

```lua
local id = indri.after(seconds, action[, payload])   -- relative delay
local id = indri.at(timestamp, action[, payload])    -- absolute RFC 3339 string
indri.cancel(id)
```

Both setters return the id the timer will be stored under, **minted synchronously** before anything
is written. That is what makes the obvious pattern work:

```lua
indri.on("start_round", function(req)
  indri.mutate(function(state)
    state.data = state.data or {}
    state.data.roundTimer = indri.after(30, "end_round", { round = 1 })

    return state
  end)
end)
```

The id is stored in the very commit that releases the timer write. If the attempt loses the version
fence, the timer goes with it *and so does the state that named it*, so the two cannot disagree.

### A timer fires with no session. Ever.

This is the contract the whole feature rests on. Nobody is connected when a timer fires, and
inventing a session would hand a scheduled action authority no player ever granted it. The dispatch
carries a nil session, and the game reaches the handler as **`req.gameId`**.

Two things follow, and both are enforced rather than documented-and-hoped:

- **Only a script-declared action may be scheduled.** A built-in is refused, because every built-in
  rejects a nil session and the entry could only ever fail. An action matching *no* handler is worse:
  `router.Dispatch` succeeds quietly when nothing matches, so the timer would fire, do nothing, and
  report success. `indri.after("…", "join", …)` and `indri.after("…", "typo", …)` both raise at the
  line that made the call.
- **The scheduler checks again at fire time** against the manifest it was built with, because an
  entry outlives the script that wrote it. An action removed in an upgrade is **dead-lettered**
  there, with the reason recorded on the entry.

### Argument rules

- `indri.after` takes **seconds**. A negative delay is refused rather than clamped — there is no
  reading of "fire this thirty seconds ago" that an author meant. NaN and infinity are refused. A
  delay beyond **one year** is refused: `time.Duration` is nanoseconds in an int64, so a few hundred
  years wraps to negative and fires immediately, and a value that large is almost always milliseconds
  passed where seconds were meant.
- `indri.at` takes **RFC 3339 and nothing else** (`"2026-09-12T18:00:00Z"`). A timestamp already in
  the past is *accepted* and fires on the next tick — unlike a negative relative delay it is not
  obviously a mistake (a deadline computed from a round that overran is a real case), and refusing it
  would leave whatever the timer was for undone.
- `indri.cancel` is **idempotent**: an id naming nothing is not an error, because the id you hold may
  name an entry the scheduler claimed a moment ago. It cannot cancel another game's timer — the game
  is taken from the authenticated session, never from the payload.
- Setting and cancelling share **one budget of 32 ops per invocation**. Each op is a write to a
  shared collection that outlives the request, so a handler looping a thousand times would fill a
  database on the strength of one client message. The deadline does not help here: queueing is cheap
  and the cost lands later, on the scheduler.

### What the scheduler does with an entry

| | |
|---|---|
| Poll interval | 1 second |
| Entries claimed per tick | 64 |
| Claim lease | 30 seconds |
| Attempts before dead-lettering | 5 |
| Retry backoff | 10 s, doubling, capped at 10 minutes |
| Orphan grace after fire time | 7 days |

None of these is environment-configurable. See [Configuration](#configuration).

## Lifecycle events

Four events, raised by the server about something that has **already happened**:

| Event | Raised by | Subject fields |
|---|---|---|
| `game:created` | `GameService.New` | `code`, `private` |
| `player:joined` | `GameService.ConnectPlayer`, only when a player was added | `userId`, `teamId` |
| `player:left` | `GameService.RemovePlayer`, only when a player was removed | `userId` |
| `scene:changed` | the engine itself, when a script's `indri.mutate` changes `stage.currentScene` | `sceneId`, `previous` |

The handler receives the subject table plus two fields the host writes **after** it, so a subject key
can never redefine which event this is or which game it belongs to:

```lua
indri.on("player:joined", function(ev)
  ev.event   -- "player:joined"
  ev.gameId  -- the game
  ev.userId  -- from the subject
  ev.teamId
end)
```

There is **no `ev.session`** — same contract as a timer, and for the same reason. Nobody is connected
when a game is created or a player drops.

Facts worth knowing before you write one:

- **A lifecycle handler cannot fail the action that caused it.** The built-in action has already
  committed and is not allowed to fail. Everything that goes wrong in a subscriber is logged and goes
  no further.
- **Nothing is dispatched inline.** Every event goes through a deferred queue, drained to exhaustion
  by whichever goroutine found it idle. Events for one game are therefore handled one at a time and
  in the order raised, which a handler that reads the game depends on. The queue caps at 256 pending
  and drops loudly past that.
- **Each handler gets a fresh 100 ms deadline**, not the emitting request's. The emitting call's
  write has committed, and a client who has already gone away must not stop the game's script hearing
  what happened.
- **An event nobody subscribed to costs nothing** — a slice scan, and no Lua state at all.
- **`scene:changed` can chain**, because the thing that raises it is the thing a handler can do
  again. A handler that moves the scene on raises another. The chain is capped at **10**, and it stops
  by *refusing the write*, not by dropping the event: letting the last move through while telling
  nobody would leave the game somewhere no subscriber ever heard about.

  Note what does *not* bound this. The deadline does not, because each event gets a fresh one on
  purpose. The pending cap does not, because a chain one link wide never grows the queue — an A→B→A
  ping-pong sits at a single pending event and would drain forever.

## Hooks on built-in actions

A hook extends a built-in action without claiming its name.

```lua
indri.before("join", function(req) ... end)          -- runs on every dispatch of "join"
indri.after_action("leave", function(req) ... end)   -- runs only after "leave" SUCCEEDS
```

The second is `after_action`, not `after`, because `indri.after` is already the timer. It is
after-***success***, not "after": `router.Dispatch` returns the moment a handler errors, so a failed
action never reaches the `processed` phase. There is no such thing here as a cleanup hook that runs
whatever happened, and calling one "after" would promise something the dispatcher cannot deliver.

### Which actions are hookable

```
create   inquire   join   kick   layout   leave   refresh
```

That list is **derived** from the built-ins by subtracting the credential actions, so a built-in
added to the framework becomes hookable by default and *not* hooking it is the change somebody has
to argue for.

**`login`, `logout`, `reconnect` and `register` can never be hooked.** `login`'s payload carries a
plaintext password and `register`'s carries the one a player is about to be given; `reconnect`'s
carries the bearer token a client resumes with, and `logout` is what invalidates it. The whole point
of running game logic behind the Lua sandbox is that credentials never sit on the script side of that
boundary, and a hook is the one mechanism that deliberately reaches a built-in — so this is where
that promise is kept.

```
bad.lua:1: indri.before: the action "login" handles credentials and cannot be hooked; a hook on it would be handed the caller's password or bearer token
```

The allow-list is positive, so a misspelling is a boot failure rather than a rule you spend an
afternoon wondering why nobody enforces:

```
bad.lua:1: indri.before: only a built-in action can be hooked, and "jion" is not one; this server's hookable actions are [create inquire join kick layout leave refresh]
```

### Where to put a state change

**A `before` hook should validate and not write.** Its `indri.mutate` commits on its own — the store's
lock and version fence cover that one write and nothing around it — so if the built-in action behind
it then fails, the hook's write stands against a move that never happened. There is no transaction
spanning the two: games live in a standalone mongod, and the phases are separate dispatches besides.

So: **`before` is for refusing a move** — raise, and `router.Dispatch` stops the chain with nothing
written — and **`after_action` is where a state change belongs**, because by then the action it is
reacting to has committed.

```lua
indri.before("join", function(req)
  local payload = req.payload or {}

  if payload.code == "CLOSED" then
    error("this room is closed", 0)
  end
end)

indri.after_action("leave", function(req)
  indri.mutate(function(state)
    state.data = state.data or {}
    state.data.departures = (tonumber(state.data.departures) or 0) + 1

    return state
  end)
end)
```

### Two more things about hooks

- **An unauthenticated caller's payload is dropped before the hook sees it.** Hooks run in the
  `received`/`processed` phases, which run *before* any Go handler has decided whether the caller may
  act, so an anonymous socket sending `{"action":"join", …}` would otherwise reach a `before` hook
  with whatever fields it chose. A hook exists to extend a game's rules, and a game's rules are about
  players it can name; a hook with no caller to name has nothing to learn from their arguments. `req`
  arrives with `session` absent and `payload` empty.
- **A hook is not dispatchable.** It lives in a third registry that no dispatch can reach, so hooking
  `join` is not a way to acquire the name `join`.

## Capabilities

A capability a script was not granted is **absent from the `indri` table that script sees** — not
present behind a permission check. What a script can do is therefore a glance at its grant list, not
an audit of every host function's body.

This server ships two:

| Grant | Installs | On the action host view? |
|---|---|---|
| `assets` | `indri.assets.read(name)` | **yes** |
| `http` | `indri.http.get(url)` | **no** |

An unrecognised grant name is a boot failure, so a typo cannot quietly grant nothing:

```
the lua script "bad.lua" is granted the unknown capability "network"; this server knows [assets http]
```

Granting the same capability twice is also a boot failure.

### `http` is absent from a dispatched action, on purpose

This is the constraint the capability lives under rather than a detail of it. The only way a script
changes anything from an action is `indri.mutate`, whose callback runs inside the store's apply
closure with the game's distributed lock held and is re-run on every version-fence retry. **A
blocking fetch there would hold the lock for the whole timeout and pay it again on each retry.**

So `indri.http` is on a **lifecycle handler's** host view and not an action's. Lifecycle handlers run
on the queue's own goroutine, hold no lock and keep nobody waiting. A `before` or `after_action` hook
gets the *narrow* view too — a hook is part of the request path whatever else it is, and handing it
the full view would put `http` back on the request path through the one mechanism designed to extend
it.

Verified behaviour: a script granted `http`, reading `tostring(indri.http)` inside an action handler,
gets `"nil"`.

`indri.assets` is on every view. An asset read is a bounded read of a file an operator put in a
directory on this host, so it costs a lock holder a disk read rather than however long somebody
else's server takes to answer.

### `indri.assets.read(name)` → string

Reads a file under the server's `assets/` directory (relative to the process's working directory) and
returns its contents. There is exactly one root and the script does not name it — a root a caller
could choose would make every other check decoration.

- Extensions allowed: `.csv`, `.json`, `.md`, `.txt`. An allowlist, on extension rather than sniffed
  content, because the question being answered is "did an operator mean to publish this to scripts",
  and a curated directory of text files is an answer an operator can check by looking.
- Cap: **1 MiB** per file.
- An absolute name is refused. A name that climbs out of the root is refused *lexically first*, so
  `"../../etc/shadow"` is reported as an escape rather than as a missing file. A name that stays
  inside the root but **symlinks** outside it is refused after resolution.
- A root that is missing or is not a directory fails **boot**, not the first player's move.
- It **raises** on any failure, so the error carries the script's own file and line. Wrap it in
  `pcall` if you want to carry on regardless.

### `indri.http.get(url)` → `{status, contentType, body}`

A bounded GET. Raises on anything it refuses.

| Bound | Value |
|---|---|
| Whole call (connect, redirects, body) | 5 s |
| One connection attempt | 3 s |
| Response body | 1 MiB |
| Redirect hops | 3 |
| Schemes | `http`, `https` only — re-checked on every redirect |
| Content types | `application/json`, `text/html`, `text/plain` — checked on the headers, before a byte of body is read |

The SSRF guard sits on the **dialer**, not in front of the request, so it fires with the
already-resolved address at the moment of connection, for every connection including each redirect
hop. That is what closes the DNS-rebinding window a pre-flight "resolve, check, then fetch" leaves
open. Refused: the unspecified address, loopback, private ranges, link-local (where cloud metadata
services live), and every multicast class. Keep-alives are disabled so "every request is dialled, so
every request is guarded" stays literally true, and no proxy is consulted — a proxy would be the only
address ever dialled, and the name the script asked for would never be resolved here at all.

## The shipped library: `indri.game`

```lua
local game = require("indri.game")
```

Read helpers over a game state, written in Lua and embedded in the binary. `require` reaches them
through `package.preload`; there is no path on disk for a script to name (see
[The sandbox](#the-sandbox)).

| Function | Returns |
|---|---|
| `game.current_scene(state)` | the current scene id, and the scene table it names |
| `game.scene_data(state)` | the current scene's `data` table, or `{}` |
| `game.team_of(state, player_id)` | the id of the player's team, and the team table |
| `game.players_in_team(state, team_id)` | a fresh array of member ids, in the team's own order |
| `game.leader_by_score(state)` | the highest-scoring player's id, and the player table |
| `game.each_team(state)` | an iterator: `for id, team in game.each_team(state) do` |

Two rules hold throughout, and both are contract rather than implementation detail:

- **Every table is optional.** `teams`, `scenes` and the three data stores are tagged `omitempty`, so
  a game with no teams arrives with no `teams` key at all. Every helper substitutes an empty table
  rather than raising, and reports "nothing found" by returning `nil`.
- **Every walk over a map is ordered by sorted key.** `pairs()` order is unspecified in Lua, so a
  helper that picked the first match out of `pairs()` would be free to answer two different ways for
  the same game — on two servers, or on two invocations of the same one. `leader_by_score` therefore
  breaks ties by the **lowest player id**, and `each_team` iterates in id order. A game that awards
  something to the leader has to award it to the same player every time it asks.

`game.scene_data` returns the state's **own** table, not a copy, so writing to it edits the state you
are about to return. `game.players_in_team` returns a fresh array, so sorting or truncating it cannot
edit the state by accident.

## The sandbox

A script runs on a `gopher-lua` state with the standard library cut down to what a game needs.

**Libraries opened:** `package`, `base`, `table`, `string`, `math`.

**Libraries never opened:** `io`, `os`, `debug`, `coroutine`, `channel` — they reach the filesystem,
the process environment, the Go call stack and the scheduler respectively.

**Base-library globals removed after `OpenBase`** (gopher-lua has no sandbox mode; they have to be
taken back out by hand):

| Group | Removed |
|---|---|
| Filesystem and code loading | `dofile`, `loadfile`, `load`, `loadstring` |
| Host reach | `collectgarbage`, `print`, `_printregs` |
| Metatable and environment control | `setmetatable`, `getmetatable`, `rawset`, `rawget`, `newproxy`, `setfenv`, `getfenv` |
| Module declaration | `module` |

`load` and `loadstring` also accept a **binary** chunk, and gopher-lua does not verify hand-crafted
bytecode, so removing them is what makes "no binary chunks" true by construction. `require` stays: it
is how a preloaded in-memory module is reached, and `package.path`, `package.cpath` and
`package.loadlib` are emptied so the preload searcher is the only one that can ever find anything.

`print` being gone is the one that most often surprises an author. There is no `print` in a game
script; a script's output would be indistinguishable from the server's own log. Use `indri.reply`,
write to state, or run the script under `indri-script test -v`.

**`string.rep` and `string.format` are capped at 1 MiB of output.** They are special because their
cost is set by an *argument* rather than by the size of the source: `string.rep("x", 1e9)` and
`string.format("%1000000000d", 1)` are each one bytecode instruction that allocates a gigabyte inside
a single Go call the context deadline cannot interrupt. The caps are the timeout on those paths, not
a convenience. (`*` field widths are refused outright, since no static check can bound them.)

**Per-invocation isolation.** A pooled state remembers whatever ran on it last, so three separate
mechanisms keep one invocation from seeing the one before it: the real globals and every module table
are frozen read-only; each invocation gets its own globals table, discarded when the call returns;
and a state whose interpreter was *interrupted* rather than unwound is closed instead of pooled,
because nothing about it can be trusted afterwards.

### Two limits that are accepted, not solved

Neither should be described to an operator as a guarantee.

**Heap.** gopher-lua has no allocator hook, so nothing here bounds memory. `RegistryMaxSize` bounds
the value stack only, and overflowing it raises an ordinary run error rather than unwinding an
allocation. **A script that grows a table forever can still exhaust the process.** The trust tier is
*operator-authored* — the person who wrote the script is the person who deployed the server — which
is why that is acceptable. It would not be acceptable for scripts submitted by players, and this
feature is not built for that.

**Time.** `L.SetContext` interrupts the VM between main-loop iterations, so a deadline stops a Lua
loop but cannot stop one long call into a Go library function. The two string caps above are the
mitigation for the two such calls a script can make arbitrarily expensive from a tiny source.

See also [Host-authored content — accepted risk](ARCHITECTURE.md#host-authored-content--accepted-risk),
which covers the *client-side* layout scripts and the same trust assumption.

## Every limit, in one table

| Limit | Value | Where |
|---|---|---|
| One dispatched action, and the whole dispatch tree below it | **100 ms** | `script.InvocationTimeout` |
| One lifecycle handler (fresh, not inherited) | **100 ms** | `lifecycleTimeout` |
| Loading all chunks onto one state | 10 s | `loadTimeout` |
| `indri.send` chain depth | 10 | `maxEventDepth` |
| `indri.send` per invocation | 32 | `maxEventFanout` |
| `indri.reply` per invocation | 1 | `maxReplies` |
| Timer ops (set + cancel) per invocation | 32 | `maxTimerOps` |
| `indri.after` furthest delay | 365 days | `maxTimerDelay` |
| `scene:changed` chain depth | 10 | `maxSceneChangeDepth` |
| Pending lifecycle events | 256 | `maxPendingLifecycle` |
| Document nesting depth | 32 | `defaultMaxDepth` |
| Values per conversion | 10 000 | `defaultMaxNodes` |
| Exactly-representable integer | ±2^53 | `maxSafeInteger` |
| `string.rep` / `string.format` output | 1 MiB | `defaultMaxStringBytes` |
| Idle pooled Lua states kept | 8 | `defaultMaxIdleStates` |
| Version-fence retries per mutate | 10 | `mutation.Run` |

The 100 ms invocation deadline is coupled to the 10 second Redis lock lease by a factor-of-ten
headroom check that runs at boot (`boot.checkScriptDeadline`). A script that outlived its game's lock
could run beside another instance's attempt on the same game: the version fence would still keep the
*write* safe, but both scripts would have run, and a script's effects are not confined to the
document. Raising `InvocationTimeout` without raising the lease fails startup.

### Configuration

**No environment variable governs any of this.** `internal/repo/env/env.go` covers HTTP, Mongo,
Redis, WebSocket and WebRTC only; there is no `INDRI_LUA_*` and no scheduler variable. Every value in
the tables above — and every scheduler tunable — is a Go constant. Changing one is a code change and
a rebuild, which is deliberate for the ones that are coupled to each other.

## Failing: refusing versus faulting

A script fails by raising, and both kinds of failure take the same route out.

```lua
local function refuse(message)
  error(message, 0)
end
```

**`error(message, 0)` is the idiom for refusing a move.** Level 0 keeps your file and line out of the
frame the player receives: "it is not your turn" is a player's mistake, not the author's. An error
raised without a level prefixes the script's own position, which is what you want when the script
itself is wrong.

Raising inside an `indri.mutate` callback unwinds the store **without saving**, so a refused move
writes nothing and publishes no delta.

The failure reaches the player as a `models.WSError` frame in `Responses`, carrying your message and a
short correlation id. It does **not** come back as a Go error, because the transports do not agree
about one: `boot.handleClientMessage` only logs a dispatch error, so a WebSocket player would see
nothing at all, while REST and GraphQL callers of the same action would each get the raw Go error
string. `Responses` is the one channel every transport writes back.

The full Lua traceback goes to the server log under the same correlation id. The frame carries your
script's file and line — normally an unthinkable thing to leak to a client, and the right call here,
because the "server-side" code in question is the operator's own game script and its author is the
only person who can act on the failure.

A `before` hook is the exception that answers through both channels: the player gets the frame *and*
the dispatch chain stops, which is what makes `indri.before` the place a game refuses a move.

A **panic** cannot take the process down. Every handler call is wrapped in a protected call, and
`router.invokeHandler` recovers besides.

## Testing a script with `indri-script`

`indri-script` runs a game's scripts against fixture state with **no server and no database**. It is
what makes "a game author writes no Go" true.

```console
$ go run ./cmd/indri-script --help
indri-script runs a game's Lua scripts without a server or a database.

usage:
  indri-script test [-script config.json] [-timeout 5s] [-v] <path>...
  indri-script check [config.json]
```

- **`check`** compiles the scripts a config declares, resolves their grants, and prints the actions
  and lifecycle events they register. Nothing is booted. Every failure a server would hit at boot
  surfaces here.
- **`test`** plays every `.test.json` fixture the paths name. A path is a fixture file, or a directory
  scanned for them. With a single directory and no `-script`, the config is **that directory's**
  `config.json`; otherwise it falls back to the working directory's, and `-script` overrides both.
- `-timeout` (default **5 s**) bounds one case. It is deliberately *not* the server's 100 ms budget: a
  CI runner under load that reported a correct script as broken because a pooled Lua state took
  120 ms to build would be worse than no harness at all. What it does bound is a script that never
  returns.
- `-v` lets the server's own log through, tracebacks included. Without it the log is discarded,
  because a good fixture set mostly asserts refusals and the host logs every one of them.

Exit codes are distinct on purpose: `0` ok, `1` a case failed, `2` the harness could not run (bad
args, a fixture that does not parse, a config declaring no scripts). "Your script is wrong" and "your
fixtures do not parse" want different responses from whoever reads the build.

### Why a fixture asserts on the delta

The delta is the whole of what a player observes. A write that never publishes is invisible, so a
harness that asserted on the *stored* game would pass on a change nobody in the game could see —
which is the single failure this repository cares most about. It is also why the assertions cannot be
written in Lua: the delta is computed in Go after the callback returned and the save committed, and a
script inside the sandbox cannot see it at all.

What is real in the harness is everything below the fixture: the production engine, the production
config loader, and a `game.MemoryStore` running the same core as the MongoDB store — same lock, same
version fence, same retry budget, same deltas. Only where the documents live differs.

### A fixture file

```json
{
  "cases": [
    {
      "name": "a correct guess solves the scene",
      "event": {
        "action": "guess",
        "session": { "userId": "p1", "teamId": "red" },
        "payload": { "guess": "lemur" }
      },
      "expected": {
        "published": { "stage.scenes.quiz.data.solvedBy": "p1" },
        "replies": [{ "correct": true }]
      }
    },
    {
      "name": "a wrong guess writes nothing",
      "event": {
        "action": "guess",
        "session": { "userId": "p1", "teamId": "red" },
        "payload": { "guess": "tapir" }
      },
      "expected": { "published": {}, "replies": [{ "correct": false }] }
    },
    {
      "name": "refuses an unauthenticated caller",
      "event": { "action": "guess", "payload": { "guess": "lemur" } },
      "expected": { "error": "not authenticated" }
    }
  ]
}
```

```console
$ indri-script test ./mygame
  guess/a correct guess solves the scene .. ok
  guess/a wrong guess writes nothing ...... ok
  guess/refuses an unauthenticated caller . ok

3 cases, all ok
```

A failing case names the mismatch. This one is a real trap, and worth reading: a handler that stores
a timer id publishes a delta carrying a fresh ObjectID, which no fixture can predict.

```console
  rounds/starting a round stores the timer id it will be cancelled by . FAIL
    - the action published 1 deltas, want none: {"data":{"roundTimer":"6aa74d26e91578fce80dea82"},"updatedAt":"2026-09-13T20:25:58.826208-05:00"}

4 cases, 1 FAILED
```

### The fixture format

| Key | Meaning |
|---|---|
| `state` (file level) | the starting `config.json` for every case in the file that declares none |
| `cases[].state` | overrides the file's, and the config's, for this case alone |
| `cases[].name` | required, unique within the file. Reported as `<file>/<name>` |
| `cases[].event.action` | required |
| `cases[].event.session` | `{userId, teamId?}`. **Omit it entirely for an unauthenticated caller** |
| `cases[].event.payload` | the client's arguments |
| `expected.published` | the delta, as dotted path → value. `{}` asserts **nothing was published** |
| `expected.removed` | removed paths. Absent asserts none, rather than ignoring them |
| `expected.replies` | `indri.reply` documents, in order. Absent asserts none |
| `expected.error` | a **substring** of the refusal message |

- `state` is a `config.json`, so a fixture is the game's own config with the one table it is about
  changed.
- Decoding is **strict**: a misspelled key is a load failure. `"cases"` written as `"case"` would
  otherwise be a file that tests nothing and says so nowhere.
- Exactly one of `error` and `published` must be present. A refused action publishes nothing and its
  replies are dropped with it, so pairing `error` with `removed` or `replies` is an assertion that can
  never hold, and is refused at load.
- `error` is a substring because the frame carries a correlation id (and a file and line when the
  script faulted rather than refused), neither of which a fixture can predict.
- A fixture's `session` carries **no game id**: the game is the one the harness just stamped, and its
  id is a fresh ObjectID no file could name.
- `updatedAt` is asserted *present* rather than compared — it is a clock reading, and a delta without
  it did not come from a save.
- One action must publish **exactly one** delta. Two would mean a player saw the game move in two
  steps.

### What the harness does not cover

- **Lifecycle handlers.** Nothing in the harness raises `game:created`, `player:joined`, `player:left`
  or `scene:changed`, so a subscriber is loaded and registration-checked but never run.
- **Hooks.** The harness invokes actions, not the `received`/`processed` phases, so `indri.before` and
  `indri.after_action` are registration-checked only.
- **Timers firing.** `indri.after` and `indri.at` write to an in-memory schedule store that no
  scheduler polls. A fixture format that asserts on one dispatched action has nothing to say about an
  action dispatched an hour later. Note that a timer id is a fresh ObjectID, so a handler that stores
  one produces a delta no fixture can predict.
- **Built-in actions via `indri.send`.** The harness dispatches only actions a game script declares;
  sending a built-in is refused rather than silently doing nothing, because running one would need
  the session, user and game services a real server boots.
- **`indri-script check` does not list hooks**, only actions and lifecycle events, even though the
  engine tracks them.

Where a fixture cannot reach, a Go test can: `example/tictactoe/game_test.go` drives the same engine
directly and is the pattern to copy.

> A note on the worked example, which does not follow its own advice: `example/tictactoe/` holds no
> `.test.json`, so `indri-script test ./example/tictactoe` fails with *"no .test.json fixture files
> found"*. Its fixtures live under `internal/services/scripttest/testdata/tictactoe/`, where they
> double as the harness's own tests, and they are run against the example's config explicitly:
>
> ```console
> $ indri-script test -script ./example/tictactoe/config.json ./internal/services/scripttest/testdata/tictactoe
> ```
>
> For your own game, put the fixtures beside the config and `indri-script test ./mygame` works with
> no flags.

## Which transports reach a script action

> **This section describes the current state and is deliberately incomplete. Story 062, "Transport
> parity and observability for script actions", is in progress and will change it. Do not design
> against what is written here as though it were settled, and do not assume the shape of what
> replaces it.**

As of this build, an action a script declares is reachable over:

| Transport | Reaches a script action? |
|---|---|
| WebSocket (`/ws`) | **yes** — `boot.handleClientMessage` → `router.DispatchMessage`, which routes on the `action` field |
| WebRTC (`game` DataChannel) | **yes** — the adapter pipes messages into the same handler |
| REST (`POST /api/<action>`) | **no** |
| GraphQL (mutations) | **no** |
| SSE (`/events`) | n/a — push-only; an SSE client sends actions over REST or GraphQL, so **no** |

REST and GraphQL both expose a **fixed allow-list** of the built-in actions:
`internal/transport/rest/routes.go` registers a route per entry in its `routes` map, and
`internal/transport/graphql/schema.graphqls` declares one mutation per built-in. gqlgen generates a
resolver interface at build time, so per-script mutations cannot be added at boot — which is the
problem 062 exists to solve.

The parity tests do not close this gap either: `TestRestRoutesMatchRegisteredActions` and
`TestGraphQLMutationsMatchRegisteredActions` both build their registry from `registerHandlers` with an
injector carrying no Lua engine, so no script action is ever in the set they compare. That is listed
as an explicit acceptance criterion on 062.

**Practical consequence today:** a game whose verbs live in Lua is a WebSocket (or WebRTC) game. An
SSE-plus-REST client can register, log in, join and refresh, but cannot send `move`.

This section will be replaced when 062 lands.

## When you still need Go

A Go handler registered with `router.RegisterHandler` remains supported, and is deliberately kept as
an escape hatch. Nothing ships using it — `example/tictactoe` is config plus a Lua script on the
stock binary — and it is not the default. Reach for it when a game action needs something the host
API does not offer:

- writing outside the game document (a Mongo collection of your own, another service)
- changing **membership or host** — `checkInvariants` refuses those from Lua by design, and the Go
  built-ins that change them also maintain the session
- work that cannot fit the 100 ms invocation budget or that must not run under the game's lock
- reaching a service on the injector that has no capability equivalent

See [CLAUDE.md, "Adding a game action"](../CLAUDE.md#adding-a-game-action) for how to wire one, and
note that a Go action registered that way is WebSocket-only unless you add a REST route and a GraphQL
mutation yourself.

## Where the code is

| File | What lives there |
|---|---|
| `internal/services/lua/engine.go` | `indri.on`, the manifests, reserved names, the host table |
| `internal/services/lua/host_mutate.go` | `indri.mutate`, the invocation context, `scene:changed` |
| `internal/services/lua/host_io.go` | `indri.reply`, `indri.send`, the depth and fan-out caps |
| `internal/services/lua/host_timer.go` | `indri.after`, `indri.at`, `indri.cancel`, the nil-session contract |
| `internal/services/lua/hooks.go` | `indri.before`, `indri.after_action`, the auth-action refusal |
| `internal/services/lua/lifecycle.go` | the four events, the deferred queue, the `trigger` interface |
| `internal/services/lua/capability.go` | grants, host views, per-script scoping |
| `internal/services/lua/host_assets.go`, `host_http.go` | the two capabilities |
| `internal/services/lua/sandbox.go` | what is opened, what is removed, the string caps |
| `internal/services/lua/marshal.go` | the Go↔Lua value rules and key validation |
| `internal/services/lua/game.go` | `checkInvariants`, `applyLua`, `coerceEmpty` |
| `internal/services/lua/effects.go` | the ledger and the delivery guarantee |
| `internal/services/lua/lib/indri/game.lua` | the shipped helper library |
| `internal/handlers/actions/script/` | the Go bridge from the router to `Engine.Invoke` |
| `internal/services/scheduler/`, `internal/repo/schedule/` | the timer scheduler and its store |
| `internal/services/scripttest/`, `cmd/indri-script/` | the fixture harness |
| `example/tictactoe/` | the worked example: `config.json` + `game.lua` |

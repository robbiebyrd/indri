# Lua Lifecycle Hooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow game scripts in `config.json` to hook into framework lifecycle events (`join`, `leave`, `kick`, `inquire`) and trigger full game-state refreshes via `indri.refresh()` / `indri.refreshSelf()`.

**Architecture:** Lifecycle hooks register as additional handlers for existing router actions (framework handler always runs first due to registration order). An `InquireNotificationHandler` handles the `inquire` case without game context; all other actions use the extended `Handler`. Post-Mutate broadcasts fire after `GameRepo.Mutate` returns successfully, never inside the apply callback.

**Tech Stack:** Go, `github.com/yuin/gopher-lua`, internal `luahandler` package, `BroadcastService.Broadcast`, `GameService.Get`/`Sanitize`, `SessionService.Get`.

**Spec:** `docs/superpowers/specs/2026-09-22-lua-lifecycle-hooks-design.md`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/handlers/luahandler/handler.go` | Modify | `postMutateActions` struct; `buildCallerTable`; `registerIndriTable`; `executePostMutateActions`; refactored `gameAndTeamFromSession`; updated `Handle` |
| `internal/handlers/luahandler/inquire_handler.go` | Create | `InquireNotificationHandler` — msg-only Lua execution, no Mutate |
| `internal/handlers/luahandler/register.go` | Modify | Fork on `"inquire"` to use `InquireNotificationHandler` |
| `internal/handlers/luahandler/handler_test.go` | Create | Unit tests for `buildCallerTable`, `registerIndriTable`, `InquireNotificationHandler` |

No changes to `stdlib.go`, `runtime.go`, `runtime_test.go`, or anything outside `luahandler/`.

---

## Task 1: Enrich `caller` with `gameId` and `userId`

Refactor `gameAndTeamFromSession` to use `SessionService.Get` (returns the full `Session` struct in one call) and extract `buildCallerTable` as a pure, testable function.

**Files:**
- Modify: `internal/handlers/luahandler/handler.go`
- Create: `internal/handlers/luahandler/handler_test.go`

- [ ] **Step 1.1 — Create `handler_test.go` with a `fakeConn` and the failing `buildCallerTable` test**

```go
package luahandler

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// fakeConn implements transport.Conn for tests that need a connection
// but don't exercise the real transport.
type fakeConn struct {
	keys    map[string]any
	written [][]byte
	closed  bool
}

func newFakeConn() *fakeConn { return &fakeConn{keys: make(map[string]any)} }

func (f *fakeConn) Get(key string) (any, bool)   { v, ok := f.keys[key]; return v, ok }
func (f *fakeConn) Set(key string, value any)    { f.keys[key] = value }
func (f *fakeConn) UnSet(key string)             { delete(f.keys, key) }
func (f *fakeConn) Write(msg []byte) error       { f.written = append(f.written, msg); return nil }
func (f *fakeConn) Close() error                 { f.closed = true; return nil }
func (f *fakeConn) IsClosed() bool               { return f.closed }

func TestBuildCallerTable_AllFieldsPresent(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := buildCallerTable(L, "game-1", "team-2", "user-3")

	cases := map[string]string{
		"gameId": "game-1",
		"teamId": "team-2",
		"userId": "user-3",
	}
	for field, want := range cases {
		if got := tbl.RawGetString(field).String(); got != want {
			t.Errorf("%s: want %q, got %q", field, want, got)
		}
	}
}
```

- [ ] **Step 1.2 — Run test to verify it fails**

```bash
go test ./internal/handlers/luahandler/ -run TestBuildCallerTable -v
```

Expected: `FAIL — buildCallerTable undefined`

- [ ] **Step 1.3 — Add `buildCallerTable` and refactor `gameAndTeamFromSession` in `handler.go`**

Replace the existing `gameAndTeamFromSession` and the `callerTable` construction block with the following. The signature change (`teamId` → `gameId, teamId, userId`) must be reflected in `Handle` too.

```go
// buildCallerTable constructs the Lua caller table exposed to every handler script.
func buildCallerTable(L *lua.LState, gameId, teamId, userId string) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("gameId", lua.LString(gameId))
	t.RawSetString("teamId", lua.LString(teamId))
	t.RawSetString("userId", lua.LString(userId))
	return t
}

// gameAndTeamFromSession resolves the game, team, and user for the connection
// that sent the current message. Uses SessionService.Get to read all three
// fields in a single round trip.
func (h *Handler) gameAndTeamFromSession(s transport.Conn) (gameId, teamId, userId string, err error) {
	cs := connection.NewService(s, h.i.Transport)

	sessionId, err := cs.GetKeyAsString("sessionId")
	if err != nil {
		return "", "", "", err
	}
	if sessionId == nil {
		return "", "", "", fmt.Errorf("sessionId is nil")
	}

	session, err := h.i.SessionService.Get(*sessionId)
	if err != nil {
		return "", "", "", err
	}
	if session.GameID == nil || session.TeamID == nil || session.UserID == nil {
		return "", "", "", fmt.Errorf("session is not in a game/team")
	}

	return *session.GameID, *session.TeamID, *session.UserID, nil
}
```

In `Handle`, update the call site:

```go
gameId, teamId, userId, err := h.gameAndTeamFromSession(s)
```

And replace the existing `callerTable` block:

```go
L.SetGlobal("caller", buildCallerTable(L, gameId, teamId, userId))
```

- [ ] **Step 1.4 — Run test to verify it passes**

```bash
go test ./internal/handlers/luahandler/ -run TestBuildCallerTable -v
```

Expected: `PASS`

- [ ] **Step 1.5 — Run the full package to catch any regressions**

```bash
go test ./internal/handlers/luahandler/ -v
```

Expected: all existing tests pass.

- [ ] **Step 1.6 — Commit**

```bash
git add internal/handlers/luahandler/handler.go internal/handlers/luahandler/handler_test.go
git commit -m "feat(luahandler): enrich caller table with gameId and userId"
```

---

## Task 2: Add `postMutateActions` struct and `indri` global

Add the `indri.refresh()` and `indri.refreshSelf()` Lua functions. Flags set during Mutate are read after it returns; the actual sends happen once, against committed state.

**Files:**
- Modify: `internal/handlers/luahandler/handler.go`
- Modify: `internal/handlers/luahandler/handler_test.go`

- [ ] **Step 2.1 — Add failing tests for flag-setting to `handler_test.go`**

```go
func TestRegisterIndriTable_RefreshAllFlag(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	pma := &postMutateActions{}
	registerIndriTable(L, pma)

	if err := L.DoString(`indri.refresh()`); err != nil {
		t.Fatalf("indri.refresh(): %v", err)
	}
	if !pma.refreshAll {
		t.Error("expected refreshAll=true after indri.refresh()")
	}
	if pma.refreshSelf {
		t.Error("expected refreshSelf unchanged (false)")
	}
}

func TestRegisterIndriTable_RefreshSelfFlag(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	pma := &postMutateActions{}
	registerIndriTable(L, pma)

	if err := L.DoString(`indri.refreshSelf()`); err != nil {
		t.Fatalf("indri.refreshSelf(): %v", err)
	}
	if !pma.refreshSelf {
		t.Error("expected refreshSelf=true after indri.refreshSelf()")
	}
	if pma.refreshAll {
		t.Error("expected refreshAll unchanged (false)")
	}
}
```

- [ ] **Step 2.2 — Run tests to verify they fail**

```bash
go test ./internal/handlers/luahandler/ -run TestRegisterIndriTable -v
```

Expected: `FAIL — postMutateActions undefined` and `registerIndriTable undefined`

- [ ] **Step 2.3 — Add `postMutateActions`, `registerIndriTable`, and `executePostMutateActions` to `handler.go`**

Add after the imports section (new structs and functions; existing `Handle` wiring comes in the next step):

```go
// postMutateActions records side-effects requested by a Lua script that must
// fire after GameRepo.Mutate commits — never inside the apply callback.
// Allocated before Mutate so the same pointer is shared across retries.
type postMutateActions struct {
	refreshAll  bool
	refreshSelf bool
}

// registerIndriTable registers the indri.refresh() and indri.refreshSelf()
// globals into L. Both functions set flags on pma; the actual network sends
// happen after Mutate returns via executePostMutateActions.
func registerIndriTable(L *lua.LState, pma *postMutateActions) {
	indri := L.NewTable()
	L.SetField(indri, "refresh", L.NewFunction(func(L *lua.LState) int {
		pma.refreshAll = true
		return 0
	}))
	L.SetField(indri, "refreshSelf", L.NewFunction(func(L *lua.LState) int {
		pma.refreshSelf = true
		return 0
	}))
	L.SetGlobal("indri", indri)
}

// executePostMutateActions runs any refresh side-effects requested via the
// indri global. Errors are logged and discarded — the mutation already
// committed, so a fan-out failure must not fail the overall operation.
func executePostMutateActions(pma *postMutateActions, gameId string, s transport.Conn, i *injector.Injector) {
	if !pma.refreshAll && !pma.refreshSelf {
		return
	}

	g, err := i.GameService.Get(gameId)
	if err != nil {
		log.Printf("indri refresh: could not fetch game %v: %v", gameId, err)
		return
	}
	sanitized := i.GameService.Sanitize(g)

	if pma.refreshAll {
		if err := i.BroadcastService.Broadcast(&gameId, nil, sanitized); err != nil {
			log.Printf("indri refresh: broadcast failed for game %v: %v", gameId, err)
		}
	}

	if pma.refreshSelf {
		jsonData, err := json.Marshal(sanitized)
		if err != nil {
			log.Printf("indri refreshSelf: marshal failed: %v", err)
			return
		}
		cs := connection.NewService(s, i.Transport)
		if err := cs.Write(jsonData); err != nil {
			log.Printf("indri refreshSelf: write failed: %v", err)
		}
	}
}
```

Add `"encoding/json"` and `"log"` to the import block in `handler.go` if not already present.

- [ ] **Step 2.4 — Wire `pma` and `indri` into `Handle`**

Update `Handle` so that:
1. `pma` is allocated before `Mutate` (outside the closure, so it survives retries — see spec for retry-flag semantics)
2. `registerIndriTable` is called inside the Mutate closure (after `buildCallerTable`)
3. `executePostMutateActions` is called after `Mutate` returns (exactly once, against committed state)

```go
func (h *Handler) Handle(s transport.Conn, decodedMsg map[string]interface{}) error {
	gameId, teamId, userId, err := h.gameAndTeamFromSession(s)
	if err != nil {
		return err
	}

	pma := &postMutateActions{}

	if err := h.i.GameRepo.Mutate(gameId, func(g *models.Game) error {
		L := lua.NewState()
		defer L.Close()

		if err := L.DoString(stdlib); err != nil {
			return fmt.Errorf("loading stdlib: %w", err)
		}

		gameTable, err := gameToLua(L, g)
		if err != nil {
			return fmt.Errorf("building game table: %w", err)
		}
		L.SetGlobal("game", gameTable)

		msgTable := L.NewTable()
		for k, v := range decodedMsg {
			msgTable.RawSetString(k, jsonToLua(L, v))
		}
		L.SetGlobal("msg", msgTable)

		L.SetGlobal("caller", buildCallerTable(L, gameId, teamId, userId))

		initialTable, err := buildInitialTable(L, h.i.Script)
		if err != nil {
			return fmt.Errorf("building initial table: %w", err)
		}
		L.SetGlobal("initial", initialTable)

		registerIndriTable(L, pma)

		if err := L.DoString(h.script); err != nil {
			return fmt.Errorf("running handler script: %w", err)
		}

		return applyLuaToGame(L, gameTable, g)
	}); err != nil {
		return err
	}

	executePostMutateActions(pma, gameId, s, h.i)
	return nil
}
```

- [ ] **Step 2.5 — Run tests to verify they pass**

```bash
go test ./internal/handlers/luahandler/ -run TestRegisterIndriTable -v
```

Expected: `PASS`

- [ ] **Step 2.6 — Run full package**

```bash
go test ./internal/handlers/luahandler/ -v
```

Expected: all tests pass.

- [ ] **Step 2.7 — Commit**

```bash
git add internal/handlers/luahandler/handler.go internal/handlers/luahandler/handler_test.go
git commit -m "feat(luahandler): add indri.refresh() and indri.refreshSelf() with post-Mutate execution"
```

---

## Task 3: Create `InquireNotificationHandler`

A thin handler that runs a Lua script with only `msg` available — no game context, no Mutate.

**Files:**
- Create: `internal/handlers/luahandler/inquire_handler.go`
- Modify: `internal/handlers/luahandler/handler_test.go`

- [ ] **Step 3.1 — Add failing tests for `InquireNotificationHandler` to `handler_test.go`**

```go
func TestInquireNotificationHandler_MsgIsAccessible(t *testing.T) {
	h := &InquireNotificationHandler{
		script: `assert(msg.inquiryType == "game", "wrong inquiry type: " .. tostring(msg.inquiryType))`,
	}
	err := h.Handle(newFakeConn(), map[string]interface{}{"inquiryType": "game"})
	if err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}
}

func TestInquireNotificationHandler_ScriptErrorPropagates(t *testing.T) {
	h := &InquireNotificationHandler{
		script: `error("deliberate error")`,
	}
	err := h.Handle(newFakeConn(), map[string]interface{}{})
	if err == nil {
		t.Error("expected error from script, got nil")
	}
}

func TestInquireNotificationHandler_NoGameGlobal(t *testing.T) {
	h := &InquireNotificationHandler{
		script: `assert(game == nil, "game should not be set")`,
	}
	err := h.Handle(newFakeConn(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected game to be nil but got error: %v", err)
	}
}
```

- [ ] **Step 3.2 — Run tests to verify they fail**

```bash
go test ./internal/handlers/luahandler/ -run TestInquireNotificationHandler -v
```

Expected: `FAIL — InquireNotificationHandler undefined`

- [ ] **Step 3.3 — Create `inquire_handler.go`**

```go
package luahandler

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/transport"
)

// InquireNotificationHandler runs a Lua script as a fire-and-forget
// notification when an inquire action completes. It has no game context:
// only msg is available and game state cannot be mutated.
type InquireNotificationHandler struct {
	script string
}

func NewInquireNotificationHandler(script string) *InquireNotificationHandler {
	return &InquireNotificationHandler{script: script}
}

func (h *InquireNotificationHandler) Handle(s transport.Conn, decodedMsg map[string]interface{}) error {
	L := lua.NewState()
	defer L.Close()

	if err := L.DoString(stdlib); err != nil {
		return fmt.Errorf("loading stdlib: %w", err)
	}

	msgTable := L.NewTable()
	for k, v := range decodedMsg {
		msgTable.RawSetString(k, jsonToLua(L, v))
	}
	L.SetGlobal("msg", msgTable)

	if err := L.DoString(h.script); err != nil {
		return fmt.Errorf("running inquire notification script: %w", err)
	}

	return nil
}
```

- [ ] **Step 3.4 — Run tests to verify they pass**

```bash
go test ./internal/handlers/luahandler/ -run TestInquireNotificationHandler -v
```

Expected: `PASS`

- [ ] **Step 3.5 — Run full package**

```bash
go test ./internal/handlers/luahandler/ -v
```

Expected: all tests pass.

- [ ] **Step 3.6 — Commit**

```bash
git add internal/handlers/luahandler/inquire_handler.go internal/handlers/luahandler/handler_test.go
git commit -m "feat(luahandler): add InquireNotificationHandler for inquire lifecycle notification"
```

---

## Task 4: Update `register.go` to fork on `inquire`

Route `"inquire"` entries in `Script.Handlers` to `InquireNotificationHandler`; all others use `Handler`.

**Files:**
- Modify: `internal/handlers/luahandler/register.go`

- [ ] **Step 4.1 — Update `Register` in `register.go`**

Replace the body of `Register`:

```go
func Register(i *injector.Injector) {
	if i.Script == nil {
		return
	}
	for action, script := range i.Script.Handlers {
		if action == "inquire" {
			router.RegisterHandler("script:"+action, action, NewInquireNotificationHandler(script))
		} else {
			router.RegisterHandler("script:"+action, action, New(i, script))
		}
	}
}
```

- [ ] **Step 4.2 — Verify the package builds and all tests pass**

```bash
go build ./internal/handlers/luahandler/ && go test ./internal/handlers/luahandler/ -v
```

Expected: build succeeds, all tests pass.

- [ ] **Step 4.3 — Run the full test suite**

```bash
go test -race ./...
```

Expected: all packages pass with no data races.

- [ ] **Step 4.4 — Commit**

```bash
git add internal/handlers/luahandler/register.go
git commit -m "feat(luahandler): route inquire to InquireNotificationHandler in Register"
```

---

## Verification

After all tasks are complete, confirm the feature works end-to-end by adding lifecycle hooks to the tictactoe example script. `example/tictactoe.json` is an existing untracked file in the repo — run `git status` to confirm it is present before editing.

Add to `handlers` in `example/tictactoe.json`:

```json
"join": "-- caller.userId and caller.gameId are available\nlocal uid = caller.userId\nif uid ~= nil and uid ~= \"\" then\n  indri.refresh()\nend\n",
"inquire": "-- msg is the only global; this is purely a notification\nlocal t = msg.inquiryType\n"
```

Run:

```bash
go run ./cmd/server -script ./example/tictactoe.json
```

Expected: server starts without error. Connecting a client and issuing `join` and `inquire` messages should not crash the server. Check server logs for any Lua errors.

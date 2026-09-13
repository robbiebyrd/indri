package lua

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// echoScript reports everything one invocation was handed, by raising it as a
// script error.
//
// Raising is the only way a handler can speak at this stage — replying is a
// host function that does not exist yet — and it is enough: an ordinary script
// error unwinds cleanly, so the state it ran on is reused by the next call,
// which is exactly the condition the isolation test below needs.
const echoScript = `
indri.on("echo", function(req)
  local s = req.session or {}

  error(table.concat({
    req.action,
    tostring(req.payload.word),
    tostring(s.userId),
    tostring(s.gameId),
    tostring(s.teamId),
    tostring(s.token),
    tostring(s.id),
  }, "|"), 0)
end)
`

// invokeEcho runs the echo handler and returns the pipe-separated report it
// raised, with the correlation id the engine appends removed.
func invokeEcho(t *testing.T, e *Engine, req actions.Request) string {
	t.Helper()

	_, err := splitScriptError(e.Invoke(context.Background(), "echo", req))
	if err == nil {
		t.Fatal("the echo handler returned without raising its report")
	}

	// echoScript raises at level 0, so its message carries no file and line of
	// its own; everything up to the bracketed correlation id is the report.
	at := strings.LastIndex(err.Error(), " [")
	if at < 0 {
		t.Fatalf("the error %q carries no correlation id", err.Error())
	}

	return err.Error()[:at]
}

func stringPtr(s string) *string {
	return &s
}

// TestInvoke_HandsTheRequestToTheRegisteredHandler is the whole point of the
// entry point: the action the router dispatched, the payload the client sent
// and the session the transport authenticated all reach the script, and
// nothing else does.
func TestInvoke_HandsTheRequestToTheRegisteredHandler(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "echo.lua", echoScript))

	tests := map[string]struct {
		req  actions.Request
		want string
	}{
		"payload and a fully bound session": {
			req: actions.Request{
				Payload: map[string]interface{}{"word": "hello"},
				Session: &models.Session{
					Token:  "the-bearer-token",
					UserID: stringPtr("player-1"),
					GameID: stringPtr("game-1"),
					TeamID: stringPtr("team-1"),
				},
			},
			// The trailing nils are the two fields a script may not have: the
			// bearer token a client resumes with, and the connection key every
			// targeted broadcast filters on.
			want: "echo|hello|player-1|game-1|team-1|nil|nil",
		},
		"a session that has not joined a game": {
			req: actions.Request{
				Payload: map[string]interface{}{"word": "hello"},
				Session: &models.Session{UserID: stringPtr("player-1")},
			},
			want: "echo|hello|player-1|nil|nil|nil|nil",
		},
		"no session at all": {
			req:  actions.Request{Payload: map[string]interface{}{"word": "hello"}},
			want: "echo|hello|nil|nil|nil|nil|nil",
		},
		"no payload at all": {
			req:  actions.Request{},
			want: "echo|nil|nil|nil|nil|nil|nil",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := invokeEcho(t, e, test.req); got != test.want {
				t.Errorf("the handler saw %q, want %q", got, test.want)
			}
		})
	}
}

// TestInvoke_RefusesAnActionNoScriptDeclared guards the wiring: the router is
// only ever given the manifest, so an action arriving here that no state
// registered means the two have drifted apart, and answering it silently would
// hide that.
func TestInvoke_RefusesAnActionNoScriptDeclared(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "echo.lua", echoScript))

	_, err := e.Invoke(context.Background(), "move", actions.Request{})
	requireErrorMentions(t, err, "move", "no lua handler")
}

// TestInvoke_RefusesAPayloadItCannotConvert keeps a value with no Lua form from
// reaching a script half-converted. A missing payload key means "absent" to a
// script, so a silently dropped field would be indistinguishable from one the
// client never sent.
func TestInvoke_RefusesAPayloadItCannotConvert(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "echo.lua", echoScript))

	tests := map[string]map[string]interface{}{
		"a value with no lua form": {"stream": make(chan int)},
		"a key that would forge a delta path": {
			"players.p1.privateData": "forged",
		},
	}

	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := e.Invoke(context.Background(), "echo", actions.Request{Payload: payload})
			requireErrorMentions(t, err, "echo")
		})
	}
}

// TestInvoke_StopsAScriptAtItsDeadline proves the caller's deadline is what
// bounds an invocation, and that the state it interrupted is discarded rather
// than handed to the next player: the loop below never unwound its own stack.
func TestInvoke_StopsAScriptAtItsDeadline(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "spin.lua", `indri.on("spin", function(req) while true do end end)`))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := splitScriptError(e.Invoke(ctx, "spin", actions.Request{})); err == nil {
		t.Fatal("the infinite loop returned without an error")
	}

	if ctx.Err() == nil {
		t.Fatal("the script failed before its deadline expired")
	}

	if got := idleCount(e.pool); got != 0 {
		t.Errorf("the pool holds %d idle states, want the interrupted one discarded", got)
	}
}

// TestInvoke_GivesEachInvocationItsOwnGlobals is what makes a pooled state safe
// to reuse. Both calls below land on the same state — an ordinary script error
// does not spoil it — so a counter surviving into the second call would mean
// one player's move could be seen by the next.
func TestInvoke_GivesEachInvocationItsOwnGlobals(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "count.lua", `
indri.on("count", function(req)
  counter = (counter or 0) + 1
  error("counter=" .. counter, 0)
end)
`))

	for attempt := 1; attempt <= 2; attempt++ {
		_, err := splitScriptError(e.Invoke(context.Background(), "count", actions.Request{}))
		requireErrorMentions(t, err, "counter=1")
	}
}

// TestInvoke_AfterCloseIsRefused keeps a request arriving during shutdown from
// resurrecting the pool.
func TestInvoke_AfterCloseIsRefused(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "echo.lua", echoScript))
	e.Close()

	if _, err := e.Invoke(context.Background(), "echo", actions.Request{}); err == nil {
		t.Fatal("the engine served an invocation after it was closed")
	}
}

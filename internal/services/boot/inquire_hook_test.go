package boot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/inquire"
	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
	"github.com/robbiebyrd/indri/internal/handlers/router"
)

// An inquiry notification — a script told that someone asked about games — is
// an after hook on inquire. It sees what was asked, runs once the inquiry has
// been answered, and cannot write: an inquiry belongs to no game.
func TestInquireNotification_RunsAfterTheInquiryAndCannotWrite(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	f.Game(t, "ASKED")
	f.I.LuaEngine = scriptEngine(t, `
indri.after_action("inquire", function(req)
  local wrote = pcall(indri.mutate, function(state) return state end)
  indri.reply({at = "after", inquiry = tostring(req.payload.inquiry), code = tostring(req.payload.code), wrote = wrote})
end)`)

	router.Reset()
	t.Cleanup(router.Reset)

	registerScriptHooks(f.I)
	router.RegisterHandler("inquire", "inquire", inquire.New(f.I))

	result, err := router.Dispatch(context.Background(), f.Player(t, "ada"), "inquire",
		map[string]interface{}{"inquiryType": "game", "inquiry": "gameInfo", "code": "ASKED"})
	if err != nil {
		t.Fatalf("inquire: %v", err)
	}

	if len(result.Responses) != 2 {
		t.Fatalf("inquire answered %d frames, want the answer then the hook's: %q", len(result.Responses), result.Responses)
	}

	if !strings.Contains(string(result.Responses[0]), "ASKED") {
		t.Fatalf("the first frame is %s, want the inquiry's own answer", result.Responses[0])
	}

	var hook map[string]interface{}
	if err := json.Unmarshal(result.Responses[1], &hook); err != nil {
		t.Fatalf("the hook's frame %s: %v", result.Responses[1], err)
	}

	if hook["at"] != "after" || hook["inquiry"] != "gameInfo" || hook["code"] != "ASKED" {
		t.Fatalf("the hook saw %v, want the inquiry it followed", hook)
	}

	if hook["wrote"] != false {
		t.Fatalf("the inquiry notification could write to a game: %v", hook)
	}
}

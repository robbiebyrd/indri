package create

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/join"
	"github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

func TestCreate_SeatsTheCreatorAsHost(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	ada := f.Player(t, "ada")

	if _, err := f.Do(New(f.I), ada, map[string]interface{}{"code": "NEWGAME", "teamId": "red"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	session := f.Session(t, ada.ID)
	if session.GameID == nil || session.SlotID == nil {
		t.Fatalf("the creator's session names game %v, slot %v", session.GameID, session.SlotID)
	}

	if p := f.GameByID(t, *session.GameID).Players[*session.SlotID]; p.UserID != *ada.UserID || !p.Host {
		t.Fatalf("the creator's slot is %+v, want them seated as host", p)
	}
}

// A seat that cannot be given must not leave a game behind with nobody in it.
func TestCreate_RefusesATeamTheScriptDoesNotHaveBeforeMakingTheGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())

	if _, err := f.Do(New(f.I), f.Player(t, "ada"), map[string]interface{}{"code": "NEWGAME", "teamId": "green"}); err == nil {
		t.Fatal("creating a game on a team the script does not have succeeded")
	}

	if f.I.GameService.Exists("NEWGAME") {
		t.Fatal("the refused create still made the game")
	}
}

func TestCreate_FreesTheCreatorsSeatInTheirPreviousGame(t *testing.T) {
	f := handlertest.New(t, handlertest.TwoTeams())
	first := f.Game(t, "FIRST")
	ada := f.Player(t, "ada")

	if _, err := f.Do(join.New(f.I), ada, map[string]interface{}{"code": "FIRST", "teamId": "red"}); err != nil {
		t.Fatalf("joining the first game: %v", err)
	}

	oldSlot := *f.Session(t, ada.ID).SlotID

	if _, err := f.Do(New(f.I), f.Session(t, ada.ID), map[string]interface{}{"code": "NEWGAME", "teamId": "blue"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if holder := f.GameByID(t, first.ID).Players[oldSlot].UserID; holder != "" {
		t.Fatalf("ada still holds slot %s in the game she left by creating another", oldSlot)
	}
}

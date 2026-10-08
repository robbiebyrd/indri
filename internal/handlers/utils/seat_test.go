package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// seatStore is the real in-memory game store under the names the game service
// gives these two calls, so seating runs the store's own slot rules.
type seatStore struct{ *gameRepo.MemoryStore }

func (s seatStore) ConnectPlayer(id, teamId, userId, displayName string) (string, error) {
	slot, _, err := s.AssignSlot(id, teamId, userId, displayName)

	return slot, err
}

func (s seatStore) GetJSONBytes(id string, _ string) (*[]byte, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(g)

	return &b, err
}

type userDirectory map[string]*models.User

func (d userDirectory) Get(id string) (*models.User, error) {
	u, ok := d[id]
	if !ok {
		return nil, fmt.Errorf("no user %v", id)
	}

	return u, nil
}

// sessionLog keeps every session update, so a test can see what the caller's
// session was told.
type sessionLog map[string]*models.UpdateSession

func (l sessionLog) Update(sessionId string, u *models.UpdateSession) error {
	l[sessionId] = u

	return nil
}

// seatFixture is a game with one two-slot team, red, and a user to seat in it.
func seatFixture(t *testing.T) (seatStore, userDirectory, sessionLog, *models.Session, string) {
	t.Helper()

	store := seatStore{gameRepo.NewMemoryStore(context.Background(), lock.NewInProcess(), nil)}

	g, err := store.New("seat-test", &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}},
	}, false)
	if err != nil {
		t.Fatalf("creating game: %v", err)
	}

	users := userDirectory{"u1": {Name: "Ada"}}
	session := &models.Session{ID: "session-1", UserID: ref("u1")}

	return store, users, sessionLog{}, session, g.ID
}

func TestSeatCaller_RecordsTheSlotOnTheSession(t *testing.T) {
	store, users, sessions, session, gameId := seatFixture(t)

	keyframe, err := SeatCaller(store, users, sessions, session, gameId, "red", "")
	if err != nil {
		t.Fatalf("SeatCaller: %v", err)
	}

	g, err := store.Get(gameId)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	update := sessions[session.ID]
	if update == nil {
		t.Fatal("the session was not told about the seat")
	}

	if update.GameID != gameId || update.TeamID != "red" || update.UserID != "u1" {
		t.Errorf("session update = %+v, want game %v, team red, user u1", update, gameId)
	}

	if slot := g.Players[update.SlotID]; slot.UserID != "u1" || slot.Name != "Ada" {
		t.Errorf("the session names slot %q, which holds %+v; want u1 seated as Ada", update.SlotID, slot)
	}

	var sent models.Game
	if err := json.Unmarshal(*keyframe, &sent); err != nil {
		t.Fatalf("decoding the keyframe: %v", err)
	}

	if sent.Players[update.SlotID].UserID != "u1" {
		t.Errorf("the keyframe was taken before the caller was seated: %v", sent.Players)
	}
}

func TestSeatCaller_UsesTheDisplayName(t *testing.T) {
	store, users, sessions, session, gameId := seatFixture(t)
	users["u1"].DisplayName = ref("Countess")

	if _, err := SeatCaller(store, users, sessions, session, gameId, "red", ""); err != nil {
		t.Fatalf("SeatCaller: %v", err)
	}

	g, _ := store.Get(gameId)
	if name := g.Players[sessions[session.ID].SlotID].Name; name != "Countess" {
		t.Errorf("seated as %q, want the display name Countess", name)
	}
}

// A caller who could not be seated must not have a session claiming they were:
// it would name a game they hold no slot in.
func TestSeatCaller_LeavesTheSessionAloneWhenNoSlotIsFree(t *testing.T) {
	for name, team := range map[string]string{"a full team": "red", "a team the game does not have": "green"} {
		t.Run(name, func(t *testing.T) {
			store, users, sessions, session, gameId := seatFixture(t)

			if team == "red" {
				for _, other := range []string{"u2", "u3"} {
					if _, _, err := store.AssignSlot(gameId, "red", other, other); err != nil {
						t.Fatalf("filling red: %v", err)
					}
				}
			}

			if _, err := SeatCaller(store, users, sessions, session, gameId, team, ""); err == nil {
				t.Fatal("SeatCaller succeeded with no slot to give")
			}

			if update, told := sessions[session.ID]; told {
				t.Errorf("the session was updated to %+v for a caller who holds no slot", update)
			}
		})
	}
}

func TestHoldsSeat(t *testing.T) {
	g := &models.Game{Players: map[string]models.Player{"p0": {UserID: "u1"}, "p1": {}}}

	for name, tc := range map[string]struct {
		session *models.Session
		want    bool
	}{
		"the holder":                    {&models.Session{UserID: ref("u1"), SlotID: ref("p0")}, true},
		"someone else's slot":           {&models.Session{UserID: ref("u2"), SlotID: ref("p0")}, false},
		"a free slot":                   {&models.Session{UserID: ref("u1"), SlotID: ref("p1")}, false},
		"a slot the game does not have": {&models.Session{UserID: ref("u1"), SlotID: ref("p9")}, false},
		"no slot":                       {&models.Session{UserID: ref("u1")}, false},
		"no user":                       {&models.Session{SlotID: ref("p1")}, false},
		"an empty user on a free slot":  {&models.Session{UserID: ref(""), SlotID: ref("p1")}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := HoldsSeat(g, tc.session); got != tc.want {
				t.Fatalf("HoldsSeat = %v, want %v", got, tc.want)
			}
		})
	}
}

// seatedFixture is a game with u1 seated on red, and u1's session naming that
// seat, all in real in-memory stores.
func seatedFixture(t *testing.T) (seatStore, *sessionRepo.MemoryStore, *models.Session, *models.Game) {
	t.Helper()

	store, _, _, _, gameId := seatFixture(t)

	slot, err := store.ConnectPlayer(gameId, "red", "u1", "Ada")
	if err != nil {
		t.Fatalf("seating u1: %v", err)
	}

	sessions := sessionRepo.NewMemoryStore()

	session, err := sessions.New(models.CreateSession{UserID: "u1", GameID: gameId, TeamID: "red", SlotID: slot})
	if err != nil {
		t.Fatalf("creating u1's session: %v", err)
	}

	g, err := store.Get(gameId)
	if err != nil {
		t.Fatalf("reading the game: %v", err)
	}

	return store, sessions, session, g
}

func TestDropLostSeat_KeepsASessionThatHoldsItsSeat(t *testing.T) {
	_, sessions, session, g := seatedFixture(t)

	held, err := DropLostSeat(sessions, session, g)
	if err != nil || !held {
		t.Fatalf("DropLostSeat = %v, %v; want held", held, err)
	}

	if got, _ := sessions.Get(session.ID); got.GameID == nil {
		t.Fatal("a session that holds its seat was taken out of the game")
	}
}

// A session that outlived its seat — the player was kicked, or left from
// elsewhere — must stop naming the game, or it keeps receiving its broadcasts.
func TestDropLostSeat_TakesASessionThatLostItsSeatOutOfTheGame(t *testing.T) {
	store, sessions, session, g := seatedFixture(t)

	if err := store.RemovePlayer(g.ID, *session.SlotID, "u1"); err != nil {
		t.Fatalf("freeing the seat: %v", err)
	}

	g, _ = store.Get(g.ID)

	held, err := DropLostSeat(sessions, session, g)
	if err != nil || held {
		t.Fatalf("DropLostSeat = %v, %v; want not held", held, err)
	}

	if got, _ := sessions.Get(session.ID); got.GameID != nil || got.SlotID != nil {
		t.Fatalf("the session still names game %v, slot %v", got.GameID, got.SlotID)
	}
}

func TestVacateSeat_FreesTheSlotTheSessionNames(t *testing.T) {
	store, _, session, g := seatedFixture(t)

	if err := VacateSeat(store, session); err != nil {
		t.Fatalf("VacateSeat: %v", err)
	}

	got, _ := store.Get(g.ID)
	if holder := got.Players[*session.SlotID].UserID; holder != "" {
		t.Fatalf("slot %s is still held by %q", *session.SlotID, holder)
	}
}

// A seat already taken by someone else is not the session's to free.
func TestVacateSeat_LeavesSomeoneElsesSeatAlone(t *testing.T) {
	store, _, session, g := seatedFixture(t)

	if err := store.RemovePlayer(g.ID, *session.SlotID, "u1"); err != nil {
		t.Fatalf("freeing the seat: %v", err)
	}

	if _, err := store.ConnectPlayer(g.ID, "red", "u2", "Bea"); err != nil {
		t.Fatalf("seating u2: %v", err)
	}

	if err := VacateSeat(store, session); err == nil {
		t.Fatal("VacateSeat freed a seat its session's user no longer holds")
	}

	got, _ := store.Get(g.ID)
	if holder := got.Players[*session.SlotID].UserID; holder != "u2" {
		t.Fatalf("slot %s is held by %q, want still u2", *session.SlotID, holder)
	}
}

// failingFree is the real store, except that freeing a seat fails the way a
// store under contention or losing its connection does.
type failingFree struct{ seatStore }

func (failingFree) RemovePlayer(string, string, string) error {
	return errors.New("the game store is unavailable")
}

// movingFixture seats u1 in a first game and returns the session naming that
// seat, with a second game to move to.
func movingFixture(t *testing.T) (seatStore, userDirectory, sessionLog, *models.Session, string) {
	t.Helper()

	store, users, sessions, _, first := seatFixture(t)

	slot, err := store.ConnectPlayer(first, "red", "u1", "Ada")
	if err != nil {
		t.Fatalf("seating u1 in the first game: %v", err)
	}

	second, err := store.New("seat-test-2", &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}},
	}, false)
	if err != nil {
		t.Fatalf("creating the second game: %v", err)
	}

	session := &models.Session{ID: "session-1", UserID: ref("u1"), GameID: ref(first), TeamID: ref("red"), SlotID: ref(slot)}

	return store, users, sessions, session, second.ID
}

// A move whose old seat could not be freed must say so: the player would
// otherwise hold a seat in a game they left, for good.
func TestSeatCaller_ReportsAnOldSeatItCouldNotFree(t *testing.T) {
	store, users, sessions, session, second := movingFixture(t)

	keyframe, err := SeatCaller(failingFree{store}, users, sessions, session, second, "red", "")
	if err == nil {
		t.Fatal("a move whose old seat could not be freed reported success")
	}

	if keyframe == nil {
		t.Fatal("the move itself succeeded, so the caller still needs the new game's keyframe")
	}

	if update := sessions[session.ID]; update == nil || update.GameID != second {
		t.Fatalf("the session was not moved to the game the player is now seated in: %+v", update)
	}
}

// An old seat already lost — to a kick, or another connection — has nothing to
// free, and is no failure.
func TestSeatCaller_AnOldSeatAlreadyGoneIsNotAnError(t *testing.T) {
	store, users, sessions, session, second := movingFixture(t)

	if err := store.RemovePlayer(*session.GameID, *session.SlotID, "u1"); err != nil {
		t.Fatalf("freeing the old seat behind the session's back: %v", err)
	}

	if _, err := SeatCaller(store, users, sessions, session, second, "red", ""); err != nil {
		t.Fatalf("SeatCaller with the old seat already gone = %v, want nil", err)
	}
}

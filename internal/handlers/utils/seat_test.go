package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// seatStore is the real in-memory game store under the names the game service
// gives these two calls, so seating runs the store's own slot rules.
type seatStore struct{ *gameRepo.MemoryStore }

func (s seatStore) ConnectPlayer(id, teamId, userId, displayName string) (string, error) {
	slot, _, err := s.AssignSlot(id, teamId, userId, displayName)

	return slot, err
}

func (s seatStore) GetJSONBytes(id string) (*[]byte, error) {
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

	keyframe, err := SeatCaller(store, users, sessions, session, gameId, "red")
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

	if _, err := SeatCaller(store, users, sessions, session, gameId, "red"); err != nil {
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

			if _, err := SeatCaller(store, users, sessions, session, gameId, team); err == nil {
				t.Fatal("SeatCaller succeeded with no slot to give")
			}

			if update, told := sessions[session.ID]; told {
				t.Errorf("the session was updated to %+v for a caller who holds no slot", update)
			}
		})
	}
}

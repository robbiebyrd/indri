package register

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

func ptr(s string) *string {
	return &s
}

// Every player's user id reaches everyone in their game, so it is no secret. A
// registration naming that id must still get an account of its own: an
// account whose id the client chose could replace the victim's, and with it
// inherit their live session.
func TestRegister_CannotChooseTheUserID(t *testing.T) {
	store := userRepo.NewMemoryStore()

	victim, err := store.New(models.CreateUser{Email: "victim@example.test", Name: "Victim", Password: ptr("victim-hash")})
	if err != nil {
		t.Fatalf("creating the victim: %v", err)
	}

	msg, err := remarshal(map[string]interface{}{
		"id":       victim.ID,
		"email":    "attacker@example.test",
		"password": "attacker-hash",
		"name":     "Attacker",
	})
	if err != nil {
		t.Fatalf("decoding the registration: %v", err)
	}

	attacker, err := store.New(*msg)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	if attacker.ID == victim.ID {
		t.Fatalf("the registration was given the id it asked for, %s", victim.ID)
	}

	got, err := store.Get(victim.ID)
	if err != nil {
		t.Fatalf("the victim's account is gone: %v", err)
	}

	if got.Email != "victim@example.test" || got.Password == nil || *got.Password != "victim-hash" {
		t.Fatalf("the victim's account was replaced: %+v", got)
	}
}

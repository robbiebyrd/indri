// Package handlertest runs action handlers against the real services over
// in-memory stores. A handler test built on it exercises what production runs
// — the slot rules, the version fence, session lookups, password hashing —
// rather than fakes that agree with whatever the handler expects.
package handlertest

import (
	"context"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	authService "github.com/robbiebyrd/indri/internal/services/authentication"
	"github.com/robbiebyrd/indri/internal/services/events"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
)

// Fixture is an injector whose stores live in memory and whose services are
// the production ones.
type Fixture struct {
	I *injector.Injector

	// credentials are each player's login, by user id.
	credentials map[string]map[string]interface{}
}

// TwoTeams is a script with teams red and blue of two slots each, enough to
// join, move between teams, and fill one.
func TwoTeams() *models.Script {
	return &models.Script{
		Config: models.Config{MaxPlayersPerTeam: 2},
		Teams:  map[string]models.Team{"red": {Name: "Red"}, "blue": {Name: "Blue"}},
	}
}

// New builds a fixture whose games are stamped from script.
func New(t *testing.T, script *models.Script) *Fixture {
	t.Helper()

	ctx := context.Background()

	games := gameRepo.NewMemoryStore(ctx, lock.NewInProcess(), events.NewInProcess())
	users := userRepo.NewMemoryStore()
	sessions := sessionRepo.NewMemoryStore()

	gs, err := gameService.NewService(games, script)
	if err != nil {
		t.Fatalf("building the game service: %v", err)
	}

	us, err := userService.NewService(users)
	if err != nil {
		t.Fatalf("building the user service: %v", err)
	}

	as, err := authService.NewService(users, sessions)
	if err != nil {
		t.Fatalf("building the auth service: %v", err)
	}

	return &Fixture{credentials: map[string]map[string]interface{}{}, I: &injector.Injector{
		ReposInjector: &injector.ReposInjector{GameRepo: games, UserRepo: users, SessionRepo: sessions},
		ServicesInjector: &injector.ServicesInjector{
			GameService:    gs,
			UserService:    us,
			AuthService:    as,
			SessionService: sessionService.NewService(sessions),
		},
		Script:        script,
		GlobalContext: ctx,
	}}
}

// Player registers a user called name and logs them in, as a client would,
// returning their session.
func (f *Fixture) Player(t *testing.T, name string) *models.Session {
	t.Helper()

	email := name + "-" + ids.New() + "@example.test"
	password := "password-" + name

	if _, err := f.I.UserService.New(models.CreateUser{Email: email, Name: name, Password: &password}); err != nil {
		t.Fatalf("registering %s: %v", name, err)
	}

	session, err := f.I.AuthService.Authenticate(&email, &password)
	if err != nil {
		t.Fatalf("logging %s in: %v", name, err)
	}

	f.credentials[*session.UserID] = map[string]interface{}{"email": email, "password": password}

	return session
}

// Credentials is the login payload of the player whose session this is.
func (f *Fixture) Credentials(session *models.Session) map[string]interface{} {
	return f.credentials[*session.UserID]
}

// Game creates a game with code.
func (f *Fixture) Game(t *testing.T, code string) *models.Game {
	t.Helper()

	g, err := f.I.GameService.New(code, false)
	if err != nil {
		t.Fatalf("creating game %s: %v", code, err)
	}

	return g
}

// Session re-reads a session from the store, as the transport does before
// every request: a handler's writes to it are only visible this way.
func (f *Fixture) Session(t *testing.T, id string) *models.Session {
	t.Helper()

	session, err := f.I.SessionService.Get(id)
	if err != nil {
		t.Fatalf("reading session %s: %v", id, err)
	}

	return session
}

// GameByID reads a game back from the store.
func (f *Fixture) GameByID(t *testing.T, id string) *models.Game {
	t.Helper()

	g, err := f.I.GameService.Get(id)
	if err != nil {
		t.Fatalf("reading game %s: %v", id, err)
	}

	return g
}

// Do runs handler for the caller holding session with payload. A nil session
// is an unauthenticated caller.
func (f *Fixture) Do(
	handler interface {
		Handle(actions.Request) (actions.Result, error)
	},
	session *models.Session,
	payload map[string]interface{},
) (actions.Result, error) {
	return handler.Handle(actions.Request{Session: session, Payload: payload})
}

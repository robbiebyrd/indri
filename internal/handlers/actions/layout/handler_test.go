package layout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/ids"
	"github.com/robbiebyrd/indri/internal/services/mutation"
)

const (
	hostID    = "host-user"
	playerID  = "other-user"
	otherGame = "000000000000000000000000"
	gameCode  = "hello-world"
)

// fakeGames is the game lookup. It answers with one game, so every test can say
// exactly what the caller is up against.
type fakeGames struct {
	game *models.Game
	err  error
	code string
}

func (f *fakeGames) GetByCode(code string) (*models.Game, error) {
	f.code = code

	return f.game, f.err
}

// fakeMutator stands in for gameRepo.Store.Mutate and copies the one behaviour
// the handler depends on: apply runs against the loaded game, ErrAbort means
// "no change needed" and is swallowed without a write, and any other error
// aborts the whole mutation. It records whether a write would have committed so
// a test can assert "no write, no delta" rather than only "no error".
type fakeMutator struct {
	game      *models.Game
	ctx       context.Context
	id        string
	called    bool
	committed bool
	applyErr  error
}

func (f *fakeMutator) Mutate(ctx context.Context, id string, apply func(g *models.Game) error) error {
	f.ctx = ctx
	f.id = id
	f.called = true

	if err := apply(f.game); err != nil {
		f.applyErr = err

		if errors.Is(err, mutation.ErrAbort) {
			return nil
		}

		return err
	}

	f.committed = true

	return nil
}

// hostedGame is a game with a host, a non-host player and a valid layout, plus
// enough of everything else that a test can prove the handler did not touch it.
func hostedGame() *models.Game {
	g := gameWithLayout(validLayout())
	g.ID = ids.New()
	g.Code = gameCode
	g.Players = map[string]models.Player{
		hostID:   {Name: "Host", Host: true},
		playerID: {Name: "Player"},
	}
	g.Teams = map[string]models.Team{"red": {Name: "Red"}}
	g.Stage = models.Stage{CurrentScene: "board"}
	g.PrivateData = map[string]interface{}{"answer": "42"}

	return g
}

func sessionFor(userID *string, gameID *string) *models.Session {
	return &models.Session{ID: ids.New(), UserID: userID, GameID: gameID}
}

// addWidgetPayload is a valid op that does not collide with the fixture's
// widget, so a test that expects a rejection can only have been rejected by
// authorization.
func addWidgetPayload() map[string]interface{} {
	return map[string]interface{}{
		"code":     gameCode,
		"op":       OpAddWidget,
		"sceneId":  "board",
		"widgetId": "score",
		"widget": map[string]interface{}{
			fieldType:      "text",
			fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 8, "row": 0, "w": 2, "h": 1},
		},
	}
}

// run wires the handler's dependencies around one game and returns both the
// outcome and the mutator, so a test can assert on what was (not) written.
func run(t *testing.T, session *models.Session, payload map[string]interface{}) (*fakeMutator, error) {
	t.Helper()

	g := hostedGame()
	mutator := &fakeMutator{game: g}

	_, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	)

	return mutator, err
}

func TestEditLayout_RejectsACallerWithNoIdentity(t *testing.T) {
	cases := map[string]*models.Session{
		"no session":                  nil,
		"session with no user":        sessionFor(nil, ptr(otherGame)),
		"session with no game":        sessionFor(ptr(hostID), nil),
		"session in a different game": sessionFor(ptr(hostID), ptr(otherGame)),
	}

	for name, session := range cases {
		t.Run(name, func(t *testing.T) {
			mutator, err := run(t, session, addWidgetPayload())
			if err == nil {
				t.Fatalf("editLayout(%v) = nil, want the caller rejected", name)
			}

			if mutator.called {
				t.Errorf("editLayout(%v) reached Mutate; a rejected caller must not write", name)
			}
		})
	}
}

func TestEditLayout_RejectsANonHost(t *testing.T) {
	g := hostedGame()
	mutator := &fakeMutator{game: g}
	session := sessionFor(ptr(playerID), ptr(g.ID))

	_, err := editLayout(
		actions.Request{Session: session, Payload: addWidgetPayload()},
		&fakeGames{game: g},
		mutator,
	)
	if err == nil {
		t.Fatalf("editLayout(non-host) = nil, want the caller rejected")
	}

	if mutator.called {
		t.Errorf("editLayout(non-host) reached Mutate; only the host may edit a layout")
	}
}

// The caller is whoever the transport authenticated, and nothing else. A
// payload naming the host is data about the *subject* of an op at most; it can
// never become the author of one. There is no code path that reads it, and this
// test exists so that stays true.
func TestEditLayout_IgnoresAUserIdInThePayload(t *testing.T) {
	g := hostedGame()
	mutator := &fakeMutator{game: g}

	payload := addWidgetPayload()
	payload["userId"] = hostID

	session := sessionFor(ptr(playerID), ptr(g.ID))

	_, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	)
	if err == nil {
		t.Fatalf("editLayout(non-host claiming to be the host) = nil, want the caller rejected")
	}

	// Assert WHICH gate rejected, not merely that something did. decodeOp also
	// refuses an unknown "userId" key, so a bare err != nil here passes even
	// with the host check removed entirely — the test would then be pinned on
	// an unrelated gate rather than on authorization. Verified by mutation:
	// disabling the host check must fail this test.
	if !strings.Contains(err.Error(), "is not the host") {
		t.Fatalf("editLayout(non-host claiming to be the host) = %v, want rejection by the HOST check", err)
	}

	if mutator.called {
		t.Errorf("a payload userId got a non-host past the host check")
	}
}

func TestEditLayout_RejectsAMessageWithNoGameCode(t *testing.T) {
	g := hostedGame()
	session := sessionFor(ptr(hostID), ptr(g.ID))

	payload := addWidgetPayload()
	delete(payload, "code")

	mutator := &fakeMutator{game: g}

	if _, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	); err == nil {
		t.Fatalf("editLayout(no code) = nil, want the message rejected")
	}
}

func TestEditLayout_RejectsAnUndecodableOp(t *testing.T) {
	g := hostedGame()
	session := sessionFor(ptr(hostID), ptr(g.ID))

	payload := addWidgetPayload()
	payload["op"] = "dropTable"

	mutator := &fakeMutator{game: g}

	if _, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	); !errors.Is(err, errUnknownOp) {
		t.Fatalf("editLayout(unknown op) = %v, want %v", err, errUnknownOp)
	}

	if mutator.called {
		t.Errorf("an undecodable op reached Mutate")
	}
}

func TestEditLayout_HostEditWritesOnlyTheLayout(t *testing.T) {
	g := hostedGame()
	g.PublicData["scores"] = map[string]interface{}{"red": 3}

	before := struct{ players, teams, stage, private, scores string }{
		mustJSON(t, g.Players), mustJSON(t, g.Teams), mustJSON(t, g.Stage),
		mustJSON(t, g.PrivateData), mustJSON(t, g.PublicData["scores"]),
	}

	mutator := &fakeMutator{game: g}
	games := &fakeGames{game: g}
	session := sessionFor(ptr(hostID), ptr(g.ID))

	result, err := editLayout(
		actions.Request{Session: session, Payload: addWidgetPayload()},
		games,
		mutator,
	)
	if err != nil {
		t.Fatalf("editLayout(host) = %v, want the edit applied", err)
	}

	if !mutator.committed {
		t.Fatalf("the edit never committed")
	}

	// The game is addressed by the code on the wire and then written by its own
	// id, never by anything else the payload carries.
	if games.code != gameCode {
		t.Errorf("looked up game %q, want the code from the payload %q", games.code, gameCode)
	}

	if mutator.id != g.ID {
		t.Errorf("Mutate id = %q, want the resolved game %q", mutator.id, g.ID)
	}

	// The published delta is the response; a direct reply would double-report
	// the edit to the host and drift from what everyone else sees.
	if len(result.Responses) != 0 || result.Session != nil || len(result.DisconnectIDs) != 0 {
		t.Errorf("Result = %+v, want it empty so only the delta reports the edit", result)
	}

	after := struct{ players, teams, stage, private, scores string }{
		mustJSON(t, g.Players), mustJSON(t, g.Teams), mustJSON(t, g.Stage),
		mustJSON(t, g.PrivateData), mustJSON(t, g.PublicData["scores"]),
	}

	if after != before {
		t.Errorf("the edit reached outside data.layout:\n before %+v\n after  %+v", before, after)
	}

	if got := at(t, layoutOf(t, g), fieldScenes, "board", fieldWidgets, "score", fieldType); got != "text" {
		t.Errorf("added widget type = %v, want %q", got, "text")
	}
}

func TestEditLayout_ANoOpRemoveAbortsWithoutWriting(t *testing.T) {
	g := hostedGame()
	mutator := &fakeMutator{game: g}
	session := sessionFor(ptr(hostID), ptr(g.ID))

	payload := map[string]interface{}{
		"code": gameCode, "op": OpRemoveWidget, "sceneId": "board", "widgetId": "absent",
	}

	if _, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	); err != nil {
		t.Fatalf("editLayout(removeWidget, missing id) = %v, want it to succeed silently", err)
	}

	if !errors.Is(mutator.applyErr, mutation.ErrAbort) {
		t.Errorf("apply returned %v, want %v so no version bump and no delta occur", mutator.applyErr, mutation.ErrAbort)
	}

	if mutator.committed {
		t.Errorf("a no-op removeWidget committed a write")
	}
}

func TestEditLayout_RejectsAnOpThatWouldInvalidateTheLayout(t *testing.T) {
	g := hostedGame()
	mutator := &fakeMutator{game: g}
	session := sessionFor(ptr(hostID), ptr(g.ID))

	before := mustJSON(t, g.PublicData)

	// The fixture's "title" widget already occupies this area.
	payload := addWidgetPayload()
	payload["widget"] = map[string]interface{}{
		fieldType:      "text",
		fieldPlacement: map[string]interface{}{fieldKind: kindGrid, "col": 1, "row": 1, "w": 2, "h": 2},
	}

	if _, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	); !errors.Is(err, errOverlap) {
		t.Fatalf("editLayout(overlapping widget) = %v, want %v", err, errOverlap)
	}

	if mutator.committed {
		t.Errorf("an invalid layout committed")
	}

	if after := mustJSON(t, g.PublicData); after != before {
		t.Errorf("public data changed on a rejected op:\n before %v\n after  %v", before, after)
	}
}

func TestEditLayout_SetGridOnAGameWithNoLayoutProducesAValidDocument(t *testing.T) {
	g := hostedGame()
	g.PublicData = nil

	mutator := &fakeMutator{game: g}
	session := sessionFor(ptr(hostID), ptr(g.ID))

	payload := map[string]interface{}{
		"code": gameCode, "op": OpSetGrid,
		"grid": map[string]interface{}{fieldCols: float64(12), fieldRows: float64(8)},
	}

	if _, err := editLayout(
		actions.Request{Session: session, Payload: payload},
		&fakeGames{game: g},
		mutator,
	); err != nil {
		t.Fatalf("editLayout(setGrid, no layout) = %v, want a seeded layout", err)
	}

	if !mutator.committed {
		t.Fatalf("setGrid on a game with no layout never committed")
	}

	if err := validateLayout(layoutOf(t, g)); err != nil {
		t.Errorf("validateLayout(seeded layout) = %v, want the stored layout to be valid", err)
	}
}

func TestEditLayout_SurfacesALookupFailure(t *testing.T) {
	wanted := errors.New("no such game")
	mutator := &fakeMutator{game: hostedGame()}

	_, err := editLayout(
		actions.Request{
			Session: sessionFor(ptr(hostID), ptr(otherGame)),
			Payload: addWidgetPayload(),
		},
		&fakeGames{err: wanted},
		mutator,
	)
	if !errors.Is(err, wanted) {
		t.Fatalf("editLayout(unknown game) = %v, want %v", err, wanted)
	}

	if mutator.called {
		t.Errorf("a failed lookup still reached Mutate")
	}
}

// TestEditLayout_GivesMutateTheRequestContext matters because Mutate waits on
// the game lock and then talks to the database. Handed the store's boot-time
// context instead of the caller's, an edit from a client that has already gone
// away would sit on that lock with nobody left to care.
func TestEditLayout_GivesMutateTheRequestContext(t *testing.T) {
	type callerKey struct{}

	g := hostedGame()
	mutator := &fakeMutator{game: g}
	ctx := context.WithValue(context.Background(), callerKey{}, "the caller")

	if _, err := editLayout(
		actions.Request{
			Context: ctx,
			Session: sessionFor(ptr(hostID), ptr(g.ID)),
			Payload: addWidgetPayload(),
		},
		&fakeGames{game: g},
		mutator,
	); err != nil {
		t.Fatalf("editLayout(host adding a widget) = %v, want no error", err)
	}

	if mutator.ctx != ctx {
		t.Errorf("Mutate got context %v, want the request's own context %v", mutator.ctx, ctx)
	}
}

// TestEditLayout_SurvivesARequestWithNoContext covers the handler being called
// by something other than the router (a test, a future transport): a nil
// Context must degrade to Background, not panic inside the lock.
func TestEditLayout_SurvivesARequestWithNoContext(t *testing.T) {
	g := hostedGame()
	mutator := &fakeMutator{game: g}

	if _, err := editLayout(
		actions.Request{
			Session: sessionFor(ptr(hostID), ptr(g.ID)),
			Payload: addWidgetPayload(),
		},
		&fakeGames{game: g},
		mutator,
	); err != nil {
		t.Fatalf("editLayout(request with no context) = %v, want no error", err)
	}

	if mutator.ctx == nil {
		t.Errorf("Mutate got a nil context, want context.Background()")
	}
}

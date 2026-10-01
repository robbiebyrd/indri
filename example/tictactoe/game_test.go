// Package tictactoe holds the worked example game. It has no Go code of its
// own: the game is config.json plus game.lua, and this file is the proof that
// the pair behaves the way the Go move handler it replaced did.
//
// Every test here has the same shape — a fixture game, one dispatched "move",
// and an assertion about the delta the store published — because the delta is
// the whole of what a player observes. A write that never publishes is
// invisible, so asserting the stored board alone would pass on a change nobody
// in the game could see.
package tictactoe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	"github.com/robbiebyrd/indri/internal/services/events"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
)

// configPath is the game's own config, read the way the server reads it. The
// engine under test is built from the scripts *it* declares rather than from a
// path spelled out here, so a config that stops listing game.lua fails this
// suite instead of quietly shipping a server that answers no game action.
const configPath = "config.json"

// The two teams every fixture plays with, named as config.json names them. The
// space in the id is deliberate: it travels into a delta path, and a test using
// tidier ids would not notice that.
const (
	teamX = "Player 1"
	teamO = "Player 2"

	markerX = "X"
	markerO = "O"
)

// The scene the board lives on, and the delta paths reached through it.
const (
	sceneID    = "board"
	boardPath  = "stage.scenes.board.data.board"
	winnerPath = "stage.scenes.board.data.winningTeam"
)

// testTimeout bounds an invocation so a script that hangs fails rather than
// stalling the suite.
const testTimeout = 5 * time.Second

// TestMain silences the host's own logging for the run.
//
// Most of this suite asserts that a move was refused, and the host logs every
// script failure with a correlation id and a Lua traceback before packing it
// into the frame the caller is answered with. Those lines are the expected
// output of a passing test, and the failures themselves are asserted on through
// the returned frame — so letting them reach stderr would bury a real one in
// several hundred lines of noise.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)

	os.Exit(m.Run())
}

// turnPath is where a team's turn flag lands in a delta.
func turnPath(team string) string {
	return "teams." + team + ".data.turn"
}

// cells is a fixture board, outermost index first: cells[row][column], both
// counted from zero exactly as a player's "r,c" is.
type cells [][]string

// empty3x3 is the starting board config.json ships.
func empty3x3() cells {
	return cells{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
}

// document renders the board the way events.ToMap does, which is the shape the
// published delta carries it in: JSON arrays all the way down.
func (c cells) document() []interface{} {
	rows := make([]interface{}, len(c))

	for i, row := range c {
		columns := make([]interface{}, len(row))

		for j, cell := range row {
			columns[j] = cell
		}

		rows[i] = columns
	}

	return rows
}

// capturingPublisher records every delta the store fans out.
type capturingPublisher struct {
	mu     sync.Mutex
	events []events.ChangeEvent
}

func (p *capturingPublisher) Publish(_ context.Context, event events.ChangeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, event)

	return nil
}

func (p *capturingPublisher) Subscribe(context.Context) (<-chan events.ChangeEvent, error) {
	return nil, nil
}

func (p *capturingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.events)
}

// only returns the single delta the move published, failing when the count is
// anything else. One move is one write, so two deltas would mean a player saw
// the board move in two steps.
func (p *capturingPublisher) only(t *testing.T) events.ChangeEvent {
	t.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.events) != 1 {
		t.Fatalf("the move published %d deltas, want exactly 1", len(p.events))
	}

	return p.events[0]
}

// fixture is one starting game: the board and the teams, with the turn already
// placed on one of them.
type fixture struct {
	board cells
	teams map[string]models.Team

	// omitBoard leaves the scene without a board key at all, which is not the
	// same as an empty one: an empty board is a zero-by-zero game whose every
	// move is out of bounds, while a missing one is a config mistake.
	omitBoard bool
}

// newFixture builds the standard two-team game around board, with the turn held
// by the named team.
func newFixture(board cells, turn string) fixture {
	f := fixture{board: board, teams: map[string]models.Team{}}

	for id, marker := range map[string]string{teamX: markerX, teamO: markerO} {
		f.teams[id] = models.Team{
			Name:       id,
			PlayerIDs:  []string{},
			PublicData: map[string]interface{}{"marker": marker, "turn": id == turn},
		}
	}

	return f
}

// script renders the fixture as the models.Script a game is stamped from, which
// is how config.json reaches a game in production too.
func (f fixture) script() *models.Script {
	sceneData := map[string]interface{}{}
	if !f.omitBoard {
		sceneData["board"] = f.board.document()
	}

	return &models.Script{
		Teams: maps.Clone(f.teams),
		Stage: models.Stage{
			CurrentScene: sceneID,
			SceneOrder:   []string{sceneID},
			Scenes:       map[string]models.Scene{sceneID: {PublicData: &sceneData}},
		},
	}
}

// newEngine builds the engine over the scripts config.json declares, which is
// the same call internal/injector makes at boot.
func newEngine(t *testing.T, games luaService.GameMutator) *luaService.Engine {
	t.Helper()

	store, err := scriptRepo.NewStore(configPath)
	if err != nil {
		t.Fatalf("reading %s: %v", configPath, err)
	}

	engine, err := luaService.NewEngineWithGrants(store.Get().Scripts, games)
	if err != nil {
		t.Fatalf("building the engine from %s: %v", configPath, err)
	}

	t.Cleanup(engine.Close)

	return engine
}

// run is everything one dispatched move leaves behind.
type run struct {
	publisher *capturingPublisher
	store     *gameRepo.MemoryStore
	id        string

	// version is the game's version before the move, so a refusal can be held to
	// having changed nothing rather than to a hard-coded number.
	version int64

	// err is the script's own refusal, or nil when the move was accepted.
	err error
}

// moveAs stamps the fixture into a real in-memory game store and dispatches one
// "move" against it as session.
//
// A MemoryStore rather than a stub: it runs the same core as the MongoDB store,
// so the lock, the version fence and the delta computation exercised here are
// the production ones.
func moveAs(t *testing.T, f fixture, session *models.Session, payload map[string]interface{}) run {
	t.Helper()

	publisher := &capturingPublisher{}
	store := gameRepo.NewMemoryStore(context.Background(), nil, publisher)

	g, err := store.New("ABCD", f.script(), false)
	if err != nil {
		t.Fatalf("creating the fixture game: %v", err)
	}

	id := g.ID

	if session != nil {
		session.GameID = &id
	}

	return run{
		publisher: publisher,
		store:     store,
		id:        id,
		version:   g.Version,
		err:       invoke(t, store, session, payload),
	}
}

// move is moveAs for the ordinary case: an authenticated player of team. An
// empty team stands for a session that has joined no team.
func move(t *testing.T, f fixture, team string, payload map[string]interface{}) run {
	t.Helper()

	session := &models.Session{UserID: stringPtr("player-1")}
	if team != "" {
		session.TeamID = &team
	}

	return moveAs(t, f, session, payload)
}

// invoke runs the handler and folds a script's own failure back into an error.
//
// Invoke does not report a script's failure through its error return — the
// transports do not agree about a Go error — so it packs it into Responses as a
// models.WSError. A test asserting "this move was refused" has to unpack that,
// and this is the one place that does.
func invoke(
	t *testing.T,
	games luaService.GameMutator,
	session *models.Session,
	payload map[string]interface{},
) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	result, err := newEngine(t, games).Invoke(ctx, "move", actions.Request{
		Context: ctx,
		Action:  "move",
		Session: session,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("the host failed to run the handler: %v", err)
	}

	for _, response := range result.Responses {
		var frame models.WSError

		// The code is what tells a script's failure from a reply: every JSON
		// object unmarshals into a WSError with a zero code.
		if json.Unmarshal(response, &frame) == nil && frame.ErrorCode == models.ErrScriptFailed.ErrorCode {
			return errors.New(frame.Message)
		}
	}

	return nil
}

// moveAt is the payload a client sends, as the wire carries it: one string.
func moveAt(move string) map[string]interface{} {
	return map[string]interface{}{"move": move}
}

func stringPtr(s string) *string {
	return &s
}

// played is the delta a successful move publishes: the whole board (an array is
// replaced whole, never patched per cell), the turn passing from the mover to
// the other team, and the winner when there is one.
func played(board cells, mover string, winner string) map[string]interface{} {
	want := map[string]interface{}{
		boardPath:       board.document(),
		turnPath(teamX): teamX != mover,
		turnPath(teamO): teamO != mover,
	}

	if winner != "" {
		want[winnerPath] = winner
	}

	return want
}

// assertPlayed checks the move was accepted and published exactly want, plus the
// updatedAt every committed save stamps, and removed nothing.
//
// updatedAt is asserted as present rather than compared, because it is a clock
// reading no fixture can predict — and its presence is worth asserting, since a
// delta without it did not come from a save.
func assertPlayed(t *testing.T, r run, want map[string]interface{}) {
	t.Helper()

	if r.err != nil {
		t.Fatalf("the move was refused: %v", r.err)
	}

	event := r.publisher.only(t)

	if len(event.RemovedFields) != 0 {
		t.Errorf("the move removed %v; a move only ever adds to the board", event.RemovedFields)
	}

	got := maps.Clone(event.UpdatedFields)

	if _, ok := got["updatedAt"]; !ok {
		t.Errorf("the delta carries no updatedAt, so no save produced it")
	}

	delete(got, "updatedAt")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("published %v,\n\twant  %v", sorted(got), sorted(want))
	}
}

// assertRefused checks the move was rejected with a message containing want,
// wrote nothing and published nothing.
//
// All three halves matter. A script that silently did nothing would publish
// nothing either, so the message is what separates "refused this move" from
// "never ran"; and the version proves the refusal unwound the store rather than
// committing and then complaining.
func assertRefused(t *testing.T, r run, want string) {
	t.Helper()

	if r.err == nil {
		t.Fatalf("the move was accepted, want it refused with a message containing %q", want)
	}

	if !strings.Contains(r.err.Error(), want) {
		t.Errorf("the move was refused with %q, want a message containing %q", r.err, want)
	}

	if got := r.publisher.count(); got != 0 {
		t.Errorf("a refused move published %d deltas, want none", got)
	}

	g, err := r.store.Get(r.id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	if g.Version != r.version {
		t.Errorf("a refused move moved the game from version %d to %d", r.version, g.Version)
	}
}

// sorted renders a delta as sorted path=value lines, so a mismatch reads as a
// diff rather than as two randomly ordered maps.
func sorted(delta map[string]interface{}) []string {
	lines := make([]string, 0, len(delta))

	for _, path := range slices.Sorted(maps.Keys(delta)) {
		lines = append(lines, path+"="+render(delta[path]))
	}

	return lines
}

func render(value interface{}) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "?"
	}

	return string(raw)
}

// --- wiring ------------------------------------------------------------------

// The router is given exactly the actions the engine declares, so what this game
// answers is what game.lua registered. One action, named "move": the client's
// layout script sends that name, and a rename on either side is a game that
// stops responding to taps.
func TestScript_DeclaresOnlyTheMoveAction(t *testing.T) {
	got := newEngine(t, nil).Actions()

	if want := []string{"move"}; !slices.Equal(got, want) {
		t.Fatalf("config.json's scripts declare %v, want %v", got, want)
	}
}

// The game needs no host capability. Granting one it does not use would widen
// what a bug in it could reach, and grants are per script precisely so that this
// is a glance at the config rather than an audit of the script.
func TestConfig_GrantsTheScriptNothing(t *testing.T) {
	store, err := scriptRepo.NewStore(configPath)
	if err != nil {
		t.Fatalf("reading %s: %v", configPath, err)
	}

	scripts := store.Get().Scripts

	if len(scripts) != 1 {
		t.Fatalf("%s declares %d scripts, want exactly 1", configPath, len(scripts))
	}

	if len(scripts[0].Grants) != 0 {
		t.Errorf("%s grants %v to %q, want nothing", configPath, scripts[0].Grants, scripts[0].Path)
	}
}

// --- an ordinary move --------------------------------------------------------

// The first move of a game, and the one case that pins the coordinate order. A
// move is "row,column", both counted from zero, so "1,2" is the last cell of the
// middle row — and a board that transposed would place it at row 2, column 1 and
// still look like a working game.
func TestMove_PlacesTheMarkerAtRowThenColumn(t *testing.T) {
	r := move(t, newFixture(empty3x3(), teamX), teamX, moveAt("1,2"))

	after := cells{
		{"", "", ""},
		{"", "", markerX},
		{"", "", ""},
	}

	assertPlayed(t, r, played(after, teamX, ""))

	// The delta is what a player sees, but it is only honest if it describes
	// what was stored. Checked once, here, rather than in every case.
	stored, err := r.store.Get(r.id)
	if err != nil {
		t.Fatalf("reloading the game: %v", err)
	}

	// Compared as JSON rather than with reflect.DeepEqual: the stored document
	// comes back through the BSON round trip as bson.A, and the assertion here
	// is about the cells, not about which list type the driver decodes into.
	if got := (*stored.Stage.Scenes[sceneID].PublicData)["board"]; render(got) != render(after.document()) {
		t.Errorf("stored board %v, want %v", render(got), render(after.document()))
	}

	if got := stored.Teams[teamX].PublicData["turn"]; got != false {
		t.Errorf("the mover still holds the turn (%v)", got)
	}

	if got := stored.Teams[teamO].PublicData["turn"]; got != true {
		t.Errorf("the turn did not pass to the other team (%v)", got)
	}
}

// --- winning -----------------------------------------------------------------

// A full row and a full column both win, for either team. Each fixture is the
// winning board the deleted Go test asserted on, minus the move that completes
// it.
func TestMove_CompletingARowOrColumnWins(t *testing.T) {
	tests := map[string]struct {
		before cells
		mover  string
		at     string
		after  cells
	}{
		"a full row": {
			before: cells{
				{markerX, markerX, ""},
				{markerO, "", markerO},
				{"", "", ""},
			},
			mover: teamX,
			at:    "0,2",
			after: cells{
				{markerX, markerX, markerX},
				{markerO, "", markerO},
				{"", "", ""},
			},
		},
		"a full column": {
			before: cells{
				{markerO, markerX, ""},
				{markerO, markerX, ""},
				{"", "", ""},
			},
			mover: teamO,
			at:    "2,0",
			after: cells{
				{markerO, markerX, ""},
				{markerO, markerX, ""},
				{markerO, "", ""},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := move(t, newFixture(test.before, test.mover), test.mover, moveAt(test.at))

			assertPlayed(t, r, played(test.after, test.mover, test.mover))
		})
	}
}

// Both diagonals win. They are the two lines a row-and-column check misses
// entirely, and a board that wins on one but not the other is the classic
// off-by-one in the anti-diagonal.
func TestMove_CompletingADiagonalWins(t *testing.T) {
	tests := map[string]struct {
		before cells
		at     string
		after  cells
	}{
		"the main diagonal": {
			before: cells{
				{markerX, markerO, ""},
				{markerO, markerX, ""},
				{"", "", ""},
			},
			at: "2,2",
			after: cells{
				{markerX, markerO, ""},
				{markerO, markerX, ""},
				{"", "", markerX},
			},
		},
		"the anti-diagonal": {
			before: cells{
				{"", markerO, markerX},
				{markerO, markerX, ""},
				{"", "", ""},
			},
			at: "2,0",
			after: cells{
				{"", markerO, markerX},
				{markerO, markerX, ""},
				{markerX, "", ""},
			},
		},
		// A board wider than it is tall has several diagonals of each kind, and
		// both cases below win on one that starts away from the corner. Neither
		// property is optional here. A square fixture cannot tell a row index
		// from a column index, because transposing a 3x3 board leaves both of its
		// diagonals where they were; and a diagonal starting at column zero
		// cannot either, because row and column are equal all the way along it.
		"a diagonal down and right on a wider board": {
			before: cells{
				{markerO, markerX, ""},
				{"", "", ""},
			},
			at: "1,2",
			after: cells{
				{markerO, markerX, ""},
				{"", "", markerX},
			},
		},
		"a diagonal down and left on a wider board": {
			before: cells{
				{markerO, markerX, ""},
				{"", "", ""},
			},
			at: "1,0",
			after: cells{
				{markerO, markerX, ""},
				{markerX, "", ""},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := move(t, newFixture(test.before, teamX), teamX, moveAt(test.at))

			assertPlayed(t, r, played(test.after, teamX, teamX))
		})
	}
}

// --- drawing -----------------------------------------------------------------

// Filling the last cell with no line anywhere is a draw. Both fixtures are full
// boards the deleted Go tests asserted "no win" on: the first has no row and no
// column, the second additionally has no diagonal either way.
func TestMove_FillingTheBoardWithNoLineDraws(t *testing.T) {
	tests := map[string]struct {
		before cells
		mover  string
		at     string
		after  cells
	}{
		"no row and no column": {
			before: cells{
				{markerX, markerO, markerX},
				{markerO, markerX, markerO},
				{markerO, markerX, ""},
			},
			mover: teamO,
			at:    "2,2",
			after: cells{
				{markerX, markerO, markerX},
				{markerO, markerX, markerO},
				{markerO, markerX, markerO},
			},
		},
		"no diagonal either way": {
			before: cells{
				{markerX, markerO, markerX},
				{markerO, markerO, markerX},
				{markerX, "", markerO},
			},
			mover: teamX,
			at:    "2,1",
			after: cells{
				{markerX, markerO, markerX},
				{markerO, markerO, markerX},
				{markerX, markerX, markerO},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := move(t, newFixture(test.before, test.mover), test.mover, moveAt(test.at))

			assertPlayed(t, r, played(test.after, test.mover, "draw"))
		})
	}
}

// One empty cell left and no line is not a draw yet. This is the case that
// separates "the board is full" from "nobody has won": drop the empty-cell test
// and this board is declared drawn with a move still to play.
func TestMove_ANearlyFullBoardIsNotADraw(t *testing.T) {
	before := cells{
		{markerX, markerO, markerX},
		{markerO, markerX, markerO},
		{"", markerX, ""},
	}

	after := cells{
		{markerX, markerO, markerX},
		{markerO, markerX, markerO},
		{markerO, markerX, ""},
	}

	r := move(t, newFixture(before, teamO), teamO, moveAt("2,0"))

	assertPlayed(t, r, played(after, teamO, ""))
}

// Two in a line is not three. A win check that counted "some" rather than "all"
// would end the game here, on boards both of the deleted Go tests asserted were
// undecided.
func TestMove_APartialLineDoesNotWin(t *testing.T) {
	tests := map[string]struct {
		before cells
		at     string
		after  cells
	}{
		"two along a row": {
			before: cells{
				{markerX, "", ""},
				{markerO, "", markerO},
				{"", "", ""},
			},
			at: "0,1",
			after: cells{
				{markerX, markerX, ""},
				{markerO, "", markerO},
				{"", "", ""},
			},
		},
		"two along the diagonal": {
			before: cells{
				{markerX, "", ""},
				{"", "", ""},
				{"", markerO, ""},
			},
			at: "1,1",
			after: cells{
				{markerX, "", ""},
				{"", markerX, ""},
				{"", markerO, ""},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := move(t, newFixture(test.before, teamX), teamX, moveAt(test.at))

			assertPlayed(t, r, played(test.after, teamX, ""))
		})
	}
}

// A board holding only one player's marks cannot have been played by both, so it
// is never won — however many of them line up. A config seeding the board
// part-way through would otherwise start a game that was already over.
func TestMove_OneMarkerAloneNeverWins(t *testing.T) {
	before := cells{
		{markerX, markerX, ""},
		{"", "", ""},
		{"", "", ""},
	}

	after := cells{
		{markerX, markerX, markerX},
		{"", "", ""},
		{"", "", ""},
	}

	r := move(t, newFixture(before, teamX), teamX, moveAt("0,2"))

	assertPlayed(t, r, played(after, teamX, ""))
}

// --- refusals ----------------------------------------------------------------

// Turn enforcement. Without it tic-tac-toe is a race: whoever taps first plays,
// twice if they are quick.
func TestMove_RefusesAPlayerOutOfTurn(t *testing.T) {
	assertRefused(t, move(t, newFixture(empty3x3(), teamX), teamO, moveAt("1,1")), "it is not your turn")
}

// An occupied cell is refused rather than overwritten, which would let a player
// erase their opponent's mark.
func TestMove_RefusesAnOccupiedCell(t *testing.T) {
	before := cells{
		{"", "", ""},
		{"", markerX, ""},
		{"", "", ""},
	}

	assertRefused(t, move(t, newFixture(before, teamO), teamO, moveAt("1,1")), "spot is taken")
}

// Everything a client can put in the "move" field that is not a cell on the
// board. Each was a case of the deleted handler's decodeMove tests, plus the
// three literals Lua's tonumber accepts and Go's strconv.Atoi does not.
func TestMove_RefusesAMalformedMove(t *testing.T) {
	tests := map[string]struct {
		payload map[string]interface{}
		want    string
	}{
		"no move at all":        {payload: map[string]interface{}{}, want: "move is nil"},
		"one coordinate":        {payload: moveAt("1"), want: "invalid move"},
		"three coordinates":     {payload: moveAt("1,2,0"), want: "invalid move"},
		"not a number":          {payload: moveAt("a,2"), want: "error converting"},
		"an empty string":       {payload: moveAt(""), want: "error converting"},
		"a negative row":        {payload: moveAt("-1,2"), want: "move out of bounds"},
		"a column off the end":  {payload: moveAt("1,3"), want: "move out of bounds"},
		"a row off the end":     {payload: moveAt("3,1"), want: "move out of bounds"},
		"not a string":          {payload: map[string]interface{}{"move": 12}, want: "invalid move"},
		"a decimal coordinate":  {payload: moveAt("1.0,2"), want: "error converting"},
		"a hexadecimal literal": {payload: moveAt("0x2,2"), want: "error converting"},
		"an exponent":           {payload: moveAt("1e1,2"), want: "error converting"},
		"padded with spaces":    {payload: moveAt(" 1,2"), want: "error converting"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertRefused(t, move(t, newFixture(empty3x3(), teamX), teamX, test.payload), test.want)
		})
	}
}

// The board's shape comes from the config, not from a constant, so the bounds
// move with it. Each case plays the last legal cell of its board and then tries
// the cell just past each edge.
func TestMove_BoundsFollowTheConfiguredBoard(t *testing.T) {
	tests := map[string]struct {
		before cells
		at     string
		after  cells
		past   []string
	}{
		"three by three": {
			before: empty3x3(),
			at:     "2,2",
			after: cells{
				{"", "", ""},
				{"", "", ""},
				{"", "", markerX},
			},
			past: []string{"3,2", "2,3"},
		},
		"three rows and two columns": {
			before: cells{{"", ""}, {"", ""}, {"", ""}},
			at:     "2,1",
			after:  cells{{"", ""}, {"", ""}, {"", markerX}},
			past:   []string{"3,1", "1,2"},
		},
		"one row": {
			before: cells{{"", "", ""}},
			at:     "0,2",
			after:  cells{{"", "", markerX}},
			past:   []string{"1,0", "0,3"},
		},
		"one column": {
			before: cells{{""}, {""}, {""}},
			at:     "2,0",
			after:  cells{{""}, {""}, {markerX}},
			past:   []string{"3,0", "2,1"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Run("the last cell is playable", func(t *testing.T) {
				r := move(t, newFixture(test.before, teamX), teamX, moveAt(test.at))

				assertPlayed(t, r, played(test.after, teamX, ""))
			})

			for _, at := range test.past {
				t.Run("past the edge at "+at, func(t *testing.T) {
					r := move(t, newFixture(test.before, teamX), teamX, moveAt(at))

					assertRefused(t, r, "move out of bounds")
				})
			}
		})
	}
}

// Who is calling comes from the session the transport authenticated, so a caller
// with no team of their own has no marker and cannot move. The move itself is
// legal in both cases: what is being refused is the caller.
func TestMove_RefusesACallerWithNoTeam(t *testing.T) {
	tests := map[string]struct {
		team string
		want string
	}{
		"a session in no team": {
			team: "",
			want: "session is not in a game/team",
		},
		"a session naming a team this game does not have": {
			team: "Player 3",
			want: "your team is not in this game",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertRefused(t, move(t, newFixture(empty3x3(), teamX), test.team, moveAt("1,1")), test.want)
		})
	}
}

// An unauthenticated caller reaches the handler as a request with no session at
// all, which is not the same as a session that has joined no team.
func TestMove_RefusesAnUnauthenticatedCaller(t *testing.T) {
	assertRefused(t, moveAs(t, newFixture(empty3x3(), teamX), nil, moveAt("1,1")), "not authenticated")
}

// A team the config never gave a marker or a turn cannot play, and says which of
// the two is missing rather than placing an empty mark on the board.
func TestMove_RefusesAMisconfiguredTeam(t *testing.T) {
	tests := map[string]struct {
		data map[string]interface{}
		want string
	}{
		"no marker": {data: map[string]interface{}{}, want: "marker is nil"},
		"no turn":   {data: map[string]interface{}{"marker": markerX}, want: "turn is nil"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := newFixture(empty3x3(), teamX)
			f.teams[teamX] = models.Team{Name: teamX, PlayerIDs: []string{}, PublicData: test.data}

			assertRefused(t, move(t, f, teamX, moveAt("1,1")), test.want)
		})
	}
}

// A scene with no board at all is a config error, and says so rather than
// faulting somewhere inside the win check.
func TestMove_RefusesASceneWithNoBoard(t *testing.T) {
	f := newFixture(empty3x3(), teamX)
	f.omitBoard = true

	assertRefused(t, move(t, f, teamX, moveAt("1,1")), "no board")
}

// Package scripttest runs a game script's own fixtures against the real engine,
// with no server, no transport and no database.
//
// It exists because the whole premise of scripting a game in Lua is that its
// author writes no Go, and every other check in this repository is a `go test`.
// A fixture here is JSON: the state a game starts from, the action a player
// sends, and what that action must publish.
//
// # Why a fixture asserts on the delta
//
// The delta is the whole of what a player observes. A write that never
// publishes is invisible, so a harness that asserted on the stored game would
// pass on a change nobody in the game could see — which is the single failure
// this repository cares most about. That is also why the assertions cannot be
// written in Lua: the delta is computed in Go by events.Diff, after the
// indri.mutate callback has returned and the save has committed, and a script
// running inside the sandbox cannot see it at all. A Lua-side assertion could
// only read state, which is strictly the weaker claim.
//
// # What is real here
//
// Everything below the fixture. The scripts are compiled and loaded by the
// production engine, from the config the server reads; each case stamps its own
// game into a game.MemoryStore, which is the same core the MongoDB store runs —
// the same lock, the same version fence, the same retry budget, the same
// change deltas. Only where the documents live differs.
package scripttest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robbiebyrd/indri/internal/models"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
)

// FixtureSuffix is what marks a file as a fixture.
//
// A suffix rather than a plain ".json" so that scanning a game's own directory
// cannot mistake its config for a test, and so that no rule anywhere has to
// name and skip config.json — a rule that would silently stop working the day a
// game keeps a second JSON file beside it.
const FixtureSuffix = ".test.json"

// Suite is a config and the fixture files that exercise the scripts it
// declares.
type Suite struct {
	// ConfigPath is the config the scripts were read from, kept for the message
	// a load failure is reported with.
	ConfigPath string

	// Script is the game's own starting state, and the state a case that
	// declares none is played from. Defaulting to it rather than to an empty
	// game is what lets a fixture say only what it changes.
	Script *models.Script

	Fixtures []*Fixture
}

// Cases counts every case the suite will run. A suite that would run none is
// refused at load: a harness reporting success without having run anything is
// the one result it must never produce.
func (s *Suite) Cases() int {
	total := 0

	for _, f := range s.Fixtures {
		total += len(f.Cases)
	}

	return total
}

// Fixture is one fixture file.
//
// Every field a file may carry is here, and decoding is strict, so a misspelled
// key is a load failure rather than a silently ignored one. "cases" written as
// "case" would otherwise be a file that tests nothing and says so nowhere.
type Fixture struct {
	// Name is the file's base name without FixtureSuffix. It is the first half
	// of every case's reported name: move.test.json's "rejects a player out of
	// turn" is reported as "move/rejects a player out of turn".
	Name string `json:"-"`

	// Path is the file it was read from, named in a load failure.
	Path string `json:"-"`

	// State is the starting state for every case in the file that declares none
	// of its own. Most of a game's fixtures differ in one table and agree about
	// the rest; this is where the rest goes.
	State *models.Script `json:"state,omitempty"`

	Cases []Case `json:"cases"`
}

// Case is one dispatched action and what it must do.
type Case struct {
	Name string `json:"name"`

	// State overrides the file's, and the config's, for this case alone.
	State *models.Script `json:"state,omitempty"`

	Event  Event       `json:"event"`
	Expect Expectation `json:"expected"`
}

// Event is the action a case dispatches, as a transport would deliver it.
type Event struct {
	Action string `json:"action"`

	// Session is who is calling, already authenticated. Absent means an
	// unauthenticated caller, which is not the same as a session that has joined
	// no team — a script has to handle both and a fixture has to be able to
	// spell both.
	Session *Session `json:"session,omitempty"`

	Payload map[string]interface{} `json:"payload,omitempty"`
}

// Session is the caller's identity.
//
// It carries no game id on purpose. The game a case plays is the one the
// harness has just stamped its fixture into, and its id is a fresh ObjectID no
// fixture could name; letting a file supply one would only ever be a way to
// write a case that cannot run.
type Session struct {
	UserID string `json:"userId"`

	// TeamID is empty for a caller who has joined no team.
	TeamID string `json:"teamId,omitempty"`
}

// model renders the fixture's caller as the session a transport would have
// resolved, bound to the game this case is playing.
func (s *Session) model(gameID string) *models.Session {
	if s == nil {
		return nil
	}

	userID := s.UserID
	session := &models.Session{UserID: &userID, GameID: &gameID}

	if s.TeamID != "" {
		teamID := s.TeamID
		session.TeamID = &teamID
	}

	return session
}

// Expectation is what the action must have done. Exactly one of Error and
// Published says which kind of outcome is expected, and the rest is read in
// that light.
type Expectation struct {
	// Published is the delta the action must have published, as a map of dotted
	// path to value. An empty object is the assertion that it published nothing
	// at all, which is how a case says "this action correctly decided to do
	// nothing" — a pointer rather than a plain map so that "published": {} is
	// distinguishable from a fixture that forgot to say.
	Published *map[string]interface{} `json:"published,omitempty"`

	// Removed is the delta's removed paths. Absent means none were removed, and
	// that is asserted rather than ignored.
	Removed []string `json:"removed,omitempty"`

	// Replies are the documents indri.reply wrote back to the caller, in order.
	// Absent means the action replied nothing, and that too is asserted.
	Replies []map[string]interface{} `json:"replies,omitempty"`

	// Error is a substring of the message the script must have refused with.
	// A substring rather than the whole of it because the frame a caller is
	// answered with carries a correlation id, and a file and line when the
	// script faulted rather than refused — neither of which a fixture can
	// predict.
	Error string `json:"error,omitempty"`
}

// refusal reports whether this case expects the script to refuse.
func (e Expectation) refusal() bool {
	return e.Error != ""
}

// Load reads the config and every fixture the paths name.
//
// Each path is either a fixture file or a directory, which is walked for files
// ending in FixtureSuffix. Nothing found is an error: a CI job whose fixture
// directory was renamed must fail rather than report that all zero of its cases
// passed.
func Load(configPath string, paths []string) (*Suite, error) {
	store, err := scriptRepo.NewStore(configPath)
	if err != nil {
		return nil, err
	}

	files, err := collect(paths)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no %s fixture files found under %s", FixtureSuffix, strings.Join(paths, ", "))
	}

	suite := &Suite{ConfigPath: configPath, Script: store.Get()}

	for _, path := range files {
		fixture, err := loadFixture(path)
		if err != nil {
			return nil, err
		}

		suite.Fixtures = append(suite.Fixtures, fixture)
	}

	if suite.Cases() == 0 {
		return nil, fmt.Errorf("the %d fixture files found under %s declare no cases at all",
			len(files), strings.Join(paths, ", "))
	}

	return suite, nil
}

// collect expands the paths into the fixture files to read, sorted, with
// duplicates removed so that naming both a directory and a file inside it does
// not run the same case twice.
func collect(paths []string) ([]string, error) {
	seen := map[string]struct{}{}
	files := []string{}

	add := func(path string) {
		clean := filepath.Clean(path)

		if _, dup := seen[clean]; dup {
			return
		}

		seen[clean] = struct{}{}
		files = append(files, clean)
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("reading fixture path %q: %w", path, err)
		}

		if !info.IsDir() {
			// A file named outright is run whatever it is called: the suffix is
			// how a directory scan decides, not a rule about what may be a
			// fixture.
			add(path)

			continue
		}

		if err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !d.IsDir() && strings.HasSuffix(d.Name(), FixtureSuffix) {
				add(p)
			}

			return nil
		}); err != nil {
			return nil, fmt.Errorf("scanning fixture directory %q: %w", path, err)
		}
	}

	sort.Strings(files)

	return files, nil
}

// loadFixture reads and validates one fixture file.
func loadFixture(path string) (*Fixture, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading fixture %q: %w", path, err)
	}

	fixture := &Fixture{
		Name: strings.TrimSuffix(filepath.Base(path), FixtureSuffix),
		Path: path,
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))

	// Strict, because every key a fixture may carry is declared above. A typo in
	// "expected" would otherwise leave a case asserting nothing and passing.
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(fixture); err != nil {
		return nil, fmt.Errorf("parsing fixture %q: %w", path, err)
	}

	if err := fixture.validate(); err != nil {
		return nil, fmt.Errorf("fixture %q: %w", path, err)
	}

	return fixture, nil
}

// validate refuses a fixture that cannot mean what it says.
func (f *Fixture) validate() error {
	if len(f.Cases) == 0 {
		return errors.New("declares no cases")
	}

	seen := make(map[string]struct{}, len(f.Cases))

	for i, c := range f.Cases {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("case %d has no name", i+1)
		}

		if _, dup := seen[c.Name]; dup {
			return fmt.Errorf("two cases are both named %q", c.Name)
		}

		seen[c.Name] = struct{}{}

		if err := c.validate(); err != nil {
			return fmt.Errorf("case %q %w", c.Name, err)
		}
	}

	return nil
}

// validate refuses a case whose expectation is not an assertion about anything.
func (c Case) validate() error {
	if strings.TrimSpace(c.Event.Action) == "" {
		return errors.New("dispatches no action")
	}

	if c.Event.Session != nil && strings.TrimSpace(c.Event.Session.UserID) == "" {
		return errors.New("has a session with no userId; omit the session for an unauthenticated caller")
	}

	switch {
	case c.Expect.refusal() && c.Expect.Published != nil:
		return errors.New(`expects both an "error" and a "published" delta; an action either refused or wrote`)
	case !c.Expect.refusal() && c.Expect.Published == nil:
		return errors.New(`expects nothing; give it a "published" delta ({} for none) or an "error"`)
	}

	// A refusal unwinds the store without saving and drops the invocation's
	// whole ledger with it, so there is no delta and no reply to expect. Saying
	// otherwise is an assertion that can never hold.
	if c.Expect.refusal() {
		if len(c.Expect.Removed) != 0 {
			return errors.New(`expects an "error" and "removed" paths; a refused action publishes nothing`)
		}

		if len(c.Expect.Replies) != 0 {
			return errors.New(`expects an "error" and "replies"; a refused action's replies are dropped with it`)
		}
	}

	return nil
}

// state is the starting state for a case: its own, else its file's, else the
// game's own config.
func (s *Suite) state(f *Fixture, c Case) *models.Script {
	switch {
	case c.State != nil:
		return c.State
	case f.State != nil:
		return f.State
	default:
		return s.Script
	}
}

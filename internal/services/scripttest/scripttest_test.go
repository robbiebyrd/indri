// The harness's own tests, and the one place its two failure modes are held
// apart: a case that failed (the script is wrong) and a run that never happened
// (the harness is wrong). A test harness reporting success when it ran nothing
// at all is the vacuous result this file exists to make impossible, so several
// tests below assert that finding no cases is an error rather than a pass.
package scripttest

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/services/events"
)

// The two configs the tests run against: the worked example, which is the
// benchmark, and a script written to produce each outcome a fixture can
// describe.
const (
	exampleConfig = "../../../example/tictactoe/config.json"
	harnessConfig = "testdata/harness/config.json"
)

// TestMain silences the host's own logging for the run.
//
// Most of what these tests drive is a script failing, and the host logs every
// one with a correlation id and a Lua traceback before packing it into the
// frame its caller is answered with. Those lines are the expected output of a
// passing test, and letting them reach stderr would bury a real failure.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)

	os.Exit(m.Run())
}

// runSuite loads and plays a suite, failing the test if it could not even be
// loaded — which is a different thing from a case failing and must never be
// mistaken for one.
func runSuite(t *testing.T, configPath string, paths ...string) []Result {
	t.Helper()

	suite, err := Load(configPath, paths)
	if err != nil {
		t.Fatalf("loading %v against %s: %v", paths, configPath, err)
	}

	runner, err := NewRunner(suite, 5*time.Second)
	if err != nil {
		t.Fatalf("preparing the runner: %v", err)
	}

	t.Cleanup(runner.Close)

	results := runner.Run(context.Background())

	if len(results) != suite.Cases() {
		t.Fatalf("the run produced %d results for %d cases", len(results), suite.Cases())
	}

	return results
}

// byName indexes results so a test can name the case it means.
func byName(t *testing.T, results []Result) map[string]Result {
	t.Helper()

	index := make(map[string]Result, len(results))

	for _, result := range results {
		index[result.Name] = result
	}

	return index
}

// --- the benchmark -----------------------------------------------------------

// The worked example, expressed as fixtures. Every case here is one of
// example/tictactoe/game_test.go's, and this test is the claim that the harness
// reaches the same verdict the Go suite does — against the same config.json,
// the same game.lua and the same store.
//
// It is the load-bearing test of the whole package. If a fixture can say what a
// Go golden test says, a game author never has to write the Go one.
func TestRun_ReproducesTheTicTacToeGoldenTests(t *testing.T) {
	results := runSuite(t, exampleConfig, "testdata/tictactoe")

	if len(results) == 0 {
		t.Fatal("the tic-tac-toe fixtures ran no cases at all")
	}

	for _, result := range results {
		if !result.Passed() {
			t.Errorf("%s failed:\n\t%s", result.Name, strings.Join(result.Failures, "\n\t"))
		}
	}
}

// A passing fixture is reported as passing, for each kind of outcome a case can
// describe: a write, a reply, and a handler that correctly did nothing.
func TestRun_ReportsSuccessForAPassingFixture(t *testing.T) {
	for _, result := range runSuite(t, harnessConfig, "testdata/harness") {
		if !result.Passed() {
			t.Errorf("%s failed:\n\t%s", result.Name, strings.Join(result.Failures, "\n\t"))
		}
	}
}

// --- a failing fixture -------------------------------------------------------

// Every way a case can be wrong is reported, named, and explained. The point of
// the table is the second column: a harness that said only "FAIL" would leave an
// author no better off than a game that does not work.
func TestRun_ReportsWhichCaseFailedAndWhy(t *testing.T) {
	results := byName(t, runSuite(t, harnessConfig, "testdata/failing", "testdata/faulting"))

	tests := map[string]string{
		"wrong/a wrong expected value":                              `data.note: want "something else", got "written"`,
		"wrong/a delta expected from a handler that writes nothing": "published 0 deltas, want exactly 1",
		"wrong/a refusal expected from a handler that succeeds":     "the action was accepted, want it refused",
		"wrong/a reply the case did not expect":                     "sent 1 replies, want 0",

		// A refusal that had already committed is the one a harness must not
		// wave through: the caller was told it failed and everybody else has
		// already seen it happen.
		"wrong/a refusal that had already written": "moved the game from version 0 to 1",

		// The script's own fault, which is the case that has a line to point at.
		"fault/a script that faults": "attempt to index",
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			result, ok := results[name]
			if !ok {
				t.Fatalf("the run produced no result named %q", name)
			}

			if result.Passed() {
				t.Fatalf("%q passed; it is written to fail", name)
			}

			if got := strings.Join(result.Failures, "\n"); !strings.Contains(got, want) {
				t.Errorf("%q failed with\n\t%s\nwant a line containing %q", name, got, want)
			}
		})
	}
}

// A script that faults rather than refusing is reported with its file and its
// line.
//
// That is the whole difference between a harness an author can act on and one
// that only says no. The message reaches here through the same frame a player
// would be sent, so this also pins that the frame still carries it.
func TestRun_ReportsTheScriptsFileAndLine(t *testing.T) {
	results := byName(t, runSuite(t, harnessConfig, "testdata/faulting"))

	result := results["fault/a script that faults"]

	got := strings.Join(result.Failures, "\n")

	for _, want := range []string{"game.lua:", "attempt to index"} {
		if !strings.Contains(got, want) {
			t.Errorf("the failure reads\n\t%s\nwant it to name %q", got, want)
		}
	}
}

// --- a malformed fixture -----------------------------------------------------

// A fixture that cannot mean what it says is a load error, not a case that
// quietly passes.
//
// Every entry here is a fixture that would otherwise assert nothing at all:
// a misspelled key, a case with no expectation, an outcome that cannot happen.
// The first two rows are the vacuous ones — a file with no cases and a
// directory with no files — and they are why this test exists.
func TestLoad_RefusesAMalformedFixture(t *testing.T) {
	tests := map[string]struct {
		path string
		want string
	}{
		"a file declaring no cases": {
			path: "testdata/malformed/no-cases.test.json",
			want: "declares no cases",
		},
		"a directory holding no fixtures": {
			path: "testdata/nofixtures",
			want: "no .test.json fixture files found",
		},
		"a misspelled top-level key": {
			path: "testdata/malformed/unknown-key.test.json",
			want: `unknown field "case"`,
		},
		"a case expecting both an error and a delta": {
			path: "testdata/malformed/both-outcomes.test.json",
			want: "an action either refused or wrote",
		},
		"a case expecting nothing": {
			path: "testdata/malformed/no-outcome.test.json",
			want: "expects nothing",
		},
		"a case with no name": {
			path: "testdata/malformed/unnamed.test.json",
			want: "case 1 has no name",
		},
		"two cases with one name": {
			path: "testdata/malformed/duplicate-names.test.json",
			want: "two cases are both named",
		},
		"a case dispatching no action": {
			path: "testdata/malformed/no-action.test.json",
			want: "dispatches no action",
		},
		"a refusal that also expects replies": {
			path: "testdata/malformed/refusal-with-replies.test.json",
			want: "replies are dropped with it",
		},
		"a session that is nobody": {
			path: "testdata/malformed/session-without-user.test.json",
			want: "session with no userId",
		},
		"a file that is not json": {
			path: "testdata/malformed/not-json.test.json",
			want: "parsing fixture",
		},
		"a path that does not exist": {
			path: "testdata/there-is-no-such-directory",
			want: "reading fixture path",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite, err := Load(harnessConfig, []string{test.path})
			if err == nil {
				t.Fatalf("loading %s succeeded with %d cases, want an error containing %q",
					test.path, suite.Cases(), test.want)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("loading %s failed with %q, want an error containing %q", test.path, err, test.want)
			}
		})
	}
}

// A config the server could not boot is a load error too, reported before any
// case runs.
func TestNewRunner_RefusesAConfigTheServerCouldNotBoot(t *testing.T) {
	tests := map[string]struct {
		config string
		want   string
	}{
		"a script that does not parse": {
			config: "testdata/syntaxerror/config.json",
			want:   "broken.lua",
		},
		"a grant this build does not have": {
			config: "testdata/badgrant/config.json",
			want:   "no-such-capability",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite, err := Load(test.config, []string{"testdata/harness/pass.test.json"})
			if err != nil {
				t.Fatalf("loading the fixtures against %s: %v", test.config, err)
			}

			runner, err := NewRunner(suite, time.Second)
			if err == nil {
				runner.Close()

				t.Fatalf("%s built a runner, want an error containing %q", test.config, test.want)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("%s failed with %q, want an error containing %q", test.config, err, test.want)
			}
		})
	}
}

// --- the comparison ----------------------------------------------------------

// delta builds one published change event carrying updated, plus the updatedAt
// every committed save stamps.
func delta(updated map[string]interface{}, removed []string) events.ChangeEvent {
	fields := map[string]interface{}{"updatedAt": time.Now()}

	for path, value := range updated {
		fields[path] = value
	}

	return events.ChangeEvent{ID: "game", UpdatedFields: fields, RemovedFields: removed}
}

// The delta comparison, driven directly.
//
// Several of these are branches no script can be made to take on demand — a
// save that stamped no updatedAt, a handler that published twice — and they are
// the ones a harness most needs to be right about, because each of them is a
// way for a wrong script to look correct.
func TestCheckDelta(t *testing.T) {
	tests := map[string]struct {
		want        map[string]interface{}
		wantRemoved []string
		published   []events.ChangeEvent
		failures    []string
	}{
		"a matching delta": {
			want:      map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{delta(map[string]interface{}{"data.note": "written"}, nil)},
		},
		"a whole number is not a float": {
			// The fixture's value comes from JSON and is a float64; the delta's
			// came through events.ToMap and may be an int. Comparing them with
			// reflect.DeepEqual would fail every counter a game keeps.
			want:      map[string]interface{}{"data.score": float64(3)},
			published: []events.ChangeEvent{delta(map[string]interface{}{"data.score": 3}, nil)},
		},
		"a delta with no updatedAt did not come from a save": {
			want: map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{{
				ID:            "game",
				UpdatedFields: map[string]interface{}{"data.note": "written"},
			}},
			failures: []string{"the delta carries no updatedAt, so no save produced it"},
		},
		"a path the case did not expect": {
			want: map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{delta(map[string]interface{}{
				"data.note":   "written",
				"data.secret": "leaked",
			}, nil)},
			failures: []string{`data.secret: want (absent), got "leaked"`},
		},
		"a path the action never wrote": {
			want:      map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{delta(nil, nil)},
			failures:  []string{`data.note: want "written", got (absent)`},
		},
		"two deltas are two writes": {
			want: map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{
				delta(map[string]interface{}{"data.note": "half"}, nil),
				delta(map[string]interface{}{"data.note": "written"}, nil),
			},
			failures: []string{"the action published 2 deltas, want exactly 1"},
		},
		"no delta at all": {
			want:      map[string]interface{}{"data.note": "written"},
			failures:  []string{"the action published 0 deltas, want exactly 1"},
			published: nil,
		},
		"nothing expected and nothing published": {
			want: map[string]interface{}{},
		},
		"nothing expected but something published": {
			// Built without the updatedAt delta adds, because the failure quotes
			// what was published and a clock reading is not comparable.
			want: map[string]interface{}{},
			published: []events.ChangeEvent{{
				ID:            "game",
				UpdatedFields: map[string]interface{}{"data.note": "written"},
			}},
			failures: []string{`the action published 1 deltas, want none: {"data.note":"written"}`},
		},
		"a removed path the case expected": {
			want:        map[string]interface{}{"data.note": "written"},
			wantRemoved: []string{"data.old"},
			published: []events.ChangeEvent{
				delta(map[string]interface{}{"data.note": "written"}, []string{"data.old"}),
			},
		},
		"a removal the case did not expect": {
			want: map[string]interface{}{"data.note": "written"},
			published: []events.ChangeEvent{
				delta(map[string]interface{}{"data.note": "written"}, []string{"data.old"}),
			},
			failures: []string{`removed: want [], got ["data.old"]`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := checkDelta(test.want, test.wantRemoved, test.published)

			if len(got) != len(test.failures) {
				t.Fatalf("reported %d failures %v, want %d %v", len(got), got, len(test.failures), test.failures)
			}

			for i, want := range test.failures {
				if got[i] != want {
					t.Errorf("failure %d reads %q, want %q", i+1, got[i], want)
				}
			}
		})
	}
}

// An absent "replies" is the assertion that the action replied nothing, not a
// case that does not care.
func TestCheckReplies(t *testing.T) {
	reply := func(value string) map[string]interface{} {
		return map[string]interface{}{"hello": value}
	}

	tests := map[string]struct {
		want, got []map[string]interface{}
		failures  []string
	}{
		"none expected and none sent": {},
		"a matching reply": {
			want: []map[string]interface{}{reply("world")},
			got:  []map[string]interface{}{reply("world")},
		},
		"an unexpected reply": {
			got:      []map[string]interface{}{reply("world")},
			failures: []string{`the action sent 1 replies, want 0; it sent [{"hello":"world"}]`},
		},
		"a reply that says something else": {
			want:     []map[string]interface{}{reply("world")},
			got:      []map[string]interface{}{reply("nobody")},
			failures: []string{`reply 1: want {"hello":"world"}, got {"hello":"nobody"}`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := checkReplies(test.want, test.got)

			if len(got) != len(test.failures) {
				t.Fatalf("reported %d failures %v, want %d %v", len(got), got, len(test.failures), test.failures)
			}

			for i, want := range test.failures {
				if got[i] != want {
					t.Errorf("failure %d reads %q, want %q", i+1, got[i], want)
				}
			}
		})
	}
}

// --- the report --------------------------------------------------------------

// The report names every case, marks the failing ones, prints their reasons and
// counts them. Its return value is what the exit code is made of.
func TestReport(t *testing.T) {
	tests := map[string]struct {
		results []Result
		passed  bool
		want    []string
	}{
		"all passing": {
			results: []Result{{Name: "move/one"}, {Name: "move/two"}},
			passed:  true,
			want:    []string{"move/one", "ok", "2 cases, all ok"},
		},
		"one failing": {
			results: []Result{{Name: "move/one"}, {Name: "move/two", Failures: []string{"a reason"}}},
			passed:  false,
			want:    []string{"move/two", "FAIL", "- a reason", "2 cases, 1 FAILED"},
		},
		"a single case reads as one": {
			results: []Result{{Name: "move/one"}},
			passed:  true,
			want:    []string{"1 case, all ok"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			out := &bytes.Buffer{}

			if got := Report(out, test.results); got != test.passed {
				t.Errorf("Report reported passed=%v, want %v", got, test.passed)
			}

			for _, want := range test.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the report reads\n%s\nwant it to contain %q", out, want)
				}
			}
		})
	}
}

// --- check -------------------------------------------------------------------

// check compiles the scripts and prints the manifest the router, the REST route
// table and the scheduler are all built from — with no database and no port.
func TestCheck_ReportsWhatAConfigDeclares(t *testing.T) {
	out := &bytes.Buffer{}

	if err := Check(out, exampleConfig); err != nil {
		t.Fatalf("checking %s: %v", exampleConfig, err)
	}

	for _, want := range []string{"is valid", "game.lua (no grants)", "actions\n  move", "lifecycle events\n  (none)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("check reported\n%s\nwant it to contain %q", out, want)
		}
	}
}

// Every failure a server would hit at boot is a check failure.
func TestCheck_RefusesAConfigTheServerCouldNotBoot(t *testing.T) {
	tests := map[string]struct {
		config string
		want   string
	}{
		"a missing config":             {config: "testdata/there-is-no-config.json", want: "reading script file"},
		"a script that does not parse": {config: "testdata/syntaxerror/config.json", want: "broken.lua"},
		"an unknown grant":             {config: "testdata/badgrant/config.json", want: "no-such-capability"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			out := &bytes.Buffer{}

			err := Check(out, test.config)
			if err == nil {
				t.Fatalf("checking %s succeeded, want an error containing %q", test.config, test.want)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("checking %s failed with %q, want an error containing %q", test.config, err, test.want)
			}
		})
	}
}

// --- the command line --------------------------------------------------------

// The exit code is the whole point of the binary: it is meant for CI, where
// nobody reads the report unless the build goes red.
//
// Three codes, and the difference between the last two matters. A failed case
// says the script is wrong; a bad-args exit says the harness never ran, which
// is the result a green build must never be able to hide.
func TestMain_ExitCodes(t *testing.T) {
	tests := map[string]struct {
		args []string
		code int
		out  []string
		err  []string
	}{
		"a passing suite": {
			args: []string{"test", "-script", harnessConfig, "testdata/harness"},
			code: ExitOK,
			out:  []string{"all ok"},
		},
		"a game directory holding its own config": {
			args: []string{"test", "testdata/harness"},
			code: ExitOK,
			out:  []string{"all ok"},
		},
		"the worked example": {
			args: []string{"test", "-script", exampleConfig, "testdata/tictactoe"},
			code: ExitOK,
			out:  []string{"move/places the marker at row then column", "all ok"},
		},
		"a failing suite": {
			args: []string{"test", "-script", harnessConfig, "testdata/failing"},
			code: ExitFailed,
			out:  []string{"wrong/a wrong expected value", "FAIL", "5 FAILED"},
		},
		"a malformed fixture": {
			args: []string{"test", "-script", harnessConfig, "testdata/malformed/no-cases.test.json"},
			code: ExitBadArgs,
			err:  []string{"declares no cases"},
		},
		"a directory with no fixtures": {
			args: []string{"test", "-script", harnessConfig, "testdata/nofixtures"},
			code: ExitBadArgs,
			err:  []string{"no .test.json fixture files found"},
		},
		"test with no paths": {
			args: []string{"test"},
			code: ExitBadArgs,
			err:  []string{"name at least one fixture file or directory"},
		},
		"a valid config": {
			args: []string{"check", harnessConfig},
			code: ExitOK,
			out:  []string{"is valid"},
		},
		"a config that would not boot": {
			args: []string{"check", "testdata/badgrant/config.json"},
			code: ExitBadArgs,
			err:  []string{"no-such-capability"},
		},
		"check with two configs": {
			args: []string{"check", harnessConfig, harnessConfig},
			code: ExitBadArgs,
			err:  []string{"name one config file"},
		},
		"no command at all": {
			args: []string{},
			code: ExitBadArgs,
			err:  []string{"usage:"},
		},
		"an unknown command": {
			args: []string{"explode"},
			code: ExitBadArgs,
			err:  []string{`unknown command "explode"`},
		},
		"help": {
			args: []string{"--help"},
			code: ExitOK,
			out:  []string{"usage:"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

			if got := Main(test.args, stdout, stderr); got != test.code {
				t.Errorf("indri-script %v exited %d, want %d\nstdout:\n%s\nstderr:\n%s",
					test.args, got, test.code, stdout, stderr)
			}

			for _, want := range test.out {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout reads\n%s\nwant it to contain %q", stdout, want)
				}
			}

			for _, want := range test.err {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr reads\n%s\nwant it to contain %q", stderr, want)
				}
			}
		})
	}
}

// TestNewRunner_RefusesAConfigWithNoScripts separates "the harness could not
// run" from "your script is wrong".
//
// A config declaring no scripts answers every case with "no lua handler is
// registered", which reads as a failing game when the truth is that no game was
// loaded. The likeliest way to reach it is a config resolved by fallback:
// naming a directory of fixtures that holds no config.json lands on the one in
// the working directory, which in this repo is the framework's own and declares
// nothing. That produced a screenful of red herrings before this check existed.
func TestNewRunner_RefusesAConfigWithNoScripts(t *testing.T) {
	suite, err := Load("testdata/noscripts/config.json", []string{"testdata/noscripts"})
	if err != nil {
		t.Fatalf("loading the fixtures: %v", err)
	}

	runner, err := NewRunner(suite, 5*time.Second)
	if err == nil {
		runner.Close()
		t.Fatal("a config declaring no scripts was accepted, so every case would have failed as though the game were wrong")
	}

	for _, want := range []string{"declares no lua scripts", "-script"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not mention %q, so it does not tell the author what to do", err.Error(), want)
		}
	}
}

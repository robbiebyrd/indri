package scripttest

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// The exit codes indri-script leaves behind.
//
// A failed case and a harness that could not run are deliberately different
// numbers. Both are non-zero, so CI fails either way, but "your script is
// wrong" and "your fixtures do not parse" want different responses from the
// person reading the build, and a single 1 makes them look alike.
const (
	ExitOK      = 0
	ExitFailed  = 1
	ExitBadArgs = 2
)

// defaultConfigName is the config a command falls back to, matching the default
// cmd/server runs with.
const defaultConfigName = "config.json"

const usage = `indri-script runs a game's Lua scripts without a server or a database.

usage:
  indri-script test [-script config.json] [-timeout 5s] [-v] <path>...
  indri-script check [config.json]

test    plays every ` + FixtureSuffix + ` fixture the paths name. A path is a fixture file,
        or a directory that is scanned for them. With a single directory and no
        -script, the config is that directory's ` + defaultConfigName + `.

check   compiles the scripts a config declares, resolves their grants and prints
        the actions and lifecycle events they register. Nothing is booted.

flags:
  -script   the config.json declaring the scripts (default: see above)
  -timeout  how long one case may run (default 5s)
  -v        let the server's own log output through, tracebacks included

a fixture file:
  {
    "state": { ... },                        // optional; defaults to the config's
    "cases": [
      {
        "name": "rejects a player out of turn",
        "state": { ... },                    // optional; overrides the file's
        "event": {
          "action": "move",
          "session": {"userId": "p1", "teamId": "Player 1"},
          "payload": {"move": "1,2"}
        },
        "expected": {
          "published": {"teams.Player 1.data.turn": false},
          "removed":   ["data.gone"],        // optional; defaults to none
          "replies":   [{"ok": true}]        // optional; defaults to none
        }
      }
    ]
  }

  "state" is a config.json, so a fixture is the game's own config with the one
  table it is about changed. "expected" holds either a "published" delta -- {}
  for an action that must write nothing -- or an "error" the script must have
  refused with. Omitting the caller's "session" is an unauthenticated caller.
`

// Main is indri-script, as a function so that its exit code and its output are
// testable without building and running a binary.
//
// cmd/indri-script/main.go is the three lines that hand it the process.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)

		return ExitBadArgs
	}

	switch args[0] {
	case "test":
		return runTest(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)

		return ExitOK
	default:
		fmt.Fprintf(stderr, "indri-script: unknown command %q\n\n%s", args[0], usage)

		return ExitBadArgs
	}
}

// runTest loads the suite and plays it.
func runTest(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("script", "", "the config.json declaring the scripts to test")
	timeout := flags.Duration("timeout", DefaultTimeout, "how long one case may run")
	verbose := flags.Bool("v", false, "let the server's own log output through")

	if err := flags.Parse(args); err != nil {
		return ExitBadArgs
	}

	paths := flags.Args()

	if len(paths) == 0 {
		fmt.Fprintf(stderr, "indri-script test: name at least one fixture file or directory\n\n%s", usage)

		return ExitBadArgs
	}

	restore := quieten(*verbose, stderr)
	defer restore()

	suite, err := Load(configFor(*configPath, paths), paths)
	if err != nil {
		fmt.Fprintf(stderr, "indri-script test: %v\n", err)

		return ExitBadArgs
	}

	runner, err := NewRunner(suite, *timeout)
	if err != nil {
		fmt.Fprintf(stderr, "indri-script test: %v\n", err)

		return ExitBadArgs
	}

	defer runner.Close()

	if Report(stdout, runner.Run(context.Background())) {
		return ExitOK
	}

	return ExitFailed
}

// runCheck compiles a config's scripts and prints what they declare.
func runCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)

	verbose := flags.Bool("v", false, "let the server's own log output through")

	if err := flags.Parse(args); err != nil {
		return ExitBadArgs
	}

	configPath := defaultConfigName

	switch paths := flags.Args(); len(paths) {
	case 0:
	case 1:
		configPath = paths[0]
	default:
		fmt.Fprintf(stderr, "indri-script check: name one config file, not %d\n\n%s", len(paths), usage)

		return ExitBadArgs
	}

	restore := quieten(*verbose, stderr)
	defer restore()

	if err := Check(stdout, configPath); err != nil {
		fmt.Fprintf(stderr, "indri-script check: %v\n", err)

		return ExitBadArgs
	}

	return ExitOK
}

// configFor decides which config the fixtures are run against.
//
// A single directory is taken to be the game's own, because that is how a game
// is laid out and how the plan spells the command: `indri-script test
// ./example/tictactoe`. Anything else falls back to the working directory's
// config.json, which is what cmd/server defaults to. -script overrides both,
// and is what a fixture set kept apart from the game it tests needs.
func configFor(flagValue string, paths []string) string {
	if flagValue != "" {
		return flagValue
	}

	if len(paths) == 1 {
		candidate := filepath.Join(paths[0], defaultConfigName)

		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return defaultConfigName
}

// quieten silences the server's own logging for the run, and returns the undo.
//
// Most of a good fixture set asserts that an action was refused, and the host
// logs every script failure with a correlation id and a Lua traceback before
// packing it into the frame its caller is answered with. Those lines are the
// expected output of a passing run, and letting them through would bury the
// report they surround. -v is for the case where the traceback is the thing
// wanted.
func quieten(verbose bool, stderr io.Writer) func() {
	previous := log.Writer()

	if verbose {
		log.SetOutput(stderr)
	} else {
		log.SetOutput(io.Discard)
	}

	return func() { log.SetOutput(previous) }
}

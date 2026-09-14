// Command indri-script runs a game's Lua scripts against fixture state, with no
// server and no database.
//
// It is what makes "a game author writes no Go" true: the whole of the work is
// in internal/services/scripttest, and this file is only the process. Keeping it
// this thin is deliberate — a main package cannot be imported, so anything that
// lived here could only be tested by building and running a binary, and the
// harness's own exit codes are among the things most worth a test.
package main

import (
	"os"

	"github.com/robbiebyrd/indri/internal/services/scripttest"
)

func main() {
	os.Exit(scripttest.Main(os.Args[1:], os.Stdout, os.Stderr))
}

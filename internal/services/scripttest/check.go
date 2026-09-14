package scripttest

import (
	"fmt"
	"io"
	"strings"

	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
)

// Check loads a config the way the server does and reports what it declares,
// without booting anything.
//
// It is the same construction internal/injector performs: the config is read
// and its script paths resolved, every script is compiled, each one's grants
// are resolved against the capabilities this build ships, and the manifest is
// collected and then checked against a second, independently prepared state.
// Every failure a server would hit at boot therefore surfaces here — a syntax
// error, a grant naming a capability that does not exist, a claimed action that
// is reserved, a registration that differs between states — with no database,
// no port and no transport.
//
// What it prints is the other half of its job: the actions and lifecycle events
// the scripts registered are what the router, the REST route table and the
// scheduler are all built from, and they are not written down anywhere else.
func Check(out io.Writer, configPath string) error {
	store, err := scriptRepo.NewStore(configPath)
	if err != nil {
		return err
	}

	script := store.Get()

	engine, err := luaService.NewEngineWithGrants(script.Scripts, nil)
	if err != nil {
		return fmt.Errorf("loading the game scripts declared by %q: %w", configPath, err)
	}

	defer engine.Close()

	fmt.Fprintf(out, "%s is valid\n\nscripts\n", configPath)

	if len(script.Scripts) == 0 {
		// A config with no scripts loads and serves nothing but the built-in
		// actions. It is legal, and it is also how a working game silently
		// becomes a shell, so it is said out loud rather than shown as an empty
		// list.
		fmt.Fprintf(out, "  (none — this game declares no Lua scripts and will answer no game actions)\n")
	}

	for _, file := range script.Scripts {
		fmt.Fprintf(out, "  %s (%s)\n", file.Path, grants(file.Grants))
	}

	fmt.Fprintf(out, "\nactions\n%s\nlifecycle events\n%s", list(engine.Actions()), list(engine.Lifecycle()))

	return nil
}

// grants names what a script was allowed to reach, which is meant to be read at
// a glance rather than inferred from the script.
func grants(granted []string) string {
	if len(granted) == 0 {
		return "no grants"
	}

	return "grants: " + strings.Join(granted, ", ")
}

// list renders a manifest, one name per line, saying so when it is empty.
func list(names []string) string {
	if len(names) == 0 {
		return "  (none)\n"
	}

	return "  " + strings.Join(names, "\n  ") + "\n"
}

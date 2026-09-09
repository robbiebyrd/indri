package boot

import (
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

const actionsDir = "../../handlers/actions"

// registeredActions runs the real registration against an injector carrying
// only a melody hub. Handler constructors just store the injector, and
// registerHandlers dereferences nothing else, so no database is needed. It
// returns each action mapped to the base package name of the handler bound to
// it, so tests can check not just presence but correct wiring.
func registeredActions(t *testing.T) map[string]string {
	t.Helper()

	// Reset first so the append-only global registry doesn't accumulate across
	// repeated runs (e.g. -count=2 or a future second test in this package).
	router.Reset()
	t.Cleanup(router.Reset)

	registerHandlers(&injector.Injector{
		ClientsInjector: &injector.ClientsInjector{Transport: ws.New()},
	})

	actions := make(map[string]string)

	for _, h := range router.RegisteredHandlers() {
		// The handler is a *<pkg>.Handler; its package base name is the action
		// package it came from.
		actions[h.Action] = path.Base(reflect.TypeOf(h.Handler).Elem().PkgPath())
	}

	return actions
}

// TestRegisterHandlers_CoversEveryActionPackage guards against an action being
// implemented but never wired into the router, which silently makes it
// unreachable from clients. Each directory under handlers/actions is one
// action, named after its package.
func TestRegisterHandlers_CoversEveryActionPackage(t *testing.T) {
	registered := registeredActions(t)

	entries, err := os.ReadDir(actionsDir)
	if err != nil {
		t.Fatalf("reading %v: %v", actionsDir, err)
	}

	found := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		found++

		if _, ok := registered[entry.Name()]; !ok {
			t.Errorf(
				"action %q is implemented in %v but not registered in registerHandlers, so clients cannot reach it",
				entry.Name(),
				filepath.Join(actionsDir, entry.Name()),
			)
		}
	}

	if found == 0 {
		t.Fatalf("found no action packages under %v; the test is not checking anything", actionsDir)
	}
}

// TestRegisterHandlers_BindsCorrectHandler guards against a mis-wiring where an
// action is registered but pointed at the wrong handler package (e.g. "kick"
// bound to login.New) — a swap that presence-only checks would miss.
func TestRegisterHandlers_BindsCorrectHandler(t *testing.T) {
	for action, handlerPkg := range registeredActions(t) {
		if action != handlerPkg {
			t.Errorf("action %q is wired to the %q handler package; expected a handler from the %q package",
				action, handlerPkg, action)
		}
	}
}

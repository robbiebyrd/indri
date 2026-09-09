package boot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/injector"
)

const actionsDir = "../../handlers/actions"

// registeredActions runs the real registration against an injector carrying
// only a melody hub. Handler constructors just store the injector, and
// registerHandlers dereferences nothing else, so no database is needed.
func registeredActions(t *testing.T) map[string]bool {
	t.Helper()

	registerHandlers(&injector.Injector{
		ClientsInjector: &injector.ClientsInjector{MelodyClient: melody.New()},
	})

	actions := make(map[string]bool)
	for _, h := range router.RegisteredHandlers() {
		actions[h.Action] = true
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

		if !registered[entry.Name()] {
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

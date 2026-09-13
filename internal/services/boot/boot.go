package boot

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions/script"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// minLeaseHeadroom is how much larger the lock lease must be than one script
// invocation's deadline.
//
// A script edits game state inside the store's apply closure, so it runs with
// the game's distributed lock held. If an invocation could outlive the lease,
// another instance could acquire the same lock and run its own script against
// the same game concurrently. The version fence still keeps the *write* safe —
// one of them loses the CAS and retries — but both scripts will have run, and a
// script's effects are not confined to the document.
//
// 10 rather than 2: the deadline bounds the Lua call alone, while the lease has
// to cover that plus the lock acquisition, two database round trips and the
// retry loop around them. A factor that only just held would be one slow query
// away from not holding.
const minLeaseHeadroom = 10

func Boot(ctx context.Context, scriptFilePath *string) (*injector.Injector, error) {
	if err := checkScriptDeadline(script.InvocationTimeout, lock.DefaultRedisLeaseTTL); err != nil {
		return nil, err
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if scriptFilePath == nil || *scriptFilePath == "" {
		s := dir + "/config.json"
		scriptFilePath = &s
	}

	clients, err := injector.GetClients(ctx, nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing clients: %w", err)
	}

	repos, err := injector.GetRepos(ctx, clients, *scriptFilePath)
	if err != nil {
		return nil, fmt.Errorf("initializing repos: %w", err)
	}

	services, err := injector.GetServices(ctx, clients, repos)
	if err != nil {
		return nil, fmt.Errorf("initializing services: %w", err)
	}

	i := &injector.Injector{
		ReposInjector:    repos,
		ClientsInjector:  clients,
		ServicesInjector: services,
		GlobalContext:    ctx,
		Script:           repos.ScriptRepo.Get(),
	}

	registerHandlers(i)

	return i, nil
}

// checkScriptDeadline refuses to start a server whose script deadline is not
// comfortably inside its lock lease.
//
// Both values are constants today, so this cannot fire on a stock build — which
// is the point. It fires the moment someone raises the script deadline or
// lowers the lease without noticing they are coupled, and it fails at startup
// where it is one line to read, rather than as two scripts intermittently
// running against one game under load.
func checkScriptDeadline(deadline, lease time.Duration) error {
	if deadline <= 0 {
		return fmt.Errorf("the script invocation deadline must be positive, got %v", deadline)
	}

	if lease < deadline*minLeaseHeadroom {
		return fmt.Errorf(
			"the script invocation deadline (%v) is too close to the lock lease (%v): "+
				"a script holding a game's lock past its lease lets another instance run one "+
				"concurrently, so the lease must be at least %dx the deadline (%v)",
			deadline, lease, minLeaseHeadroom, deadline*minLeaseHeadroom,
		)
	}

	return nil
}

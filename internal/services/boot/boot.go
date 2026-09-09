package boot

import (
	"context"
	"fmt"
	"os"

	"github.com/robbiebyrd/indri/internal/injector"
)

func Boot(ctx context.Context, scriptFilePath *string) (*injector.Injector, error) {
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

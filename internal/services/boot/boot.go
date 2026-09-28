package boot

import (
	"context"
	"fmt"
	"os"

	"github.com/robbiebyrd/indri/internal/handlers/luahandler"
	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
)

func Boot(ctx context.Context, scriptFilePath *string) (*injector.Injector, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if scriptFilePath == nil || *scriptFilePath == "" {
		// TODO: The absence of a valid script file path should panic and halt the server from starting.
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

	script := repos.ScriptRepo.Get()
	if err := validateScript(script); err != nil {
		return nil, fmt.Errorf("invalid script: %w", err)
	}

	i := &injector.Injector{
		ReposInjector:    repos,
		ClientsInjector:  clients,
		ServicesInjector: services,
		GlobalContext:    ctx,
		Script:           script,
	}

	registerHandlers(i)
	luahandler.Register(i)

	return i, nil
}

func validateScript(script *models.Script) error {
	if script.Config.MaxTeams <= 0 {
		return fmt.Errorf("config.maxTeams must be > 0, got %d", script.Config.MaxTeams)
	}
	if script.Config.MaxTeams != len(script.Teams) {
		return fmt.Errorf(
			"config.maxTeams (%d) does not match number of declared teams (%d)",
			script.Config.MaxTeams, len(script.Teams),
		)
	}
	if script.Config.MaxPlayersPerTeam <= 0 {
		return fmt.Errorf("config.maxPlayersPerTeam must be > 0, got %d", script.Config.MaxPlayersPerTeam)
	}
	return nil
}

package boot

import (
	"context"
	"fmt"
	"log"
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
		log.Fatalf("invalid script: %v", err)
	}
	layoutHash, layoutData := injector.ComputeLayoutHash(script)

	i := &injector.Injector{
		ReposInjector:    repos,
		ClientsInjector:  clients,
		ServicesInjector: services,
		GlobalContext:    ctx,
		Script:           script,
		LayoutHash:       layoutHash,
		LayoutData:       layoutData,
	}

	registerHandlers(i)
	luahandler.Register(i)

	return i, nil
}

func validateScript(script *models.Script) error {
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

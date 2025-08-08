package boot

import (
	"context"
	"log"
	"os"

	"github.com/robbiebyrd/indri/internal/injector"
)

func Boot(scriptFilePath *string) (*injector.Injector, error) {
	ctx := context.Background()

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if scriptFilePath == nil || *scriptFilePath == "" {
		s := dir + "/config.json"
		scriptFilePath = &s
	}

	clients, err := injector.GetClients(ctx, nil, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	repos, err := injector.GetRepos(ctx, clients, *scriptFilePath)
	if err != nil {
		log.Fatal(err)
	}

	services, err := injector.GetServices(ctx, clients, repos)
	if err != nil {
		log.Fatal(err)
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

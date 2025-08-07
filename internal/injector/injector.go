package injector

import (
	"context"

	"github.com/olahol/melody"

	"github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/models"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	connectionService "github.com/robbiebyrd/indri/internal/services/connection"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	userService "github.com/robbiebyrd/indri/internal/services/user"
)

type ReposInjector struct {
	EnvVars     *envVars.Vars
	GameRepo    *gameRepo.Repo
	UserRepo    *userRepo.Repo
	SessionRepo *sessionRepo.Repo
}

type ClientsInjector struct {
	MongoDBClient *mongodb.Client
	MelodyClient  *melody.Melody
	GlobalMonitor *changestream.MongoChangeMonitor
	Context       context.Context
}

type ServicesInjector struct {
	GameService       *gameService.Service
	ConnectionService *connectionService.Service
	UserService       *userService.Service
}

type Injector struct {
	*ReposInjector
	*ClientsInjector
	*ServicesInjector
	Script        *models.Script
	GlobalContext context.Context
}

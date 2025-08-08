package injector

import (
	"context"

	"github.com/olahol/melody"

	mongodbClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/entrypoints/changestream"
	"github.com/robbiebyrd/indri/internal/models"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	authSevice "github.com/robbiebyrd/indri/internal/services/authentication"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
)

type ReposInjector struct {
	EnvVars     *envVars.Vars
	GameRepo    *gameRepo.Store
	UserRepo    *userRepo.Store
	SessionRepo *sessionRepo.Store
	ScriptRepo *scriptRepo.Store
}

type ClientsInjector struct {
	MongoDBClient *mongodbClient.Client
	MelodyClient  *melody.Melody
	GlobalMonitor *changestream.MongoChangeMonitor
}

type ServicesInjector struct {
	GameService      *gameService.Service
	BroadcastService *broadcastService.Service
	UserService      *userService.Service
	AuthService      *authSevice.Service
	SessionService   *sessionService.Service
}

type Injector struct {
	*ReposInjector
	*ClientsInjector
	*ServicesInjector
	Script        *models.Script
	GlobalContext context.Context
}

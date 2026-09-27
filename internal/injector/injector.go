package injector

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	mongodbClient "github.com/robbiebyrd/indri/internal/clients/mongodb"
	"github.com/robbiebyrd/indri/internal/models"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	authSevice "github.com/robbiebyrd/indri/internal/services/authentication"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	"github.com/robbiebyrd/indri/internal/services/events"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"github.com/robbiebyrd/indri/internal/services/lock"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
	"github.com/robbiebyrd/indri/internal/transport"
)

type ReposInjector struct {
	EnvVars     *envVars.Vars
	GameRepo    gameRepo.Storer
	UserRepo    userRepo.Storer
	SessionRepo sessionRepo.Storer
	ScriptRepo  *scriptRepo.Store
	// SQLDB is the database/sql pool shared by the sqlite and postgres
	// stores, closed on shutdown; nil for the mongodb and memory backends.
	SQLDB *sql.DB
}

type ClientsInjector struct {
	MongoDBClient *mongodbClient.Client
	Transport     transport.Transport
	LockManager   lock.Manager
	Publisher     events.Publisher
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
	LayoutHash    string
	LayoutData    map[string]interface{}
	GlobalContext context.Context
}

// ComputeLayoutHash extracts the layout data from the script's public data,
// computes a short stable hash, and returns both. Called once at boot time.
func ComputeLayoutHash(script *models.Script) (string, map[string]interface{}) {
	if script == nil || script.PublicData == nil {
		return "", nil
	}
	layout, _ := script.PublicData["layout"].(map[string]interface{})
	raw, _ := json.Marshal(layout)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:8]), layout
}

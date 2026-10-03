package injector

import (
	"context"
	"errors"
	"fmt"
	"log"

	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scheduleRepo "github.com/robbiebyrd/indri/internal/repo/schedule"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
)

// stores are the repositories that follow INDRI_DB_BACKEND.
type stores struct {
	game     gameRepo.Storer
	user     userRepo.Storer
	session  sessionRepo.Storer
	schedule scheduleRepo.Storer
}

// openStores builds every repository over the one backend named, so games,
// users and sessions always live in the same database.
//
// Timers are the exception: the schedule store exists only for MongoDB and in
// memory, so the SQL backends keep timers in memory, and say so at boot.
func openStores(ctx context.Context, backend string, clients *ClientsInjector) (stores, error) {
	switch backend {
	case backendMongoDB:
		return mongoStores(ctx, clients)
	case backendMemory:
		return stores{
			game:     gameRepo.NewMemoryStore(ctx, clients.LockManager, clients.Publisher),
			user:     userRepo.NewMemoryStore(),
			session:  sessionRepo.NewMemoryStore(),
			schedule: scheduleRepo.NewMemoryStore(scheduleRepo.Config{}),
		}, nil
	case backendSQLite:
		if clients.SQLDB == nil {
			return stores{}, errors.New("INDRI_DB_BACKEND is sqlite but no SQLite database was opened")
		}

		return stores{
			game:     gameRepo.NewSQLiteStore(ctx, clients.SQLDB, clients.LockManager, clients.Publisher),
			user:     userRepo.NewSQLiteStore(ctx, clients.SQLDB),
			session:  sessionRepo.NewSQLiteStore(ctx, clients.SQLDB),
			schedule: memoryTimers(backend),
		}, nil
	case backendPostgres:
		return postgresStores(ctx, clients)
	default:
		return stores{}, fmt.Errorf("INDRI_DB_BACKEND is %q, which names no backend", backend)
	}
}

func mongoStores(ctx context.Context, clients *ClientsInjector) (stores, error) {
	if clients.MongoDBClient == nil {
		return stores{}, errors.New("INDRI_DB_BACKEND is mongodb but no MongoDB client was opened")
	}

	game, err := gameRepo.NewStore(ctx, clients.MongoDBClient, clients.LockManager, clients.Publisher)
	if err != nil {
		return stores{}, err
	}

	user, err := userRepo.NewStore(ctx, clients.MongoDBClient)
	if err != nil {
		return stores{}, err
	}

	session, err := sessionRepo.NewStore(ctx, clients.MongoDBClient)
	if err != nil {
		return stores{}, err
	}

	schedule, err := scheduleRepo.NewStore(ctx, clients.MongoDBClient, scheduleRepo.Config{})
	if err != nil {
		return stores{}, err
	}

	return stores{game: game, user: user, session: session, schedule: schedule}, nil
}

func postgresStores(ctx context.Context, clients *ClientsInjector) (stores, error) {
	if clients.SQLDB == nil {
		return stores{}, errors.New("INDRI_DB_BACKEND is postgres but no PostgreSQL connection was opened")
	}

	game, err := gameRepo.NewPostgresStore(ctx, clients.SQLDB, clients.LockManager, clients.Publisher)
	if err != nil {
		return stores{}, err
	}

	user, err := userRepo.NewPostgresStore(ctx, clients.SQLDB)
	if err != nil {
		return stores{}, err
	}

	session, err := sessionRepo.NewPostgresStore(ctx, clients.SQLDB)
	if err != nil {
		return stores{}, err
	}

	return stores{game: game, user: user, session: session, schedule: memoryTimers(backendPostgres)}, nil
}

// memoryTimers is the schedule store for a backend with no schedule store of
// its own. Timers kept in memory are lost when the server stops and are not
// shared between instances, which an operator must hear about.
func memoryTimers(backend string) *scheduleRepo.MemoryStore {
	log.Printf(
		"WARNING: INDRI_DB_BACKEND is %s, which has no timer store; script timers (indri.after) are kept in memory,"+
			" lost on restart and not shared between instances",
		backend,
	)

	return scheduleRepo.NewMemoryStore(scheduleRepo.Config{})
}

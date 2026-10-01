package game

import (
	"context"

	"github.com/robbiebyrd/indri/internal/models"
)

// Storer is the contract a game store must satisfy. The assertions below keep
// it in step with every backend: change one without the others and the build
// fails.
var (
	_ Storer = (*Store)(nil)
	_ Storer = (*MemoryStore)(nil)
	_ Storer = (*SQLiteStore)(nil)
	_ Storer = (*PostgresStore)(nil)
)

type Storer interface {
	HasPlayerOnTeam(id string, teamId string, userId string) bool
	ChangePlayerTeam(id string, teamId string, userId string) error
	AddPlayerToTeam(id string, teamId string, userId string) error
	RemovePlayerFromTeam(id string, userId string) error
	PlayerOnWhichTeam(id string, userId string) (*string, error)
	New(code string, script *models.Script, privateGame bool) (*models.Game, error)
	Get(id string) (*models.Game, error)
	FindByCode(gameCode string) (*models.Game, error)
	FindOpen(limit int) ([]*models.Game, error)
	GetIDHex(gameCode string) (*string, error)
	Exists(id string) (bool, error)
	Update(id string, game *models.UpdateGame) error
	UpdateField(id string, key string, value interface{}) error
	DeleteField(id string, key string) error
	Mutate(ctx context.Context, id string, apply func(g *models.Game) error) error
	MutateResult(ctx context.Context, id string, apply func(g *models.Game) error) (committed bool, err error)
	HasPlayer(id string, userId string) bool
	PlayerOnATeam(id string, userId string) bool
	AddPlayer(id string, userId string, displayName string) error
	AddPlayerResult(id string, userId string, displayName string) (added bool, err error)
	RemovePlayer(id string, userId string) error
	RemovePlayerResult(id string, userId string) (removed bool, err error)
	ConnectPlayer(id string, userId string) error
	DisconnectPlayer(id string, userId string) error
	HasHost(id string) bool
	PlayerIsHost(id string, playerId string) bool
	UnsetHost(id string) error
	SetPlayerAsHost(id string, playerId string) error
}

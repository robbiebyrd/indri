package game

import "github.com/robbiebyrd/indri/internal/models"

// Storer is the contract a game store must satisfy. The assertion below keeps
// it in step with *Store: change one without the other and the build fails.
var _ Storer = (*Store)(nil)

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
	Mutate(id string, apply func(g *models.Game) error) error
	HasPlayer(id string, userId string) bool
	PlayerOnATeam(id string, userId string) bool
	AddPlayer(id string, userId string, displayName string) error
	RemovePlayer(id string, userId string) error
	ConnectPlayer(id string, userId string) error
	DisconnectPlayer(id string, userId string) error
	HasHost(id string) bool
	PlayerIsHost(id string, playerId string) bool
	UnsetHost(id string) error
	SetPlayerAsHost(id string, playerId string) error
}

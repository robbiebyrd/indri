package game

import "github.com/robbiebyrd/indri/internal/models"

// Storer is the contract a game store must satisfy. The assertion below keeps
// it in step with *Store: change one without the other and the build fails.
var _ Storer = (*MongoStore)(nil)

type Storer interface {
	New(code string, script *models.Script, privateGame bool) (*models.Game, error)
	Get(id string) (*models.Game, error)
	FindByCode(gameCode string) (*models.Game, error)
	FindOpen(limit int) ([]*models.Game, error)
	GetIDHex(gameCode string) (*string, error)
	Exists(code string) (bool, error)
	Update(id string, game *models.UpdateGame) error
	UpdateField(id string, key string, value interface{}) error
	DeleteField(id string, key string) error
	Mutate(id string, apply func(g *models.Game) error) error
	AssignSlot(id string, teamId string, userId string, displayName string) (string, error)
	RemovePlayer(id string, slotId string) error
	ConnectPlayer(id string, slotId string) error
	DisconnectPlayer(id string, slotId string) error
	HasHost(id string) bool
	PlayerIsHost(id string, playerId string) bool
	UnsetHost(id string) error
	SetPlayerAsHost(id string, playerId string) error
}

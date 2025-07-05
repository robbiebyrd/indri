package stage

import (
	"errors"
	"fmt"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/game"
	"github.com/robbiebyrd/indri/internal/services/script"
	"go.mongodb.org/mongo-driver/v2/bson"
	"slices"
)

type Service struct {
	gameRepo      *game.Repo
	scriptService *script.Service
}

var stageService *Service

// NewService creates a new repository for accessing game data.
func NewService(gameRepo *game.Repo, scriptService *script.Service) *Service {
	if gameRepo == nil {
		gameRepo = game.NewRepo()
	}

	if scriptService == nil {
		scriptService = script.NewService(nil)
	}

	if stageService == nil {
		stageService = &Service{
			gameRepo,
			scriptService,
		}
	}

	return stageService
}

// Sanitize removes private items.
func (gs *Service) Sanitize(game *models.Game) *models.Game {
	game.Stage.PrivateData = nil

	for i, g := range game.Stage.Scenes {
		g.PrivateData = nil
		game.Stage.Scenes[i] = g
	}

	for i, t := range game.Teams {
		for j, p := range t.Players {
			p.PrivateData = nil
			t.Players[j] = p
		}

		game.Teams[i] = t
	}

	return game
}

// Get will fetch the stage for a specific game code.
func (gs *Service) Get(gameCode string) (*models.Stage, error) {
	g, err := gs.gameRepo.Get(gameCode)
	if err != nil {
		return nil, err
	}

	return &g.Stage, nil
}

// New creates and returns a new stage object.
func (gs *Service) New() (*models.Stage, error) {
	return &models.Stage{}, nil
}

// AddScene adds a scene to the Stage.
func (gs *Service) AddScene(gameCode string, sceneId string, scene *models.Scene) error {
	if gameCode == "" {
		return fmt.Errorf("game code cannot be nil")
	}

	if sceneId == "" {
		return fmt.Errorf("scene id cannot be nil")
	}

	g, err := gs.gameRepo.FindByCode(gameCode)
	if err != nil {
		return err
	}

	_, ok := g.Stage.Scenes[sceneId]
	if ok {
		return fmt.Errorf("scene with id %s already exists", sceneId)
	}

	path := "stage.scenes." + sceneId

	err = gs.gameRepo.UpdateField(g.ID.Hex(), path, scene)
	if err != nil {
		return err
	}

	return nil
}

// AddScenes adds multiple scenes to the Stage.
func (gs *Service) AddScenes(gameCode string, scenes map[string]models.Scene) error {
	if gameCode == "" {
		return errors.New("game code cannot be nil")
	}

	if len(scenes) == 0 {
		return errors.New("scenes cannot be empty")
	}

	for sceneId, scene := range scenes {
		err := gs.AddScene(gameCode, sceneId, &scene)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteScene deletes a scene from the Stage.
func (gs *Service) DeleteScene(gameCode string, sceneId string) error {
	g, err := gs.validateAndFetchGame(gameCode, sceneId)
	if err != nil {
		return err
	}

	path := "stage.scenes." + sceneId

	err = gs.gameRepo.DeleteField(g.ID.Hex(), path)
	if err != nil {
		return err
	}

	return nil
}

// SetScript.
func (gs *Service) SetScript(gameCode string, id string) error {
	if gameCode == "" {
		return fmt.Errorf("game code cannot be nil")
	}

	g, err := gs.gameRepo.FindByCode(gameCode)
	if err != nil {
		return err
	}

	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	err = gs.gameRepo.UpdateField(g.ID.Hex(), "stage.scriptId", objectId)
	if err != nil {
		return err
	}

	return nil
}

// LoadFromScript loads a script's stage data into the current game's stage.
func (gs *Service) LoadFromScript(gameCode string, scriptId string) error {
	if gameCode == "" {
		return fmt.Errorf("game code cannot be nil")
	}

	gameId, err := gs.gameRepo.GetCode(gameCode)
	if err != nil {
		return err
	}

	loadedScript, err := gs.scriptService.Get(&scriptId)
	if err != nil {
		return err
	}

	err = gs.gameRepo.UpdateField(*gameId, "stage", loadedScript.Stage)
	if err != nil {
		return err
	}

	return nil
}

// LoadSceneFromScript loads a script's data for a given scene into the current game's stage.
func (gs *Service) LoadSceneFromScript(
	gameCode string,
	scriptId string,
	sceneId string,
	dataType *models.DataStoreType,
) error {
	if gameCode == "" {
		return fmt.Errorf("game code cannot be nil")
	}

	gameId, err := gs.gameRepo.GetCode(gameCode)
	if err != nil {
		return err
	}

	loadedScript, err := gs.scriptService.Get(&scriptId)
	if err != nil {
		return err
	}

	updatedPath := "stage.scene." + sceneId
	if dataType != nil {
		updatedPath += "." + dataType.String()
	}

	err = gs.gameRepo.UpdateField(*gameId, updatedPath, loadedScript.Stage)
	if err != nil {
		return err
	}

	return nil
}

// SetSceneOrder sets the order the scenes should display; specifying a scene not on the stage results in an error.
func (gs *Service) SetSceneOrder(gameCode string, sceneOrder []string) error {
	if gameCode == "" {
		return fmt.Errorf("game code cannot be nil")
	}

	if len(sceneOrder) == 0 {
		return fmt.Errorf("must set scene order")
	}

	g, err := gs.gameRepo.FindByCode(gameCode)
	if err != nil {
		return err
	}

	for _, sceneId := range sceneOrder {
		if !slices.Contains(gs.getValidScenes(g), sceneId) {
			return fmt.Errorf("scene %s is not a valid scene", sceneId)
		}
	}

	path := "stage.sceneOrder"

	err = gs.gameRepo.UpdateField(g.ID.Hex(), path, sceneOrder)
	if err != nil {
		return err
	}

	return nil
}

// UpdateScene saves a field (or all fields if path is nil) into one of the data stores for a given scene.
func (gs *Service) UpdateScene(
	gameCode string,
	sceneId string,
	dataType models.DataStoreType,
	path *string,
	data interface{},
) error {
	g, err := gs.validateAndFetchGame(gameCode, sceneId)
	if err != nil {
		return err
	}

	fullPath := "stage.scenes." + sceneId + "." + dataType.String()

	if path != nil && *path != "" {
		fullPath += "." + *path
	}

	err = gs.gameRepo.UpdateField(g.ID.Hex(), fullPath, data)
	if err != nil {
		return err
	}

	return nil
}

// SetCurrentScene sets the current scene.
func (gs *Service) SetCurrentScene(gameCode string, sceneId string) error {
	g, err := gs.validateAndFetchGame(gameCode, sceneId)
	if err != nil {
		return err
	}

	if !slices.Contains(gs.getValidScenes(g), sceneId) {
		return fmt.Errorf("scene %s is not a valid scene", sceneId)
	}

	err = gs.gameRepo.UpdateField(g.ID.Hex(), "stage.currentScene", sceneId)
	if err != nil {
		return err
	}

	return nil
}

func (gs *Service) getValidScenes(g *models.Game) []string {
	var validScenes []string
	for key := range g.Stage.Scenes {
		validScenes = append(validScenes, key)
	}

	return validScenes
}

func (gs *Service) validateAndFetchGame(gameCode string, sceneId string) (*models.Game, error) {
	if gameCode == "" {
		return nil, fmt.Errorf("game code cannot be nil")
	}

	if sceneId == "" {
		return nil, fmt.Errorf("must specify scene")
	}

	g, err := gs.gameRepo.FindByCode(gameCode)
	if err != nil {
		return nil, err
	}

	return g, nil
}

package stage

import (
	"errors"
	"fmt"
	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	"go.mongodb.org/mongo-driver/v2/bson"
	"slices"
)

type Service struct {
	gameRepo    *gameRepo.Repo
	gameService *gameService.Service
}

// NewService creates a new repository for accessing game data.
func NewService(gameRepo *gameRepo.Repo, gameService *gameService.Service) *Service {
	return &Service{
		gameRepo,
		gameService,
	}
}

// Get will fetch the stage for a specific gameId.
func (ss *Service) Get(gameId string) (*models.Stage, error) {
	g, err := ss.gameService.Get(gameId)
	if err != nil {
		return nil, err
	}

	return &g.Stage, nil
}

// New creates and returns a new stage object.
func (ss *Service) New() (*models.Stage, error) {
	return &models.Stage{}, nil
}

// AddScene adds a scene to the Stage.
func (ss *Service) AddScene(gameId string, sceneId string, scene *models.Scene) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	if sceneId == "" {
		return fmt.Errorf("scene id cannot be nil")
	}

	g, err := ss.gameService.Fetch(gameId)
	if err != nil {
		return err
	}

	_, ok := g.Stage.Scenes[sceneId]
	if ok {
		return fmt.Errorf("scene with id %s already exists", sceneId)
	}

	path := "stage.scenes." + sceneId

	err = ss.gameRepo.UpdateField(g.ID.Hex(), path, scene)
	if err != nil {
		return err
	}

	return nil
}

// AddScenes adds multiple scenes to the Stage.
func (ss *Service) AddScenes(gameId string, scenes map[string]models.Scene) error {
	if gameId == "" {
		return errors.New("gameId cannot be nil")
	}

	if len(scenes) == 0 {
		return errors.New("scenes cannot be empty")
	}

	for sceneId, scene := range scenes {
		err := ss.AddScene(gameId, sceneId, &scene)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteScene deletes a scene from the Stage.
func (ss *Service) DeleteScene(gameId string, sceneId string) error {
	g, err := ss.validateAndFetchGame(gameId, sceneId)
	if err != nil {
		return err
	}

	path := "stage.scenes." + sceneId

	err = ss.gameRepo.DeleteField(g.ID.Hex(), path)
	if err != nil {
		return err
	}

	return nil
}

// SetScript sets a script for the current stage.
func (ss *Service) SetScript(gameId string, id string) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	g, err := ss.gameService.Fetch(gameId)
	if err != nil {
		return err
	}

	objectId, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	err = ss.gameRepo.UpdateField(g.ID.Hex(), "stage.scriptId", objectId)
	if err != nil {
		return err
	}

	return nil
}

// LoadFromScript loads a script's stage data into the current game's stage.
func (ss *Service) LoadFromScript(gameId string, scriptId string) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	err := ss.gameRepo.UpdateField(gameId, "stage", ss.gameService.Script.DefaultStage)
	if err != nil {
		return err
	}

	return nil
}

// LoadSceneFromScript loads a script's data for a given scene into the current game's stage.
func (ss *Service) LoadSceneFromScript(
	gameId string,
	sceneId string,
	dataType *models.DataStoreType,
) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	updatedPath := "stage.scene." + sceneId

	var sceneData interface{}

	sceneData, ok := ss.gameService.Script.DefaultStage.Scenes[sceneId]
	if !ok {
		return fmt.Errorf("scene %s does not exist in the default stage", sceneId)
	}

	if dataType != nil {
		updatedPath += "." + dataType.String()

		switch *dataType {
		case models.DataStorePrivate:
			sceneData = ss.gameService.Script.DefaultStage.Scenes[sceneId].PrivateData
		case models.DataStorePublic:
			sceneData = ss.gameService.Script.DefaultStage.Scenes[sceneId].PublicData
		default:
			return fmt.Errorf("invalid data store type %s", dataType.String())
		}
	}

	err := ss.gameRepo.UpdateField(gameId, updatedPath, sceneData)
	if err != nil {
		return err
	}

	return nil
}

// SetSceneOrder sets the order the scenes should display; specifying a scene not on the stage results in an error.
func (ss *Service) SetSceneOrder(gameId string, sceneOrder []string) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	g, err := ss.gameService.Fetch(gameId)
	if err != nil {
		return err
	}

	if len(sceneOrder) == 0 {
		return fmt.Errorf("must set scene order")
	}

	for _, sceneId := range sceneOrder {
		if !slices.Contains(ss.getValidScenes(g), sceneId) {
			return fmt.Errorf("scene %s is not a valid scene", sceneId)
		}
	}

	path := "stage.sceneOrder"

	err = ss.gameRepo.UpdateField(gameId, path, sceneOrder)
	if err != nil {
		return err
	}

	return nil
}

// UpdateScene saves a field (or all fields if the path is nil) into one of the data stores for a given scene.
func (ss *Service) UpdateScene(
	gameId string,
	sceneId string,
	dataType models.DataStoreType,
	path *string,
	data interface{},
) error {
	g, err := ss.gameRepo.Get(gameId)
	if err != nil {
		return err
	}

	fullPath := "stage.scenes." + sceneId + "." + dataType.String()

	if path != nil && *path != "" {
		fullPath += "." + *path
	}

	err = ss.gameRepo.UpdateField(g.ID.Hex(), fullPath, data)
	if err != nil {
		return err
	}

	return nil
}

// SetCurrentScene sets the current scene.
func (ss *Service) SetCurrentScene(gameId string, sceneId string) error {
	g, err := ss.validateAndFetchGame(gameId, sceneId)
	if err != nil {
		return err
	}

	if !slices.Contains(ss.getValidScenes(g), sceneId) {
		return fmt.Errorf("scene %s is not a valid scene", sceneId)
	}

	err = ss.gameRepo.UpdateField(g.ID.Hex(), "stage.currentScene", sceneId)
	if err != nil {
		return err
	}

	return nil
}

func (ss *Service) getValidScenes(g *models.Game) []string {
	var validScenes []string
	for key := range g.Stage.Scenes {
		validScenes = append(validScenes, key)
	}

	return validScenes
}

func (ss *Service) validateAndFetchGame(gameId string, sceneId string) (*models.Game, error) {
	if gameId == "" {
		return nil, fmt.Errorf("gameId cannot be nil")
	}

	if sceneId == "" {
		return nil, fmt.Errorf("must specify scene")
	}

	g, err := ss.gameRepo.Get(gameId)
	if err != nil {
		return nil, err
	}

	return g, nil
}

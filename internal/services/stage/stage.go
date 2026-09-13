package stage

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/robbiebyrd/indri/internal/models"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
)

// sceneStore is the narrow part of the game store this service writes through:
// one read, plus the two single-field writes whose key is published verbatim as
// the delta path. Depending on the port rather than the store keeps the path
// rules below testable without a database.
type sceneStore interface {
	Get(id string) (*models.Game, error)
	UpdateField(id string, key string, value interface{}) error
	DeleteField(id string, key string) error
}

type Service struct {
	gameRepo    sceneStore
	gameService *gameService.Service
}

// NewService creates a new repository for accessing game data.
func NewService(gameRepo *gameRepo.Store, gameService *gameService.Service) *Service {
	return &Service{
		gameRepo,
		gameService,
	}
}

// ErrInvalidPathSegment reports a caller-supplied key that cannot be used as a
// path segment.
//
// UpdateField and DeleteField use the key they are given twice: as the MongoDB
// update path and, unchanged, as the path of the published delta. A key holding
// a "." would therefore forge a segment in both — a scene id of
// "foo.privateData" writes to a nested "privateData" field and produces a delta
// path whose last segment is literally "privateData", which SanitizeDelta drops,
// so every update to that scene would vanish from the broadcast. Escaping the
// key (what events.joinPath does) is not available here, because MongoDB would
// read the escape characters as part of the field name. The boundary therefore
// rejects the key instead of transforming it.
//
// "$" is rejected for the same reason in the other direction: MongoDB does not
// accept it in a field name, and a leading "$" is read as an update operator.
var ErrInvalidPathSegment = errors.New("invalid path segment")

const (
	pathSeparator   = "."
	mongoOperator   = "$"
	forbiddenInKeys = pathSeparator + mongoOperator
)

// validateSegment rejects a key that cannot stand as a single path segment.
func validateSegment(label string, segment string) error {
	switch {
	case segment == "":
		return fmt.Errorf("%s is empty: %w", label, ErrInvalidPathSegment)
	case strings.ContainsAny(segment, forbiddenInKeys):
		return fmt.Errorf("%s %q contains %q or %q: %w",
			label, segment, pathSeparator, mongoOperator, ErrInvalidPathSegment)
	}

	return nil
}

// scenePath is the one place a scene's path is composed.
//
// Every write into a scene goes through it so the field name cannot drift
// between call sites: LoadSceneFromScript used to build "stage.scene." while
// the other three built "stage.scenes.", and the bson tag on models.Stage is
// "scenes". A scene loaded from the script therefore went into a field the
// model does not declare, where no read could ever find it again, and the
// delta announcing it named a path the client has nothing to apply to.
func scenePath(sceneId string) string {
	return "stage.scenes." + sceneId
}

// validatePath validates a caller-supplied dotted path into a data store. Dots
// in it are the caller's own separators — the path addresses a nested field —
// so each segment is checked on its own.
func validatePath(label string, path string) error {
	for _, segment := range strings.Split(path, pathSeparator) {
		if err := validateSegment(label, segment); err != nil {
			return err
		}
	}

	return nil
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

	if err := validateSegment("scene id", sceneId); err != nil {
		return err
	}

	g, err := ss.gameRepo.Get(gameId)
	if err != nil {
		return err
	}

	_, ok := g.Stage.Scenes[sceneId]
	if ok {
		return fmt.Errorf("scene with id %s already exists", sceneId)
	}

	err = ss.gameRepo.UpdateField(g.ID.Hex(), scenePath(sceneId), scene)
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

	err = ss.gameRepo.DeleteField(g.ID.Hex(), scenePath(sceneId))
	if err != nil {
		return err
	}

	return nil
}

// There is deliberately no SetScript here and no script id on models.Stage.
//
// SetScript wrote "stage.scriptId" — a bson path models.Stage does not declare
// — from an ObjectID hex that nothing in this repo ever minted: a script is not
// a Mongo document. It is the config.json the server booted with (models.Script,
// one per process, held as gameService.Script), and the Lua game scripts that
// file lists are compiled once at boot with their per-script grants
// (internal/services/lua). The layout editor attaches Lua to a board, scene or
// widget as inline source inside the layout itself, not by reference.
//
// No script is therefore selected per game or per stage, so a stage-level
// script id addresses nothing. It was removed rather than given a real field,
// because adding the field would mean inventing the per-game script selection
// it implies — a feature no caller has asked for.

// LoadFromScript loads a script's stage data into the current game's stage.
func (ss *Service) LoadFromScript(gameId string, scriptId string) error {
	if gameId == "" {
		return fmt.Errorf("gameId cannot be nil")
	}

	err := ss.gameRepo.UpdateField(gameId, "stage", ss.gameService.Script.Stage)
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

	if err := validateSegment("scene id", sceneId); err != nil {
		return err
	}

	updatedPath := scenePath(sceneId)

	var sceneData interface{}

	sceneData, ok := ss.gameService.Script.Stage.Scenes[sceneId]
	if !ok {
		return fmt.Errorf("scene %s does not exist in the default stage", sceneId)
	}

	if dataType != nil {
		updatedPath += "." + dataType.String()

		switch *dataType {
		case models.DataStorePrivate:
			sceneData = ss.gameService.Script.Stage.Scenes[sceneId].PrivateData
		case models.DataStorePublic:
			sceneData = ss.gameService.Script.Stage.Scenes[sceneId].PublicData
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

	g, err := ss.gameRepo.Get(gameId)
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
	if err := validateSegment("scene id", sceneId); err != nil {
		return err
	}

	if path != nil && *path != "" {
		if err := validatePath("scene data path", *path); err != nil {
			return err
		}
	}

	g, err := ss.gameRepo.Get(gameId)
	if err != nil {
		return err
	}

	fullPath := scenePath(sceneId) + "." + dataType.String()

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

	if err := validateSegment("scene id", sceneId); err != nil {
		return nil, err
	}

	g, err := ss.gameRepo.Get(gameId)
	if err != nil {
		return nil, err
	}

	return g, nil
}

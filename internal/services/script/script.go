package script

import (
	"fmt"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/repo/script"
	"log"
)

type Service struct {
	scriptRepo *script.Repo
}

var scriptService *Service

// NewService creates a new repository for accessing script data.
func NewService() *Service {
	if scriptService == nil {
		scriptService = &Service{
			scriptRepo: script.NewRepo(),
		}
	}

	return scriptService
}

// Sanitize removes private items.
func (ss *Service) Sanitize(script *models.Script) *models.Script {
	script.Stage.PrivateData = nil

	for i, g := range script.Stage.Scenes {
		g.PrivateData = nil
		script.Stage.Scenes[i] = g
	}

	return script
}

// Get will fetch script data for a specific script ID, or create a new one if it doesn't exist.
func (ss *Service) Get(id *string) (*models.Script, error) {
	if id == nil || *id == "" {
		return nil, fmt.Errorf("id is required")
	}

	return ss.scriptRepo.Get(*id)
}

// Fetch retrieves script data for a specific script ID, and returns an error if not found.
func (ss *Service) Fetch(id *string) (*models.Script, error) {
	if id == nil {
		return nil, fmt.Errorf("id is  nil")
	}

	return ss.scriptRepo.Get(*id)
}

// Exists checks to see if a script with the given ID already exists.
func (ss *Service) Exists(id *string) bool {
	if id == nil {
		return false
	}

	exists, err := ss.scriptRepo.Exists(*id)
	if err != nil {
		return false
	}

	return exists
}

// Update saves script data to the repository.
func (ss *Service) Update(scriptCode *string, script *models.UpdateScript) error {
	if scriptCode == nil || *scriptCode == "" {
		return fmt.Errorf("script code cannot be nil")
	}

	err := ss.scriptRepo.Update(*scriptCode, script)
	if err != nil {
		return err
	}

	return nil
}

// New creates a new script.
func (ss *Service) New() (*models.Script, error) {
	log.Printf("creating new script with script")

	s, err := ss.scriptRepo.New(models.CreateScript{})
	if err != nil {
		return nil, err
	}

	return s, nil
}

// Reset a script to its defaults.
func (ss *Service) Reset() *models.Script {
	// TODO: reload a script from a script
	return nil
}

func (ss *Service) AddScene(id *string, scene models.Scene) error {
	retrievedScript, err := ss.Get(id)
	if err != nil {
		return err
	}

	_, ok := retrievedScript.Stage.Scenes[*id]
	if ok {
		return fmt.Errorf("scene with id %v already exists", *id)
	}

	retrievedScript.Stage.Scenes[*id] = scene
	updateScript := &models.UpdateScript{
		Stage: &models.Stage{
			Scenes: retrievedScript.Stage.Scenes,
		},
	}

	err = ss.Update(id, updateScript)
	if err != nil {
		return err
	}

	return nil
}

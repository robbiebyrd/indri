package script

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/robbiebyrd/indri/internal/models"
)

type Store struct {
	script *models.Script
}

// NewStore creates a new repository for accessing user data.
func NewStore(configFilePath string) (*Store, error) {
	jsonData, err := os.ReadFile(filepath.Clean(configFilePath))
	if err != nil {
		log.Fatalf("Error reading script file: %v", err)
	}

	c := models.Script{}

	err = json.Unmarshal(jsonData, &c)
	if err != nil {
		log.Fatalf("Error parsing script JSON: %v", err)
	}

	return &Store{script: &c}, err
}

func (s *Store) Get() *models.Script {
	return s.script
}

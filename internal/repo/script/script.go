package script

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbiebyrd/indri/internal/models"
)

type Store struct {
	script *models.Script
}

// NewStore loads the script at configFilePath.
//
// Every failure is returned rather than fatal. The store is built during
// boot.Boot, which returns an error all the way to main, and a server that
// cannot read its script should say so through that path — a log.Fatalf here
// would take the process down before any caller could report or test it.
//
// The script's Scripts entries are resolved against the config file's own
// directory before they are stored, so the paths the rest of the server sees
// do not depend on the working directory it was started from.
func NewStore(configFilePath string) (*Store, error) {
	jsonData, err := os.ReadFile(filepath.Clean(configFilePath))
	if err != nil {
		return nil, fmt.Errorf("reading script file %q: %w", configFilePath, err)
	}

	c := models.Script{}

	if err := json.Unmarshal(jsonData, &c); err != nil {
		return nil, fmt.Errorf("parsing script file %q: %w", configFilePath, err)
	}

	c.Scripts = resolveScriptPaths(configFilePath, c.Scripts)

	return &Store{script: &c}, nil
}

func (s *Store) Get() *models.Script {
	return s.script
}

// resolveScriptPaths rebases each relative script path onto the directory
// holding the config file. An absolute path is left alone: an operator who
// wrote one meant it.
func resolveScriptPaths(configFilePath string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	base := filepath.Dir(configFilePath)
	resolved := make([]string, 0, len(paths))

	for _, path := range paths {
		if filepath.IsAbs(path) {
			resolved = append(resolved, filepath.Clean(path))

			continue
		}

		resolved = append(resolved, filepath.Join(base, path))
	}

	return resolved
}

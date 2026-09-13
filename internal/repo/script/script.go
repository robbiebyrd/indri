package script

import (
	"encoding/json"
	"fmt"
	"log"
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
//
// Two things here are warnings rather than errors, because a server can still
// legitimately run with either: a config written before schemaVersion existed,
// and a config that declares no game scripts. They are logged loudly because
// the second one in particular is how a working game silently becomes a shell
// that answers nothing but the built-in actions.
func NewStore(configFilePath string) (*Store, error) {
	jsonData, err := os.ReadFile(filepath.Clean(configFilePath))
	if err != nil {
		return nil, fmt.Errorf("reading script file %q: %w", configFilePath, err)
	}

	c := models.Script{}

	if err := json.Unmarshal(jsonData, &c); err != nil {
		return nil, fmt.Errorf("parsing script file %q: %w", configFilePath, err)
	}

	if err := checkSchemaVersion(configFilePath, c.SchemaVersion); err != nil {
		return nil, err
	}

	if len(c.Scripts) == 0 {
		log.Printf(
			"WARNING: the script file %q lists no lua scripts, so this server will serve no game actions;"+
				" add a %q list to it if that is not what you meant",
			configFilePath, "scripts",
		)
	}

	c.Scripts = resolveScriptPaths(configFilePath, c.Scripts)

	return &Store{script: &c}, nil
}

// checkSchemaVersion refuses a config written for a format this build does not
// know.
//
// A missing version is read as the current one and warned about, because every
// config written before this field existed has none. A version this server does
// not recognise is an error: reading a newer file with older rules is how a
// grant list or a script entry gets silently misread.
func checkSchemaVersion(configFilePath string, version int) error {
	switch {
	case version == models.ScriptSchemaVersion:
		return nil
	case version == 0:
		log.Printf(
			"WARNING: the script file %q declares no schemaVersion; reading it as version %d."+
				" Add \"schemaVersion\": %d to it.",
			configFilePath, models.ScriptSchemaVersion, models.ScriptSchemaVersion,
		)

		return nil
	default:
		return fmt.Errorf(
			"script file %q declares schema version %d, but this server understands version %d",
			configFilePath, version, models.ScriptSchemaVersion,
		)
	}
}

func (s *Store) Get() *models.Script {
	return s.script
}

// resolveScriptPaths rebases each relative script path onto the directory
// holding the config file. An absolute path is left alone: an operator who
// wrote one meant it. Grants are carried through untouched — they are validated
// where the capabilities live, when the engine is built.
func resolveScriptPaths(configFilePath string, scripts []models.ScriptFile) []models.ScriptFile {
	if len(scripts) == 0 {
		return nil
	}

	base := filepath.Dir(configFilePath)
	resolved := make([]models.ScriptFile, 0, len(scripts))

	for _, script := range scripts {
		if filepath.IsAbs(script.Path) {
			script.Path = filepath.Clean(script.Path)
		} else {
			script.Path = filepath.Join(base, script.Path)
		}

		resolved = append(resolved, script)
	}

	return resolved
}

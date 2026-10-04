// Package cli parses the server's command line: -script, and one flag per
// setting, each overriding the environment variable it is named after.
// Settings that carry a credential have no flag and come from the environment.
package cli

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"

	"github.com/robbiebyrd/indri/internal/repo/env"
)

// Parse reads args into fs and loads the settings, command-line values taking
// precedence over the environment. It returns the -script path, empty when not
// given.
func Parse(fs *flag.FlagSet, args []string) (string, error) {
	script := fs.String("script", "", "The JSON game script to stamp games from (default ./config.json).")

	for _, s := range env.Settings() {
		// A credential on the command line is readable by every user (ps).
		if s.Secret {
			continue
		}

		usage := "Overrides " + s.EnvVar + "."

		// Typed flags, so a malformed value is refused as the command line is
		// parsed rather than when the setting is first used.
		switch s.Kind {
		case reflect.Int:
			n, _ := strconv.Atoi(s.Default)
			fs.Int(s.Flag, n, usage)
		case reflect.Bool:
			b, _ := strconv.ParseBool(s.Default)
			fs.Bool(s.Flag, b, usage)
		default:
			fs.String(s.Flag, s.Default, usage)
		}
	}

	if err := fs.Parse(args); err != nil {
		return "", err
	}

	// Only flags actually given override; the rest leave the environment be.
	overrides := map[string]string{}

	fs.Visit(func(f *flag.Flag) {
		if f.Name != "script" {
			overrides[f.Name] = f.Value.String()
		}
	})

	if _, err := env.Load(overrides); err != nil {
		return "", fmt.Errorf("applying the command line: %w", err)
	}

	return *script, nil
}

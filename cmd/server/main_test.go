package main

import (
	"flag"
	"reflect"
	"testing"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

// TestRegisterSettingsFlags_CoversEverySetting guards against a config field
// that declares a flag tag but has no registered flag: env.Load would honor
// it, but the flag package rejects the unknown flag before Load ever runs.
func TestRegisterSettingsFlags_CoversEverySetting(t *testing.T) {
	fs := flag.NewFlagSet("indri", flag.ContinueOnError)
	registerSettingsFlags(fs)

	vars := reflect.TypeFor[envVars.Vars]()
	for i := range vars.NumField() {
		name := vars.Field(i).Tag.Get("flag")
		if name == "" {
			continue
		}

		if fs.Lookup(name) == nil {
			t.Errorf("config field %s has flag tag %q but no -%s flag is registered",
				vars.Field(i).Name, name, name)
		}
	}
}

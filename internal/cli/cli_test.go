package cli

import (
	"flag"
	"io"
	"testing"

	"github.com/robbiebyrd/indri/internal/repo/env"
)

func newFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("indri", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	return fs
}

// Every setting can be given on the command line.
func TestParse_RegistersAFlagForEverySetting(t *testing.T) {
	fs := newFlagSet()

	if _, err := Parse(fs, nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, s := range env.Settings() {
		if s.Secret {
			continue
		}

		if fs.Lookup(s.Flag) == nil {
			t.Errorf("setting %s has no -%s flag", s.EnvVar, s.Flag)
		}
	}
}

func TestParse_ReturnsTheScriptAndAppliesTheSettings(t *testing.T) {
	t.Setenv("INDRI_LISTEN_PORT", "6001")

	script, err := Parse(newFlagSet(), []string{"-script", "./game/config.json", "-listen-port", "7003", "-allowed-origins", "https://a.example"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if script != "./game/config.json" {
		t.Errorf("script = %q, want ./game/config.json", script)
	}

	if v := env.GetEnv(); v.ListenPort != 7003 || v.AllowedOrigins != "https://a.example" {
		t.Errorf("settings = port %d, origins %q; want the flags' 7003 and https://a.example", v.ListenPort, v.AllowedOrigins)
	}
}

func TestParse_RefusesAMalformedValue(t *testing.T) {
	if _, err := Parse(newFlagSet(), []string{"-listen-port", "lots"}); err == nil {
		t.Fatal("Parse accepted a port that is not a number")
	}
}

// A command line is visible to every user on the machine (ps), so a setting
// that carries a credential has no flag; it is set in the environment only.
func TestParse_GivesCredentialsNoFlag(t *testing.T) {
	fs := newFlagSet()

	if _, err := Parse(fs, nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, flagName := range []string{"redis-password", "postgres-uri", "mongo-uri"} {
		if fs.Lookup(flagName) != nil {
			t.Errorf("-%s exists; a credential on the command line is readable by every user", flagName)
		}
	}
}

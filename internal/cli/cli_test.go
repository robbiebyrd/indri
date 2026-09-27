package cli

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

// scriptDir returns a temp dir holding an empty script file, and its path.
func scriptDir(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	script := filepath.Join(dir, "config.json")
	if err := os.WriteFile(script, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	return dir, script
}

func newFlagSet() *flag.FlagSet {
	return flag.NewFlagSet("indri", flag.ContinueOnError)
}

// unsetAfter removes an env var the JSON config loader may set, since
// env.Load writes JSON values into the process environment.
func unsetAfter(t *testing.T, key string) {
	t.Helper()

	if _, set := os.LookupEnv(key); set {
		t.Skipf("%s is set in the environment; it would override the config under test", key)
	}
	t.Cleanup(func() { _ = os.Unsetenv(key) })
}

func TestParse_FlagOverridesSetting(t *testing.T) {
	_, script := scriptDir(t)

	path, vars, err := Parse(newFlagSet(), []string{"-script", script, "-webrtc-max-peers", "7"}, "")
	if err != nil {
		t.Fatal(err)
	}

	if path != script {
		t.Errorf("script path = %q, want %q", path, script)
	}
	if vars.WebRTCMaxPeers != 7 {
		t.Errorf("WebRTCMaxPeers = %d, want 7 from -webrtc-max-peers", vars.WebRTCMaxPeers)
	}
}

func TestParse_ReadsServerJSONBesideTheScript(t *testing.T) {
	unsetAfter(t, "INDRI_WEBRTC_MAX_PEERS")
	dir, script := scriptDir(t)
	if err := os.WriteFile(filepath.Join(dir, "server.json"), []byte(`{"webrtcMaxPeers": 9}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, vars, err := Parse(newFlagSet(), []string{"-script", script}, "")
	if err != nil {
		t.Fatal(err)
	}

	if vars.WebRTCMaxPeers != 9 {
		t.Errorf("WebRTCMaxPeers = %d, want 9 from server.json", vars.WebRTCMaxPeers)
	}
}

func TestParse_ExplicitConfigFile(t *testing.T) {
	unsetAfter(t, "INDRI_WEBRTC_MAX_PEERS")
	_, script := scriptDir(t)
	config := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.WriteFile(config, []byte(`{"webrtcMaxPeers": 11}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, vars, err := Parse(newFlagSet(), []string{"-script", script, "-config", config}, "")
	if err != nil {
		t.Fatal(err)
	}

	if vars.WebRTCMaxPeers != 11 {
		t.Errorf("WebRTCMaxPeers = %d, want 11 from -config", vars.WebRTCMaxPeers)
	}
}

func TestParse_UsesTheDefaultScriptWhenNoneIsGiven(t *testing.T) {
	_, script := scriptDir(t)

	path, _, err := Parse(newFlagSet(), nil, script)
	if err != nil {
		t.Fatal(err)
	}

	if path != script {
		t.Errorf("script path = %q, want the default %q", path, script)
	}
}

func TestParse_RequiresAScriptWithoutADefault(t *testing.T) {
	if _, _, err := Parse(newFlagSet(), nil, ""); err == nil {
		t.Fatal("Parse succeeded with no -script and no default")
	}
}

func TestParse_RejectsAMissingScriptFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")

	if _, _, err := Parse(newFlagSet(), []string{"-script", missing}, ""); err == nil {
		t.Fatal("Parse succeeded with a script file that doesn't exist")
	}
}

// TestParse_RegistersAFlagForEverySetting guards against a config field that
// declares a flag tag but has no registered flag: env.Load would honor it, but
// the flag package rejects the unknown flag before Load ever runs.
func TestParse_RegistersAFlagForEverySetting(t *testing.T) {
	fs := newFlagSet()
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

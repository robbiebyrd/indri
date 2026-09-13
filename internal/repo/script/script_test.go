package script

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeConfig writes a config file into the given directory and returns its
// path.
func writeConfig(t *testing.T, dir, name, contents string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %v: %v", path, err)
	}

	return path
}

// TestNewStore_ResolvesScriptPathsAgainstTheConfigFile is the rule a server
// started from another working directory depends on: the paths in a config are
// relative to the config, not to wherever the process happens to have been
// launched.
func TestNewStore_ResolvesScriptPathsAgainstTheConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "shared.lua")

	tests := []struct {
		name    string
		scripts string
		want    []string
	}{
		{
			name:    "no scripts key",
			scripts: "",
			want:    nil,
		},
		{
			name:    "empty list",
			scripts: `, "scripts": []`,
			want:    nil,
		},
		{
			name:    "relative paths",
			scripts: `, "scripts": ["game.lua", "lua/rules.lua"]`,
			want:    []string{filepath.Join(dir, "game.lua"), filepath.Join(dir, "lua", "rules.lua")},
		},
		{
			name:    "a path reaching above the config",
			scripts: `, "scripts": ["../shared/lib.lua"]`,
			want:    []string{filepath.Join(filepath.Dir(dir), "shared", "lib.lua")},
		},
		{
			name:    "an absolute path is left alone",
			scripts: `, "scripts": ["` + absolute + `"]`,
			want:    []string{absolute},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, dir, strings.ReplaceAll(test.name, " ", "_")+".json",
				`{"config": {"maxTeams": 2}`+test.scripts+`}`)

			store, err := NewStore(path)
			if err != nil {
				t.Fatalf("loading %v: %v", path, err)
			}

			if got := store.Get().Scripts; !slices.Equal(got, test.want) {
				t.Fatalf("Scripts = %v, want %v", got, test.want)
			}
		})
	}
}

// TestNewStore_LoadsTheRestOfTheScript checks the scripts list was added
// without disturbing what the config already carried.
func TestNewStore_LoadsTheRestOfTheScript(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, t.TempDir(), "config.json",
		`{"config": {"pvp": true, "maxTeams": 3}, "scripts": ["game.lua"]}`)

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("loading %v: %v", path, err)
	}

	script := store.Get()

	if !script.Config.PVP || script.Config.MaxTeams != 3 {
		t.Fatalf("Config = %+v, want pvp true and maxTeams 3", script.Config)
	}
}

// TestNewStore_ReturnsErrors is the whole point of the change: a bad config
// used to call log.Fatalf, which killed the process before boot.Boot — which
// returns an error — could report anything.
func TestNewStore_ReturnsErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "the file is missing",
			path: filepath.Join(dir, "absent.json"),
			want: "reading script file",
		},
		{
			name: "the file is not json",
			path: writeConfig(t, dir, "broken.json", `{"config": `),
			want: "parsing script file",
		},
		{
			name: "a field has the wrong type",
			path: writeConfig(t, dir, "mistyped.json", `{"scripts": "game.lua"}`),
			want: "parsing script file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewStore(test.path)
			if err == nil {
				t.Fatalf("loading %v succeeded, want an error", test.path)
			}

			if store != nil {
				t.Fatalf("a failed load returned a store: %+v", store)
			}

			for _, want := range []string{test.want, test.path} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
	}
}

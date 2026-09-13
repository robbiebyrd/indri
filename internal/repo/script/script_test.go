package script

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
)

// logs collects everything the store logs during the test binary's run.
//
// The store warns rather than errors on two conditions, so those warnings are
// the only evidence a test has that they were noticed at all. Capturing them
// also keeps them out of the test binary's own output, where they would be
// indistinguishable from a real failure.
var logs = &recorder{}

type recorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.buf.Write(p)
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf.Reset()
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.buf.String()
}

func TestMain(m *testing.M) {
	log.SetOutput(logs)

	os.Exit(m.Run())
}

// versioned wraps a config body in the current schema version, so a test that
// is not about versioning does not trip the missing-version warning.
func versioned(body string) string {
	return `{"schemaVersion": 1` + body + `}`
}

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
		want    []models.ScriptFile
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
			scripts: `, "scripts": [{"path": "game.lua"}, {"path": "lua/rules.lua"}]`,
			want: []models.ScriptFile{
				{Path: filepath.Join(dir, "game.lua")},
				{Path: filepath.Join(dir, "lua", "rules.lua")},
			},
		},
		{
			name:    "a path reaching above the config",
			scripts: `, "scripts": [{"path": "../shared/lib.lua"}]`,
			want:    []models.ScriptFile{{Path: filepath.Join(filepath.Dir(dir), "shared", "lib.lua")}},
		},
		{
			name:    "an absolute path is left alone",
			scripts: `, "scripts": [{"path": "` + absolute + `"}]`,
			want:    []models.ScriptFile{{Path: absolute}},
		},
		{
			name:    "grants ride along with the path",
			scripts: `, "scripts": [{"path": "game.lua", "grants": ["http"]}]`,
			want:    []models.ScriptFile{{Path: filepath.Join(dir, "game.lua"), Grants: []string{"http"}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, dir, strings.ReplaceAll(test.name, " ", "_")+".json",
				versioned(`, "config": {"maxTeams": 2}`+test.scripts))

			store, err := NewStore(path)
			if err != nil {
				t.Fatalf("loading %v: %v", path, err)
			}

			if got := store.Get().Scripts; !slices.EqualFunc(got, test.want, sameScriptFile) {
				t.Fatalf("Scripts = %+v, want %+v", got, test.want)
			}
		})
	}
}

func sameScriptFile(a, b models.ScriptFile) bool {
	return a.Path == b.Path && slices.Equal(a.Grants, b.Grants)
}

// TestNewStore_LoadsTheRestOfTheScript checks the scripts list was added
// without disturbing what the config already carried.
func TestNewStore_LoadsTheRestOfTheScript(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, t.TempDir(), "config.json",
		versioned(`, "config": {"pvp": true, "maxTeams": 3}, "scripts": [{"path": "game.lua"}]`))

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("loading %v: %v", path, err)
	}

	script := store.Get()

	if !script.Config.PVP || script.Config.MaxTeams != 3 {
		t.Fatalf("Config = %+v, want pvp true and maxTeams 3", script.Config)
	}

	if script.SchemaVersion != models.ScriptSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", script.SchemaVersion, models.ScriptSchemaVersion)
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
		{
			name: "a script entry is a bare string",
			path: writeConfig(t, dir, "bare.json", `{"scripts": ["game.lua"]}`),
			want: "parsing script file",
		},
		{
			name: "the schema version is from the future",
			path: writeConfig(t, dir, "future.json", `{"schemaVersion": 99}`),
			want: "declares schema version 99, but this server understands version 1",
		},
		{
			name: "the schema version is nonsense",
			path: writeConfig(t, dir, "negative.json", `{"schemaVersion": -1}`),
			want: "declares schema version -1",
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

// TestNewStore_WarnsLoudly covers the two conditions a server can survive but
// an operator almost never meant.
//
// A config with no scripts boots a server that answers nothing but the built-in
// actions — a working game becomes a shell, and without this warning the only
// symptom is that every move is refused. A config with no schemaVersion is
// every config written before the field existed, and saying so at boot is what
// makes a later format change a migration rather than a mystery.
//
// These subtests are serial on purpose: they read a process-wide log.
func TestNewStore_WarnsLoudly(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		contents string
		want     []string
	}{
		{
			name:     "no scripts key at all",
			contents: versioned(`, "config": {"maxTeams": 2}`),
			want:     []string{"WARNING", "no lua scripts", "no game actions"},
		},
		{
			name:     "an empty scripts list",
			contents: versioned(`, "scripts": []`),
			want:     []string{"WARNING", "no lua scripts"},
		},
		{
			name:     "no schema version",
			contents: `{"scripts": [{"path": "game.lua"}]}`,
			want:     []string{"WARNING", "declares no schemaVersion", "version 1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs.reset()

			path := writeConfig(t, dir, strings.ReplaceAll(test.name, " ", "_")+".json", test.contents)

			if _, err := NewStore(path); err != nil {
				t.Fatalf("loading %v: %v", path, err)
			}

			got := logs.String()

			for _, want := range append(test.want, path) {
				if !strings.Contains(got, want) {
					t.Fatalf("the log %q does not mention %q", got, want)
				}
			}
		})
	}
}

// TestNewStore_IsQuietWhenTheConfigIsComplete is the other half of the warning
// tests: a warning that fires on a healthy config is one an operator learns to
// ignore.
func TestNewStore_IsQuietWhenTheConfigIsComplete(t *testing.T) {
	logs.reset()

	path := writeConfig(t, t.TempDir(), "complete.json",
		versioned(`, "scripts": [{"path": "game.lua"}]`))

	if _, err := NewStore(path); err != nil {
		t.Fatalf("loading %v: %v", path, err)
	}

	if got := logs.String(); got != "" {
		t.Fatalf("loading a complete config logged %q, want nothing", got)
	}
}
